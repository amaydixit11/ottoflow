/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cmd

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionOutputFormat string

// buildInfo holds the build version details reported by the version command.
type buildInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildTime string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:          "version",
	Short:        "Show CLI build version details",
	SilenceUsage: true,
	Long: `Show the OttoFlow CLI build version details, including the version,
git commit, build time, Go version, and platform.

Examples:
  # Show build version details
  ottoflow version

  # Show build version details in JSON format
  ottoflow version --output json`,
	Args: cobra.NoArgs,
	RunE: getVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().StringVarP(&versionOutputFormat, "output", "o", "table", "Output format: table, json")
}

func getVersion(cmd *cobra.Command, args []string) error {
	info := buildInfo{
		Version:   version,
		GitCommit: gitCommit,
		BuildTime: buildTime,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}

	switch versionOutputFormat {
	case "json":
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal version info: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	case "table":
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Version:    %s\n", info.Version)
		fmt.Fprintf(out, "Git Commit: %s\n", info.GitCommit)
		fmt.Fprintf(out, "Build Time: %s\n", info.BuildTime)
		fmt.Fprintf(out, "Go Version: %s\n", info.GoVersion)
		fmt.Fprintf(out, "Platform:   %s\n", info.Platform)
	default:
		return fmt.Errorf("unsupported output format: %s (must be table or json)", versionOutputFormat)
	}

	return nil
}
