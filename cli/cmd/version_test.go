/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionCommandTable(t *testing.T) {
	oldVersion, oldCommit, oldBuildTime := version, gitCommit, buildTime
	version, gitCommit, buildTime = "v1.2.3", "abc1234", "2026-08-24_00:00:00"
	defer func() { version, gitCommit, buildTime = oldVersion, oldCommit, oldBuildTime }()

	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version"})
	defer rootCmd.SetArgs(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"Version:    v1.2.3", "Git Commit: abc1234", "Build Time: 2026-08-24_00:00:00", "Go Version:", "Platform:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestVersionCommandJSON(t *testing.T) {
	oldVersion, oldCommit, oldBuildTime := version, gitCommit, buildTime
	version, gitCommit, buildTime = "v1.2.3", "abc1234", "2026-08-24_00:00:00"
	defer func() { version, gitCommit, buildTime = oldVersion, oldCommit, oldBuildTime }()

	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version", "--output", "json"})
	defer rootCmd.SetArgs(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info buildInfo
	if err := json.Unmarshal(buf.Bytes(), &info); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v\noutput: %s", err, buf.String())
	}
	if info.Version != "v1.2.3" || info.GitCommit != "abc1234" || info.BuildTime != "2026-08-24_00:00:00" {
		t.Errorf("unexpected build info: %+v", info)
	}
	if info.GoVersion == "" || info.Platform == "" {
		t.Errorf("expected GoVersion and Platform to be populated: %+v", info)
	}
}

func TestVersionCommandInvalidOutputFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version", "--output", "xml"})
	defer rootCmd.SetArgs(nil)

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for unsupported output format, got nil")
	}
}
