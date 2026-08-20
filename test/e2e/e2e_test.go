//go:build e2e

/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nirmata/ottoflow/test/utils"
)

var _ = Describe("controller", Ordered, func() {
	Context("Operator", func() {
		It("should run successfully", func() {
			var controllerPodName string

			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func() error {
				podListTemplate := "{{ range .items }}" +
					"{{ if not .metadata.deletionTimestamp }}" +
					"{{ .metadata.name }}" +
					"{{ \"\\n\" }}" +
					"{{ end }}" +
					"{{ end }}"
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template="+podListTemplate,
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				ExpectWithOffset(2, err).NotTo(HaveOccurred())
				podNames := utils.GetNonEmptyLines(string(podOutput))
				if len(podNames) != 1 {
					return fmt.Errorf("expect 1 controller pods running, but got %d", len(podNames))
				}
				controllerPodName = podNames[0]

				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				status, err := utils.Run(cmd)
				ExpectWithOffset(2, err).NotTo(HaveOccurred())
				if string(status) != "Running" {
					return fmt.Errorf("controller pod in %s status", status)
				}
				return nil
			}
			// Controller was already waited on in BeforeSuite; short timeout for sanity check
			EventuallyWithOffset(1, verifyControllerUp, 15*time.Second, time.Second).Should(Succeed())
		})

		It("should execute WorkflowRuns through a runner Job", func() {
			workflowManifest := `
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: e2e-job-workflow
  namespace: ottoflow
spec:
  steps:
    - name: hello
      expressions:
        - name: result
          expression: '"hello from runner"'
---
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: WorkflowRun
metadata:
  name: e2e-job-run
  namespace: ottoflow
spec:
  workflowRef:
    name: e2e-job-workflow
    namespace: ottoflow
`
			manifestFile, err := os.CreateTemp("", "ottoflow-e2e-*.yaml")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = os.Remove(manifestFile.Name())
			}()
			_, err = manifestFile.WriteString(workflowManifest)
			Expect(err).NotTo(HaveOccurred())
			Expect(manifestFile.Close()).To(Succeed())

			By("creating a Workflow and WorkflowRun")
			cmd := exec.Command("kubectl", "apply", "-f", manifestFile.Name())
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() {
				cmd := exec.Command("kubectl", "delete", "--ignore-not-found=true", "-f", manifestFile.Name())
				_, _ = utils.Run(cmd)
			})

			var jobName string
			By("waiting for the controller to create a runner Job")
			Eventually(func() string {
				cmd := exec.Command(
					"kubectl", "get", "workflowrun", "e2e-job-run",
					"-n", namespace,
					"-o", "jsonpath={.status.execution.jobName}",
				)
				output, err := utils.Run(cmd)
				if err != nil {
					return ""
				}
				jobName = string(output)
				return jobName
			}, 2*time.Minute, time.Second).ShouldNot(BeEmpty())

			By("waiting for the WorkflowRun to succeed")
			Eventually(func() string {
				cmd := exec.Command(
					"kubectl", "get", "workflowrun", "e2e-job-run",
					"-n", namespace,
					"-o", "jsonpath={.status.phase}",
				)
				output, err := utils.Run(cmd)
				if err != nil {
					return ""
				}
				return string(output)
			}, 3*time.Minute, time.Second).Should(Equal("Succeeded"))

			By("verifying the runner Job exists")
			cmd = exec.Command("kubectl", "get", "job", jobName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the runner pod name is reported")
			Eventually(func() string {
				cmd := exec.Command(
					"kubectl", "get", "workflowrun", "e2e-job-run",
					"-n", namespace,
					"-o", "jsonpath={.status.execution.podName}",
				)
				output, err := utils.Run(cmd)
				if err != nil {
					return ""
				}
				return string(output)
			}, 2*time.Minute, time.Second).ShouldNot(BeEmpty())
		})
	})
})

// buildAndLoadLocalImage builds the image with ko and loads it into the kind cluster.
// Returns (imageRef, nil) on success.
func buildAndLoadLocalImageResult(importPath string) (string, error) {
	projectDir, err := utils.GetProjectDir()
	if err != nil {
		return "", err
	}
	koBinary := "ko"
	if _, err := exec.LookPath(koBinary); err != nil {
		koBinary = filepath.Join(projectDir, "bin", "ko-latest")
	}
	cmd := exec.Command(koBinary, "build", "--local", "--base-import-paths", importPath)
	output, err := utils.Run(cmd)
	if err != nil {
		return "", err
	}
	lines := utils.GetNonEmptyLines(string(output))
	if len(lines) == 0 {
		return "", fmt.Errorf("ko build produced no image ref for %s", importPath)
	}
	imageRef := lines[len(lines)-1]
	if err := utils.LoadImageToKindClusterWithName(imageRef); err != nil {
		return "", err
	}
	return imageRef, nil
}
