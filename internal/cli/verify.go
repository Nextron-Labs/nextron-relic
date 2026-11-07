package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Nextron-Labs/nextron-relic/internal/verify"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

func newVerifyCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		asJSON     bool
		resultPath string
	)
	cmd := &cobra.Command{
		Use:   "verify <archive.zip>",
		Short: "Verify an archive: integrity, manifest and every file hash",
		Long: `Verify a Relic archive without extracting it:

- the ZIP structure and the CRC-32 of every entry,
- manifest.json, files.jsonl, relic.log and config.effective.yaml are present,
- every stored file has a record in files.jsonl, and its size and hashes
  match the record,
- the counters in the manifest match the records,
- with --result, the archive size and SHA-256 match the result written by
  'relic collect --json' or --status-file.

Exit codes: 0 verified, 2 problems found or archive unreadable, 4 invalid usage.`,
		Example: `  relic verify relic_HOST_20261107T215424Z.zip
  relic verify out/relic.zip --result out/status.json --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var o verify.Options
			if resultPath != "" {
				data, err := os.ReadFile(resultPath)
				if err != nil {
					return usageErr("--result: %v", err)
				}
				var res result.Result
				if err := json.Unmarshal(data, &res); err != nil {
					return usageErr("--result: not a Relic result: %v", err)
				}
				if res.Output == nil || res.Output.SHA256 == "" {
					return usageErr("--result: the result has no archive hash (status %s)", res.Status)
				}
				o.Expect = res.Output
			}
			rep, err := verify.Archive(args[0], o)
			if err != nil {
				return &exitError{code: result.ExitFailed, err: fmt.Errorf("verify %s: %v", args[0], err)}
			}
			if asJSON {
				if err := writeJSON(stdout, rep); err != nil {
					return err
				}
			} else {
				printVerify(stdout, rep)
			}
			if !rep.OK {
				return silentExit(result.ExitFailed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the report as JSON")
	cmd.Flags().StringVar(&resultPath, "result", "", "result JSON `file` of the collection (--json output or --status-file) to check the archive hash against")
	return cmd
}

func printVerify(w io.Writer, r *verify.Report) {
	state := "OK"
	if !r.OK {
		state = fmt.Sprintf("FAILED (%d problems)", len(r.Problems)+r.ProblemsDropped)
	}
	fmt.Fprintf(w, "%s: %s\n", r.Archive, state)
	fmt.Fprintf(w, "  archive:    %s, sha256 %s\n", humanBytes(r.ArchiveSize), r.ArchiveSHA256)
	if c := r.Collection; c != nil {
		fmt.Fprintf(w, "  collection: %s on %s (%s), %s, started %s", c.Tool, c.Host, c.OS, c.Status, c.StartedAt)
		if c.JobID != "" {
			fmt.Fprintf(w, ", job %s", c.JobID)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "  verified:   %d stored files (%s) match their recorded size and hashes\n", r.FilesVerified, humanBytes(r.BytesVerified))
	for _, p := range r.Problems {
		fmt.Fprintf(w, "  problem:    %s\n", p)
	}
	if r.ProblemsDropped > 0 {
		fmt.Fprintf(w, "  ... and %d more problems\n", r.ProblemsDropped)
	}
}
