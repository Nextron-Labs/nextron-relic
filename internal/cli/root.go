// Package cli implements the relic command line interface.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Nextron-Labs/nextron-relic/internal/version"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

// exitError carries a process exit code.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func usageErr(format string, args ...interface{}) error {
	return &exitError{code: result.ExitUsage, err: fmt.Errorf(format, args...)}
}

// silentExit ends the process with a code without printing anything more.
func silentExit(code int) error { return &exitError{code: code} }

// Main runs the CLI and returns the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	root := newRoot(stdout, stderr)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.err != nil {
			fmt.Fprintln(stderr, "relic:", ee.err)
		}
		return ee.code
	}
	// cobra flag/argument errors
	fmt.Fprintln(stderr, "relic:", err)
	fmt.Fprintln(stderr, "Run 'relic --help' for usage.")
	return result.ExitUsage
}

func newRoot(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "relic",
		Short: "Nextron Relic - forensic artifact collector",
		Long: `Nextron Relic collects forensic artifacts on Windows, Linux and macOS into a
ZIP archive with a manifest (hashes, timestamps, access method and status of
every file). Collections are controlled by built-in profiles or a YAML
configuration.

Exit codes: 0 complete, 1 partial, 2 failed, 3 canceled, 4 invalid usage/configuration.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetVersionTemplate("relic {{.Version}}\n")
	root.CompletionOptions.HiddenDefaultCmd = true
	root.AddCommand(
		newCollectCmd(stdout, stderr),
		newValidateCmd(stdout, stderr),
		newListCmd(stdout),
		newShowCmd(stdout),
		newConfigCmd(stdout, stderr),
		newVerifyCmd(stdout, stderr),
		newVersionCmd(stdout),
	)
	return root
}

func writeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func newVersionCmd(stdout io.Writer) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			t := version.Tool()
			if asJSON {
				return writeJSON(stdout, t)
			}
			fmt.Fprintf(stdout, "relic %s", t.Version)
			if t.Commit != "" {
				fmt.Fprintf(stdout, " (%s)", t.Commit)
			}
			fmt.Fprintf(stdout, " %s\n", t.GoVersion)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	return cmd
}
