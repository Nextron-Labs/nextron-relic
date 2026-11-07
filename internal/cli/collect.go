package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Nextron-Labs/nextron-relic/internal/catalog"
	"github.com/Nextron-Labs/nextron-relic/internal/collector"
	"github.com/Nextron-Labs/nextron-relic/internal/hostinfo"
	"github.com/Nextron-Labs/nextron-relic/internal/logging"
	"github.com/Nextron-Labs/nextron-relic/internal/version"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

type collectFlags struct {
	configPath   string
	profile      string
	artifacts    []string
	excludes     []string
	output       string
	overwrite    bool
	dryRun       bool
	asJSON       bool
	statusFile   string
	maxDuration  string
	maxTotalSize string
	maxFileSize  string
	method       string
	logLevel     string
	logFormat    string
	quiet        bool
}

func newCollectCmd(stdout, stderr io.Writer) *cobra.Command {
	f := &collectFlags{}
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect artifacts into a ZIP archive",
		Long: `Collect artifacts into a ZIP archive.

Without --config the built-in "standard" profile is collected. Flags override
the corresponding configuration values and are recorded in the result.

The archive is written to <output>.partial and renamed when complete. It
contains files/ (collected data), manifest.json (result), files.jsonl (one
record per file), relic.log, config.yaml and config.effective.yaml.`,
		Example: `  relic collect
  relic collect --profile minimal --output D:\cases\
  relic collect --config triage.yaml --json --status-file C:\Windows\Temp\relic-status.json
  relic collect --profile standard --artifact browser.* --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollect(cmd, f, stdout, stderr)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&f.configPath, "config", "c", "", "YAML configuration file ('-' reads from stdin)")
	fl.StringVarP(&f.profile, "profile", "p", "", "built-in profile (minimal, standard, full); replaces collect.profile")
	fl.StringArrayVarP(&f.artifacts, "artifact", "a", nil, "add an artifact or wildcard (repeatable)")
	fl.StringArrayVarP(&f.excludes, "exclude-artifact", "x", nil, "exclude an artifact or wildcard (repeatable)")
	fl.StringVarP(&f.output, "output", "o", "", "archive file, or directory if it exists or ends with a path separator; .zip is appended to file names without it; placeholders {hostname}, {timestamp}, {job_id}")
	fl.BoolVar(&f.overwrite, "overwrite", false, "overwrite an existing archive")
	fl.BoolVar(&f.dryRun, "dry-run", false, "resolve and list matching files without reading them or writing an archive")
	fl.BoolVar(&f.asJSON, "json", false, "print the result as JSON on stdout")
	fl.StringVar(&f.statusFile, "status-file", "", "also write the result JSON to this file")
	fl.StringVar(&f.maxDuration, "max-duration", "", "override limits.max_duration (e.g. 30m, 0 = unlimited)")
	fl.StringVar(&f.maxTotalSize, "max-total-size", "", "override limits.max_total_size (e.g. 10GiB, 0 = unlimited)")
	fl.StringVar(&f.maxFileSize, "max-file-size", "", "override limits.max_file_size (e.g. 1GiB, 0 = unlimited)")
	fl.StringVar(&f.method, "method", "", "override collect.method (auto, os, raw)")
	fl.StringVar(&f.logLevel, "log-level", "info", "console log level (debug, info, warn, error)")
	fl.StringVar(&f.logFormat, "log-format", "text", "console log format on stderr (text, json)")
	fl.BoolVarP(&f.quiet, "quiet", "q", false, "only log errors on the console")
	return cmd
}

func runCollect(cmd *cobra.Command, f *collectFlags, stdout, stderr io.Writer) error {
	level, err := logging.ParseLevel(f.logLevel)
	if err != nil {
		return usageErr("%v", err)
	}
	if f.quiet {
		level = 8 // slog.LevelError
	}
	logger, err := logging.NewConsole(stderr, f.logFormat, level)
	if err != nil {
		return usageErr("%v", err)
	}

	doc, issues := loadDocument(f.configPath)
	var overrides []string
	if doc != nil {
		overrides, err = applyOverrides(cmd, doc.Config, f)
		if err != nil {
			return usageErr("%v", err)
		}
		if len(overrides) > 0 {
			issues = append(issues, doc.Config.Validate().Errors()...)
		}
	}
	if issues.HasErrors() {
		return configFailure(f, doc, issues, stdout, stderr)
	}
	for _, is := range issues {
		logger.Warn("configuration", "issue", is.String())
	}

	cat, err := catalog.Builtin()
	if err != nil {
		return &exitError{code: result.ExitFailed, err: err}
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)
	go func() {
		s, ok := <-sigs
		if !ok {
			return
		}
		logger.Warn("signal received; finishing the archive (send again to abort immediately)", "signal", s.String())
		cancel(collector.ErrInterrupted)
		if _, ok := <-sigs; ok {
			fmt.Fprintln(stderr, "relic: aborted; the incomplete archive is left as *.partial")
			os.Exit(result.ExitCanceled)
		}
	}()

	res := collector.Run(ctx, collector.Options{
		Doc:       doc,
		Catalog:   cat,
		Issues:    issues,
		DryRun:    f.dryRun,
		Overrides: overrides,
		Logger:    logger,
	})
	if res.ExitCode == result.ExitUsage {
		printIssues(stderr, res.Issues)
	}
	if err := emitResult(res, f, stdout, stderr); err != nil {
		return err
	}
	return silentExit(res.ExitCode)
}

func loadDocument(path string) (*config.Document, config.Issues) {
	if path == "" {
		c := config.Default()
		c.Collect.Profile = catalog.DefaultProfile
		return config.FromConfig(c, "builtin"), nil
	}
	return config.LoadFile(path)
}

func applyOverrides(cmd *cobra.Command, c *config.Config, f *collectFlags) ([]string, error) {
	var out []string
	changed := cmd.Flags().Changed
	if changed("profile") {
		c.Collect.Profile = f.profile
		out = append(out, "profile="+f.profile)
	}
	if len(f.artifacts) > 0 {
		c.Collect.Artifacts = append(c.Collect.Artifacts, f.artifacts...)
		out = append(out, "artifact+="+strings.Join(f.artifacts, ","))
	}
	if len(f.excludes) > 0 {
		c.Collect.ExcludeArtifacts = append(c.Collect.ExcludeArtifacts, f.excludes...)
		out = append(out, "exclude_artifact+="+strings.Join(f.excludes, ","))
	}
	if changed("output") {
		// A directory keeps the file name pattern of the configuration.
		if isDirArg(f.output) {
			name := filepath.Base(c.Output.Path)
			if c.Output.Path == "" || isDirArg(c.Output.Path) {
				name = config.DefaultOutputName
			}
			c.Output.Path = filepath.Join(f.output, name)
		} else {
			c.Output.Path = f.output
		}
		out = append(out, "output="+f.output)
	}
	if changed("overwrite") {
		c.Output.Overwrite = f.overwrite
		out = append(out, fmt.Sprintf("overwrite=%v", f.overwrite))
	}
	if changed("method") {
		c.Collect.Method = f.method
		out = append(out, "method="+f.method)
	}
	if changed("max-duration") {
		d, err := parseDuration(f.maxDuration)
		if err != nil {
			return nil, fmt.Errorf("--max-duration: %v", err)
		}
		c.Limits.MaxDuration = config.Duration(d)
		out = append(out, "max_duration="+f.maxDuration)
	}
	for _, s := range []struct {
		flag string
		val  string
		dst  *config.Size
		name string
	}{
		{"max-total-size", f.maxTotalSize, &c.Limits.MaxTotalSize, "max_total_size"},
		{"max-file-size", f.maxFileSize, &c.Limits.MaxFileSize, "max_file_size"},
	} {
		if changed(s.flag) {
			v, err := config.ParseSize(s.val)
			if err != nil {
				return nil, fmt.Errorf("--%s: %v", s.flag, err)
			}
			*s.dst = v
			out = append(out, s.name+"="+s.val)
		}
	}
	return out, nil
}

func isDirArg(p string) bool {
	if strings.HasSuffix(p, "/") || strings.HasSuffix(p, string(os.PathSeparator)) {
		return true
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func parseDuration(s string) (time.Duration, error) {
	if s == "0" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

// configFailure reports configuration errors. With --json a failed result is
// still printed so that automation always receives a result object.
func configFailure(f *collectFlags, doc *config.Document, issues config.Issues, stdout, stderr io.Writer) error {
	printIssues(stderr, issues)
	now := time.Now().UTC()
	res := &result.Result{
		SchemaVersion: result.SchemaVersion,
		Tool:          version.Tool(),
		Status:        result.StatusFailed,
		ExitCode:      result.ExitUsage,
		StatusReason:  "invalid configuration",
		Host:          hostinfo.Host(),
		StartedAt:     now,
		FinishedAt:    now,
		Issues:        issues,
		Artifacts:     []result.ArtifactResult{},
	}
	if doc != nil {
		res.Config = result.ConfigInfo{Source: doc.Source, SHA256: doc.SHA256}
		res.Job = doc.Config.Job
	} else if f.configPath != "" {
		res.Config = result.ConfigInfo{Source: "file:" + f.configPath}
	}
	if err := emitResult(res, f, stdout, stderr); err != nil {
		return err
	}
	return silentExit(result.ExitUsage)
}

func printIssues(w io.Writer, issues config.Issues) {
	for _, is := range issues {
		fmt.Fprintln(w, is.String())
	}
}

func emitResult(res *result.Result, f *collectFlags, stdout, stderr io.Writer) error {
	if f.statusFile != "" {
		if err := writeStatusFile(f.statusFile, res); err != nil {
			fmt.Fprintf(stderr, "relic: cannot write status file: %v\n", err)
			if res.ExitCode == result.ExitComplete {
				res.ExitCode = result.ExitPartial
			}
		}
	}
	if f.asJSON {
		return writeJSON(stdout, res)
	}
	if !f.quiet {
		printSummary(stderr, res)
	}
	return nil
}

// writeStatusFile writes atomically (temp file + rename).
func writeStatusFile(path string, res *result.Result) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".relic-status-*.json")
	if err != nil {
		return err
	}
	if err := writeJSON(tmp, res); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func printSummary(w io.Writer, r *result.Result) {
	s := r.Summary
	fmt.Fprintf(w, "\nRelic %s: collection %s", r.Tool.Version, strings.ToUpper(string(r.Status)))
	if r.DryRun {
		fmt.Fprint(w, " (dry run)")
	}
	fmt.Fprintf(w, " in %.1fs\n", r.DurationSec)
	if r.StatusReason != "" {
		fmt.Fprintf(w, "  reason:    %s\n", r.StatusReason)
	}
	if r.Output != nil {
		fmt.Fprintf(w, "  archive:   %s (%s, sha256 %s)\n", r.Output.Path, humanBytes(r.Output.Size), r.Output.SHA256)
	}
	fmt.Fprintf(w, "  artifacts: %d selected\n", s.ArtifactsSelected)
	verb := "collected"
	if r.DryRun {
		verb = "would be collected"
	}
	fmt.Fprintf(w, "  files:     %d %s (%s), %d failed, %d skipped, %d truncated, %d duplicates\n",
		s.FilesCollected, verb, humanBytes(s.BytesCollected), s.FilesFailed, s.FilesSkipped, s.FilesTruncated, s.FilesDuplicate)
	if s.EnumerationErrors > 0 {
		fmt.Fprintf(w, "  enumeration errors: %d (see artifact errors in the manifest)\n", s.EnumerationErrors)
	}
	for _, l := range r.Limits {
		fmt.Fprintf(w, "  limit:     %s\n", l.Message)
	}
	for _, e := range r.Errors {
		fmt.Fprintf(w, "  error:     %s\n", e)
	}
	fmt.Fprintf(w, "  exit code: %d\n", r.ExitCode)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
