/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/cli/internal/display"
)

// SaveRunOutput saves JSON and Markdown files for a WorkflowRun to outputDir.
// includeInputs controls whether spec.inputValues are included in the JSON file
// (they may contain secrets). Returns the paths of saved files.
func SaveRunOutput(workflowRun *ottoflowv1alpha1.WorkflowRun, outputDir string, includeInputs bool) (jsonPath, mdPath string, err error) {
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return "", "", fmt.Errorf("create output dir: %w", err)
	}

	workflowName := workflowNameForFilename(workflowRun)
	ts := completionTimestamp(workflowRun)
	base := fmt.Sprintf("%s-%s", workflowName, ts.Format("20060102-1504"))
	base = uniqueBaseName(outputDir, base)

	jsonPath = filepath.Join(outputDir, base+".json")
	mdPath = filepath.Join(outputDir, base+".md")

	if err := writeJSON(workflowRun, jsonPath, includeInputs); err != nil {
		return "", "", fmt.Errorf("write JSON: %w", err)
	}
	if err := writeMarkdown(workflowRun, mdPath, workflowName, ts); err != nil {
		return jsonPath, "", fmt.Errorf("write markdown: %w", err)
	}

	return jsonPath, mdPath, nil
}

func workflowNameForFilename(workflowRun *ottoflowv1alpha1.WorkflowRun) string {
	if workflowRun.Spec.WorkflowRef.Name != "" {
		return workflowRun.Spec.WorkflowRef.Name
	}
	if workflowRun.Name != "" {
		return workflowRun.Name
	}
	return "workflow"
}

func writeJSON(workflowRun *ottoflowv1alpha1.WorkflowRun, path string, includeInputs bool) error {
	outputMap := display.BuildOutputMap(workflowRun, includeInputs)
	data, err := json.MarshalIndent(outputMap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0640)
}

func writeMarkdown(workflowRun *ottoflowv1alpha1.WorkflowRun, path, workflowName string, ts time.Time) error {
	content := buildMarkdown(workflowRun, workflowName, ts)
	return os.WriteFile(path, []byte(content), 0640)
}

func buildMarkdown(workflowRun *ottoflowv1alpha1.WorkflowRun, workflowName string, ts time.Time) string {
	phase := string(workflowRun.Status.Phase)
	if phase == "" {
		phase = "Unknown"
	}

	duration := ""
	if workflowRun.Status.StartTime != nil && workflowRun.Status.CompletionTime != nil {
		d := workflowRun.Status.CompletionTime.Sub(workflowRun.Status.StartTime.Time)
		duration = fmt.Sprintf(" | Duration: %s", d.Round(time.Second))
	}

	timestampLabel := "Saved"
	if workflowRun.Status.CompletionTime != nil {
		timestampLabel = "Completed"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s — %s\n", workflowName, phase)
	fmt.Fprintf(&sb, "_%s: %s%s_\n", timestampLabel, ts.Format("2006-01-02 15:04:05"), duration)
	sb.WriteString("\n---\n\n")

	if md := findMarkdownOutput(workflowRun); md != "" {
		sb.WriteString(md)
		sb.WriteString("\n")
		return sb.String()
	}

	sb.WriteString(generateStructuredReport(workflowRun))
	return sb.String()
}

// findMarkdownOutput checks outputs for markdown content.
// Prioritizes known markdown output names before falling back to content detection.
// Keys are sorted for deterministic selection when multiple outputs match.
func findMarkdownOutput(workflowRun *ottoflowv1alpha1.WorkflowRun) string {
	names := sortedKeys(workflowRun.Status.Outputs)
	// First pass: check known markdown output names
	for _, name := range names {
		if !display.MarkdownOutputNames[name] {
			continue
		}
		if str := unmarshalString(workflowRun.Status.Outputs[name].Raw); str != "" {
			return str
		}
	}
	// Second pass: detect markdown by content (skip already-checked named outputs)
	for _, name := range names {
		if display.MarkdownOutputNames[name] {
			continue
		}
		if str := unmarshalString(workflowRun.Status.Outputs[name].Raw); str != "" && display.LooksLikeMarkdown(str) {
			return str
		}
	}
	return ""
}

func unmarshalString(raw []byte) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	str, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str)
}

func generateStructuredReport(workflowRun *ottoflowv1alpha1.WorkflowRun) string {
	var sb strings.Builder

	// Step summary
	if len(workflowRun.Status.StepStatuses) > 0 {
		sb.WriteString("## Steps\n\n")
		sb.WriteString("| Step | Phase | Duration | Message |\n")
		sb.WriteString("|------|-------|----------|---------|\n")
		stepNames := sortedKeys(workflowRun.Status.StepStatuses)
		for _, name := range stepNames {
			s := workflowRun.Status.StepStatuses[name]
			dur := ""
			if s.StartTime != nil && s.CompletionTime != nil {
				d := s.CompletionTime.Sub(s.StartTime.Time)
				dur = d.Round(time.Millisecond).String()
			}
			msg := s.Message
			if s.Error != "" {
				msg = s.Error
			}
			msg = truncateRunes(msg, 80)
			fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n",
				escapeTableCell(name), s.Phase, dur, escapeTableCell(msg))
		}
		sb.WriteString("\n")
	}

	// Outputs
	if len(workflowRun.Status.Outputs) > 0 {
		sb.WriteString("## Outputs\n\n")
		outputNames := sortedKeys(workflowRun.Status.Outputs)
		for _, name := range outputNames {
			value := workflowRun.Status.Outputs[name]
			var outputValue interface{}
			if err := json.Unmarshal(value.Raw, &outputValue); err != nil {
				fmt.Fprintf(&sb, "**%s**: %s\n\n", name, string(value.Raw))
				continue
			}
			switch v := outputValue.(type) {
			case string:
				if len([]rune(v)) > 200 {
					fmt.Fprintf(&sb, "**%s**:\n```\n%s\n```\n\n", name, truncateRunes(v, 200))
				} else {
					fmt.Fprintf(&sb, "**%s**: %s\n\n", name, v)
				}
			default:
				formatted, err := json.MarshalIndent(v, "", "  ")
				if err != nil {
					fmt.Fprintf(&sb, "**%s**:\n```json\n%s\n```\n\n", name, string(value.Raw))
				} else {
					fmt.Fprintf(&sb, "**%s**:\n```json\n%s\n```\n\n", name, string(formatted))
				}
			}
		}
	}

	if sb.Len() == 0 {
		sb.WriteString("_No step results or outputs._\n")
	}

	return sb.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func completionTimestamp(workflowRun *ottoflowv1alpha1.WorkflowRun) time.Time {
	if workflowRun.Status.CompletionTime != nil {
		return workflowRun.Status.CompletionTime.Time
	}
	return time.Now()
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}

func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// uniqueBaseName returns base if no collision, or base-1, base-2, etc. up to base-999.
// If all 1000 candidates exist, falls back to base (which will overwrite existing files).
// Checks existence of both .json and .md to keep the pair in sync.
func uniqueBaseName(dir, base string) string {
	jsonPath := filepath.Join(dir, base+".json")
	mdPath := filepath.Join(dir, base+".md")
	if !fileExists(jsonPath) && !fileExists(mdPath) {
		return base
	}
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		jsonPath = filepath.Join(dir, candidate+".json")
		mdPath = filepath.Join(dir, candidate+".md")
		if !fileExists(jsonPath) && !fileExists(mdPath) {
			return candidate
		}
	}
	// Extremely unlikely; fall back to original base (will overwrite)
	return base
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	// Treat permission errors conservatively as "exists" to avoid accidental overwrites.
	return !os.IsNotExist(err)
}
