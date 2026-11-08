package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Nextron-Labs/nextron-relic/internal/catalog"
	"github.com/Nextron-Labs/nextron-relic/internal/collector"
	"github.com/Nextron-Labs/nextron-relic/internal/hostinfo"
	"github.com/Nextron-Labs/nextron-relic/internal/logging"
	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
	"github.com/Nextron-Labs/nextron-relic/internal/version"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

type collectFlags struct {
	configPath   string
	profile      string
	artifacts    []string
	excludes     []string
	paths        []string
	excludePaths []string
	output       string
	overwrite    bool
	dryRun       bool
	asJSON       bool
	statusFile   string

	jobID   string
	jobName string
	labels  []string

	hashes         []string
	method         string
	followSymlinks bool

	maxDuration  string
	maxTotalSize string
	maxFileSize  string
	maxFiles     int
	minFreeSpace string
	readRate     string
	oversize     string
	priority     string

	compression      string
	compressionLevel int

	logLevel         string
	logFormat        string
	logFile          string
	progressInterval string
	verbose          bool
	quiet            bool
}

// PathsArtifact is the name of the custom artifact built from --path.
const PathsArtifact = "cli.paths"

func newCollectCmd(stdout, stderr io.Writer) *cobra.Command {
	f := &collectFlags{}
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect artifacts into a ZIP archive",
		Long: `Collect artifacts into a ZIP archive.

Without --config the built-in "standard" profile is collected, unless
--artifact or --path select something else: then only those are collected
(add --profile to combine them with a profile). Flags override the
corresponding configuration values and are recorded in the result.

The archive is written to <output>.partial and renamed when complete. It
contains files/ (collected data), manifest.json (result), files.jsonl (one
record per file), relic.log, config.yaml and config.effective.yaml.`,
		Example: `  relic collect
  relic collect --profile minimal --output D:\cases\ --job-id IR-2026-0042
  relic collect --config triage.yaml --json --status-file C:\Windows\Temp\relic-status.json
  relic collect --profile standard --artifact 'browser.*' --dry-run
  relic collect --artifact windows.eventlogs --path 'C:\inetpub\logs\**\*.log'
  relic collect --path '/opt/app/logs/**' --exclude-path '/opt/app/logs/**/*.gz' --read-rate 20MiB -v`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCollect(cmd, f, stdout, stderr)
		},
	}
	fl := cmd.Flags()
	fl.SortFlags = false
	// Selection
	fl.StringVarP(&f.configPath, "config", "c", "", "YAML configuration `file` ('-' reads from stdin)")
	fl.StringVarP(&f.profile, "profile", "p", "", "built-in profile `name`: minimal, standard or full (collect.profile)")
	fl.StringArrayVarP(&f.artifacts, "artifact", "a", nil, "add an artifact `name` or wildcard (repeatable)")
	fl.StringArrayVarP(&f.excludes, "exclude-artifact", "x", nil, "exclude an artifact `name` or wildcard (repeatable)")
	fl.StringArrayVar(&f.paths, "path", nil, "also collect files matching this path `pattern` (repeatable; artifact "+PathsArtifact+")")
	fl.StringArrayVar(&f.excludePaths, "exclude-path", nil, "never collect files matching this path `pattern` (repeatable)")
	// Output
	fl.StringVarP(&f.output, "output", "o", "", "archive `path`: a file (.zip is appended if missing) or a directory (existing, or ending with a separator); placeholders {hostname}, {timestamp}, {job_id}")
	fl.BoolVar(&f.overwrite, "overwrite", false, "overwrite an existing archive")
	fl.StringVar(&f.compression, "compression", "", "compression `mode`: auto, deflate or store (output.compression)")
	fl.IntVar(&f.compressionLevel, "compression-level", 0, "deflate `level` from 1 (fastest) to 9 (smallest) (output.compression_level)")
	fl.BoolVar(&f.dryRun, "dry-run", false, "resolve and list matching files without reading them or writing an archive")
	fl.BoolVar(&f.asJSON, "json", false, "print the result as JSON on stdout")
	fl.StringVar(&f.statusFile, "status-file", "", "also write the result JSON to this `file`")
	// Job metadata
	fl.StringVar(&f.jobID, "job-id", "", "case or task `id` (job.id; recorded in the result, usable as {job_id})")
	fl.StringVar(&f.jobName, "job-name", "", "job `name` (job.name)")
	fl.StringArrayVar(&f.labels, "label", nil, "job label as `key=value` (repeatable; job.labels)")
	// Collection
	fl.StringVar(&f.method, "method", "", "file access `method`: auto, os or raw (collect.method)")
	fl.StringSliceVar(&f.hashes, "hash", nil, "hash `algorithms`: md5, sha1, sha256, sha512, comma-separated (collect.hashes)")
	fl.BoolVar(&f.followSymlinks, "follow-symlinks", false, "follow symbolic links matched by wildcards (collect.follow_symlinks)")
	// Limits
	fl.StringVar(&f.maxDuration, "max-duration", "", "stop after this `duration`, e.g. 30m; 0 = unlimited (limits.max_duration)")
	fl.StringVar(&f.maxTotalSize, "max-total-size", "", "stop at this total `size`, e.g. 10GiB; 0 = unlimited (limits.max_total_size)")
	fl.StringVar(&f.maxFileSize, "max-file-size", "", "maximum `size` per file, e.g. 1GiB; 0 = unlimited (limits.max_file_size)")
	fl.StringVar(&f.oversize, "oversize", "", "handling `mode` for files above the maximum file size: skip or truncate (limits.oversize)")
	fl.IntVar(&f.maxFiles, "max-files", 0, "stop after this `number` of files; 0 = unlimited (limits.max_files)")
	fl.StringVar(&f.minFreeSpace, "min-free-space", "", "stop when the output volume has less than `size` free, e.g. 5GiB; 0 = off (limits.min_free_space)")
	fl.StringVar(&f.readRate, "read-rate", "", "maximum read `rate` in bytes per second, e.g. 20MiB; 0 = unthrottled (limits.read_rate)")
	fl.StringVar(&f.priority, "priority", "", "process `priority`: low or normal (limits.priority)")
	// Logging
	fl.StringVar(&f.logLevel, "log-level", "info", "log `level`: debug, info, warn or error; debug also applies to relic.log in the archive")
	fl.BoolVarP(&f.verbose, "verbose", "v", false, "same as --log-level debug")
	fl.BoolVarP(&f.quiet, "quiet", "q", false, "only log errors on the console (does not affect --log-file)")
	fl.StringVar(&f.logFormat, "log-format", "text", "console log `format` on stderr: text or json")
	fl.StringVar(&f.logFile, "log-file", "", "also write the log as JSON lines (UTC) to this `file`; appended if it exists")
	fl.StringVar(&f.progressInterval, "progress-interval", "15s", "minimum `duration` between progress messages (0 = none)")
	return cmd
}

func runCollect(cmd *cobra.Command, f *collectFlags, stdout, stderr io.Writer) error {
	level, err := logging.ParseLevel(f.logLevel)
	if err != nil {
		return usageErr("%v", err)
	}
	if f.verbose {
		if cmd.Flags().Changed("log-level") && level != slog.LevelDebug {
			return usageErr("--verbose conflicts with --log-level %s", f.logLevel)
		}
		level = slog.LevelDebug
	}
	consoleLevel := level
	if f.quiet {
		consoleLevel = slog.LevelError
	}
	logger, err := logging.NewConsole(stderr, f.logFormat, consoleLevel)
	if err != nil {
		return usageErr("%v", err)
	}
	progress, err := parseDuration(f.progressInterval)
	if err != nil || progress < 0 {
		return usageErr("--progress-interval: invalid duration %q (e.g. 30s, 5m, 0 = none)", f.progressInterval)
	}
	if progress == 0 {
		progress = -1 // disabled
	}
	if f.logFile != "" {
		lf, err := os.OpenFile(f.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return usageErr("--log-file: %v", err)
		}
		defer lf.Close()
		logger = logging.Tee(logger, logging.NewFileHandler(lf, level))
	}

	doc, issues := loadDocument(f.configPath)
	var overrides []string
	if doc != nil {
		overrides, err = applyOverrides(cmd, doc.Config, f)
		if err != nil {
			return usageErr("%v", err)
		}
		if len(overrides) > 0 && !issues.HasErrors() {
			issues = append(issues, doc.Config.Validate().Errors()...)
		}
	}
	if dropsDefaultProfile(cmd, f) && doc != nil && doc.Config.Collect.Profile == "" {
		logger.Info("no profile selected: collecting only the artifacts and paths given on the command line (add --profile " + catalog.DefaultProfile + " to include the default profile)")
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
		Doc:              doc,
		Catalog:          cat,
		Issues:           issues,
		DryRun:           f.dryRun,
		Overrides:        overrides,
		Logger:           logger,
		ArchiveLogLevel:  level,
		ProgressInterval: progress,
	})
	if res.ExitCode == result.ExitUsage {
		printIssues(stderr, res.Issues)
	}
	if err := emitResult(res, f, stdout, stderr); err != nil {
		return err
	}
	return silentExit(res.ExitCode)
}

// loadDocument loads the configuration file, or without a file the built-in
// defaults with the default profile. Their YAML form is archived as
// config.yaml, so every archive holds the configuration its flags changed.
func loadDocument(path string) (*config.Document, config.Issues) {
	if path == "" {
		c := config.Default()
		c.Collect.Profile = catalog.DefaultProfile
		doc, err := config.FromBuiltin(c)
		if err != nil {
			var is config.Issues
			is.Errorf("", "built-in configuration: %v", err)
			return nil, is
		}
		return doc, nil
	}
	return config.LoadFile(path)
}

// dropsDefaultProfile reports whether --artifact or --path select what to
// collect without a configuration file and without --profile: then only those
// are collected, not the default profile.
func dropsDefaultProfile(cmd *cobra.Command, f *collectFlags) bool {
	return f.configPath == "" && !cmd.Flags().Changed("profile") && (len(f.artifacts) > 0 || len(f.paths) > 0)
}

func applyOverrides(cmd *cobra.Command, c *config.Config, f *collectFlags) ([]string, error) {
	var out []string
	changed := cmd.Flags().Changed
	if dropsDefaultProfile(cmd, f) {
		c.Collect.Profile = ""
		out = append(out, "profile=")
	}
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
	if len(f.paths) > 0 {
		paths, err := cliPatterns("--path", f.paths)
		if err != nil {
			return nil, err
		}
		c.Collect.CustomArtifacts = append(c.Collect.CustomArtifacts, config.Artifact{
			Name:        PathsArtifact,
			Description: "Files given with --path on the command line.",
			Category:    "custom",
			Sources:     []config.Source{{Type: config.SourceTypeFile, OS: []string{runtime.GOOS}, Paths: paths}},
		})
		out = append(out, "path+="+strings.Join(paths, ","))
	}
	if len(f.excludePaths) > 0 {
		paths, err := cliPatterns("--exclude-path", f.excludePaths)
		if err != nil {
			return nil, err
		}
		c.Collect.ExcludePaths = append(c.Collect.ExcludePaths, paths...)
		out = append(out, "exclude_path+="+strings.Join(paths, ","))
	}
	if changed("job-id") {
		c.Job.ID = f.jobID
		out = append(out, "job_id="+f.jobID)
	}
	if changed("job-name") {
		c.Job.Name = f.jobName
		out = append(out, "job_name="+f.jobName)
	}
	for _, l := range f.labels {
		k, v, ok := strings.Cut(l, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("--label: expected KEY=VALUE, got %q", l)
		}
		if c.Job.Labels == nil {
			c.Job.Labels = map[string]string{}
		}
		c.Job.Labels[strings.TrimSpace(k)] = v
		out = append(out, "label+="+l)
	}
	if changed("hash") {
		var hs []string
		for _, h := range f.hashes {
			if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
				hs = append(hs, h)
			}
		}
		if len(hs) == 0 {
			return nil, fmt.Errorf("--hash: at least one algorithm is required (%s)", strings.Join(config.ValidHashes, ", "))
		}
		c.Collect.Hashes = hs
		out = append(out, "hashes="+strings.Join(hs, ","))
	}
	if changed("follow-symlinks") {
		c.Collect.FollowSymlinks = f.followSymlinks
		out = append(out, fmt.Sprintf("follow_symlinks=%v", f.followSymlinks))
	}
	if changed("compression") {
		c.Output.Compression = f.compression
		out = append(out, "compression="+f.compression)
	}
	if changed("compression-level") {
		c.Output.CompressionLevel = f.compressionLevel
		out = append(out, fmt.Sprintf("compression_level=%d", f.compressionLevel))
	}
	if changed("oversize") {
		c.Limits.Oversize = f.oversize
		out = append(out, "oversize="+f.oversize)
	}
	if changed("priority") {
		c.Limits.Priority = f.priority
		out = append(out, "priority="+f.priority)
	}
	if changed("max-files") {
		c.Limits.MaxFiles = f.maxFiles
		out = append(out, fmt.Sprintf("max_files=%d", f.maxFiles))
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
		{"min-free-space", f.minFreeSpace, &c.Limits.MinFreeSpace, "min_free_space"},
		{"read-rate", f.readRate, &c.Limits.ReadRate, "read_rate"},
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

// cliPatterns checks path patterns given on the command line for this
// platform. Relative paths are made absolute against the working directory.
func cliPatterns(flag string, in []string) ([]string, error) {
	style := pathspec.StyleFor(runtime.GOOS)
	out := make([]string, 0, len(in))
	for _, p := range in {
		if strings.TrimSpace(p) == "" {
			return nil, fmt.Errorf("%s: empty pattern", flag)
		}
		if !filepath.IsAbs(p) && !strings.HasPrefix(p, "%") {
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
		}
		if err := pathspec.Check(p, style); err != nil {
			return nil, fmt.Errorf("%s %q: %v", flag, p, err)
		}
		out = append(out, p)
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
