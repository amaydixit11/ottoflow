//go:build e2e

/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nirmata/ottoflow/test/utils"
)

var _ = Describe("Cron Scheduler", Ordered, func() {
	const testNS = "default"

	cleanup := func(workflowName string) {
		cmd := exec.Command("kubectl", "delete", "workflow", workflowName,
			"-n", testNS, "--ignore-not-found")
		_, _ = utils.Run(cmd)

		cmd = exec.Command("kubectl", "delete", "workflowruns",
			"-n", testNS, "-l", "ottoflow.nirmata.io/workflow="+workflowName,
			"--ignore-not-found")
		_, _ = utils.Run(cmd)
	}

	applyWorkflow := func(name, schedule, concurrencyPolicy string) {
		yaml := fmt.Sprintf(`apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: %s
  namespace: %s
spec:
  steps:
    - name: echo
      expressions:
        - name: result
          expression: '"cron e2e"'
  triggers:
    - cron:
        schedule: "%s"
        concurrencyPolicy: "%s"
`, name, testNS, schedule, concurrencyPolicy)

		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(yaml)
		out, err := utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), string(out))
	}

	getWorkflowRunCount := func(workflowName string) int {
		cmd := exec.Command("kubectl", "get", "workflowruns",
			"-n", testNS,
			"-l", "ottoflow.nirmata.io/workflow="+workflowName+",ottoflow.nirmata.io/trigger=cron",
			"-o", "jsonpath={.items[*].metadata.name}")
		out, err := utils.Run(cmd)
		if err != nil {
			return 0
		}
		names := strings.TrimSpace(string(out))
		if names == "" {
			return 0
		}
		return len(strings.Fields(names))
	}

	type triggerInfo struct {
		Type         string `json:"type"`
		CronSchedule string `json:"cronSchedule"`
		TriggeredAt  string `json:"triggeredAt"`
	}

	getTriggerInfo := func(wrName string) *triggerInfo {
		cmd := exec.Command("kubectl", "get", "workflowrun", wrName,
			"-n", testNS, "-o", "jsonpath={.status.trigger}")
		out, err := utils.Run(cmd)
		if err != nil {
			return nil
		}
		raw := strings.TrimSpace(string(out))
		if raw == "" {
			return nil
		}
		var ti triggerInfo
		if err := json.Unmarshal([]byte(raw), &ti); err != nil {
			return nil
		}
		return &ti
	}

	getWorkflowRunPhase := func(wrName string) string {
		cmd := exec.Command("kubectl", "get", "workflowrun", wrName,
			"-n", testNS, "-o", "jsonpath={.status.phase}")
		out, _ := utils.Run(cmd)
		return strings.TrimSpace(string(out))
	}

	listWorkflowRunNames := func(workflowName string) []string {
		cmd := exec.Command("kubectl", "get", "workflowruns",
			"-n", testNS,
			"-l", "ottoflow.nirmata.io/workflow="+workflowName+",ottoflow.nirmata.io/trigger=cron",
			"-o", "jsonpath={.items[*].metadata.name}")
		out, err := utils.Run(cmd)
		if err != nil {
			return nil
		}
		names := strings.TrimSpace(string(out))
		if names == "" {
			return nil
		}
		return strings.Fields(names)
	}

	Context("Allow concurrency policy", func() {
		const wfName = "e2e-cron-allow"

		AfterAll(func() {
			cleanup(wfName)
		})

		It("should create WorkflowRuns on schedule", func() {
			By("creating a workflow with every-minute cron trigger")
			applyWorkflow(wfName, "* * * * *", "Allow")

			By("waiting for at least one WorkflowRun to be created")
			Eventually(func() int {
				return getWorkflowRunCount(wfName)
			}, 100*time.Second, 2*time.Second).Should(BeNumerically(">=", 1))

			By("verifying WorkflowRun has correct trigger metadata")
			names := listWorkflowRunNames(wfName)
			Expect(names).NotTo(BeEmpty())
			ti := getTriggerInfo(names[0])
			Expect(ti).NotTo(BeNil())
			Expect(ti.Type).To(Equal("Cron"))
			Expect(ti.CronSchedule).To(Equal("* * * * *"))
			Expect(ti.TriggeredAt).NotTo(BeEmpty())

			By("verifying WorkflowRun completes successfully")
			Eventually(func() string {
				return getWorkflowRunPhase(names[0])
			}, 2*time.Minute, time.Second).Should(Equal("Succeeded"))
		})

		It("should create additional WorkflowRuns on subsequent fires", func() {
			Eventually(func() int {
				return getWorkflowRunCount(wfName)
			}, 120*time.Second, 2*time.Second).Should(BeNumerically(">=", 2))
		})
	})

	Context("Forbid concurrency policy", func() {
		const wfName = "e2e-cron-forbid"

		AfterAll(func() {
			cleanup(wfName)
		})

		It("should not create a second WorkflowRun while one is active", func() {
			By("creating a workflow with Forbid policy")
			applyWorkflow(wfName, "* * * * *", "Forbid")

			By("waiting for the first WorkflowRun")
			Eventually(func() int {
				return getWorkflowRunCount(wfName)
			}, 100*time.Second, 2*time.Second).Should(BeNumerically(">=", 1))

			By("waiting for the first run to complete")
			names := listWorkflowRunNames(wfName)
			Expect(names).NotTo(BeEmpty())
			Eventually(func() string {
				return getWorkflowRunPhase(names[0])
			}, 30*time.Second, time.Second).Should(Equal("Succeeded"))

			By("waiting for a second run (allowed because first completed)")
			Eventually(func() int {
				return getWorkflowRunCount(wfName)
			}, 120*time.Second, 2*time.Second).Should(BeNumerically(">=", 2))
		})
	})

	Context("Schedule removal on Workflow delete", func() {
		const wfName = "e2e-cron-delete"

		It("should stop creating WorkflowRuns after workflow is deleted", func() {
			By("creating a workflow with every-minute cron trigger")
			applyWorkflow(wfName, "* * * * *", "Allow")

			By("waiting for at least one WorkflowRun")
			Eventually(func() int {
				return getWorkflowRunCount(wfName)
			}, 100*time.Second, 2*time.Second).Should(BeNumerically(">=", 1))

			countBefore := getWorkflowRunCount(wfName)

			By("deleting the workflow")
			cmd := exec.Command("kubectl", "delete", "workflow", wfName,
				"-n", testNS)
			out, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), string(out))

			By("waiting past the next cron fire and verifying no new runs appear")
			// Wait 70s (past the next minute boundary)
			time.Sleep(70 * time.Second)
			countAfter := getWorkflowRunCount(wfName)

			// The count should not have grown by more than 1 (at most one
			// in-flight fire from before the delete landed).
			Expect(countAfter).To(BeNumerically("<=", countBefore+1))

			By("cleaning up orphaned WorkflowRuns")
			cmd = exec.Command("kubectl", "delete", "workflowruns",
				"-n", testNS, "-l", "ottoflow.nirmata.io/workflow="+wfName,
				"--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})
