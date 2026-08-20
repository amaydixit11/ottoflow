//go:build gendocs

// Generates CLI reference docs (docs/cli/*.md) from the Cobra command tree.
// This file lives inside the command package so it can reference the root
// command directly — the repo does not need to export it. The gendocs build
// tag keeps it out of normal `go test ./...` runs; the docs workflow invokes
// it explicitly before every publish.
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra/doc"
)

func TestGenerateCliDocs(t *testing.T) {
	// Walk up from the package dir to the repo root (marked by go.mod).
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from package dir")
		}
		dir = parent
	}

	out := filepath.Join(dir, "docs", "cli")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	root := rootCmd
	root.DisableAutoGenTag = true
	if err := doc.GenMarkdownTree(root, out); err != nil {
		t.Fatal(err)
	}
}
