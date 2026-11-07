// Package collector runs a collection: it resolves the selected artifacts,
// reads every matching file with the configured access method, streams it
// into the archive while hashing, enforces limits and produces the result.
package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Nextron-Labs/nextron-relic/internal/archive"
	"github.com/Nextron-Labs/nextron-relic/internal/catalog"
	"github.com/Nextron-Labs/nextron-relic/internal/hostinfo"
	"github.com/Nextron-Labs/nextron-relic/internal/logging"
	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
	"github.com/Nextron-Labs/nextron-relic/internal/source"
	"github.com/Nextron-Labs/nextron-relic/internal/version"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

// ErrInterrupted is used as cancellation cause when the user or the agent
// stops the collection (SIGINT, SIGTERM, Ctrl+C, Ctrl+Break).
var ErrInterrupted = errors.New("interrupted")

var errTimeLimit = errors.New("max_duration reached")

// DefaultProgressInterval is used when Options.ProgressInterval is 0.
const DefaultProgressInterval = 15 * time.Second

// Archive entry names of the metadata files.
const (
	ManifestName        = "manifest.json"
	FilesName           = "files.jsonl"
	LogName             = "relic.log"
	ConfigName          = "config.yaml"
	ConfigEffectiveName = "config.effective.yaml"
)

// Options configure a run.
type Options struct {
	Doc       *config.Document
	Catalog   *catalog.Catalog
	Issues    config.Issues // parser warnings to include in the result
	DryRun    bool
	Overrides []string
	Logger    *slog.Logger
	// ArchiveLogLevel is the level of relic.log in the archive. It is never
	// higher than info, so the archive always holds the complete run log;
	// slog.LevelDebug adds per-file decisions.
	ArchiveLogLevel slog.Level
	// ProgressInterval is the minimum time between progress log messages.
	// 0 uses DefaultProgressInterval; a negative value disables them.
	ProgressInterval time.Duration

	// Optional overrides for tests.
	Accessor *source.Accessor
	Expander *pathspec.Expander
	Host     *result.Host
	// GOOS overrides the target platform conventions (path style, catalog
	// selection). Only for tests: Windows patterns against an NTFS image.
	GOOS string
	// SkipPriority leaves process priority and memory limit untouched.
	SkipPriority bool
}

type run struct {
	opts  Options
	cfg   *config.Config
	goos  string
	style pathspec.Style
	log   *slog.Logger
	acc   *source.Accessor
	exp   *pathspec.Expander
	res   *result.Result

	arch    *archive.Writer
	outPath string
	outDir  string
	recFile *os.File
	recBuf  *bufio.Writer
	recEnc  *json.Encoder
	logFile *os.File
	logBuf  *bufio.Writer

	globalExcl []*pathspec.Pattern
	seen       map[[16]byte]struct{}
	buf        []byte
	thr        throttle

	total        int64
	limitSkips   int
	failLogged   int
	canceled     bool
	stopped      string // name of the limit that stopped the collection
	fatal        error
	lastProgress time.Time
	progressIvl  time.Duration
	step, steps  int // current artifact (1-based) and number of artifacts
	limitsHit    map[string]bool
}

// Run executes a collection and returns its result. It never panics on
// collection errors; all problems are reflected in the result.
func Run(ctx context.Context, o Options) *result.Result {
	r := &run{
		opts:      o,
		cfg:       o.Doc.Config,
		goos:      runtime.GOOS,
		log:       o.Logger,
		acc:       o.Accessor,
		exp:       o.Expander,
		seen:      map[[16]byte]struct{}{},
		buf:       make([]byte, 1<<20),
		limitsHit: map[string]bool{},
	}
	if r.log == nil {
		r.log = logging.Discard()
	}
	if o.GOOS != "" {
		r.goos = o.GOOS
	}
	r.progressIvl = o.ProgressInterval
	if r.progressIvl == 0 {
		r.progressIvl = DefaultProgressInterval
	}
	r.style = pathspec.StyleFor(r.goos)
	host := hostinfo.Host()
	if o.Host != nil {
		host = *o.Host
	}
	now := time.Now().UTC()
	r.res = &result.Result{
		SchemaVersion: result.SchemaVersion,
		Tool:          version.Tool(),
		DryRun:        o.DryRun,
		Job:           r.cfg.Job,
		Host:          host,
		StartedAt:     now,
		Config: result.ConfigInfo{
			Source:    o.Doc.Source,
			SHA256:    o.Doc.SHA256,
			Profile:   r.cfg.Collect.Profile,
			Overrides: o.Overrides,
		},
		Issues:    o.Issues,
		Artifacts: []result.ArtifactResult{},
	}
	r.execute(ctx)
	return r.res
}

func (r *run) execute(parent context.Context) {
	sel, issues := r.opts.Catalog.Select(r.cfg.Collect, r.goos)
	r.res.Issues = append(r.res.Issues, issues...)
	if issues.HasErrors() {
		r.finishEarly(result.StatusFailed, result.ExitUsage, "invalid configuration")
		return
	}
	r.res.Summary.ArtifactsSelected = len(sel.Artifacts)

	if r.acc == nil {
		r.acc = source.New()
	}
	defer r.acc.Close()
	if r.exp == nil {
		r.exp = hostinfo.Expander()
	}
	r.compileGlobalExcludes()

	if !r.opts.DryRun {
		if err := r.openOutput(); err != nil {
			r.res.Errors = append(r.res.Errors, err.Error())
			r.finishEarly(result.StatusFailed, result.ExitFailed, "cannot create output: "+err.Error())
			return
		}
	}

	if !r.opts.SkipPriority {
		if r.cfg.Limits.Priority == config.PriorityLow {
			if err := hostinfo.LowerPriority(); err != nil {
				r.log.Warn("could not lower process priority", "error", err)
			}
		}
		if r.cfg.Limits.MaxMemory > 0 {
			debug.SetMemoryLimit(int64(r.cfg.Limits.MaxMemory))
		}
	}

	ctx := parent
	if d := r.cfg.Limits.MaxDuration.D(); d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(parent, d, errTimeLimit)
		defer cancel()
	}
	r.thr.start(float64(r.cfg.Limits.ReadRate))
	r.lastProgress = time.Now()

	r.log.Info("collection started",
		"profile", r.cfg.Collect.Profile, "artifacts", len(sel.Artifacts),
		"output", r.outPath, "dry_run", r.opts.DryRun, "method", r.cfg.Collect.Method,
		"elevated", r.res.Host.Elevated, "user", r.res.Host.User)
	if !r.res.Host.Elevated {
		r.log.Warn("not running " + elevationHint(r.goos) + "; protected files and directories will be reported as unreadable")
	}

	r.steps = len(sel.Artifacts)
	for i, s := range sel.Artifacts {
		r.step = i + 1
		if r.halted(ctx) {
			r.res.Artifacts = append(r.res.Artifacts, result.ArtifactResult{
				Name: s.Artifact.Name, Status: result.ArtifactSkipped, Custom: s.Custom, Reason: s.Reason})
			continue
		}
		r.collectArtifact(ctx, s)
	}
	for _, n := range sel.NotApplicable {
		r.res.Artifacts = append(r.res.Artifacts, result.ArtifactResult{Name: n, Status: result.ArtifactNotApplicable, Reason: "config"})
		r.log.Warn("artifact has no source for this platform", "artifact", n, "os", r.goos)
	}
	r.halted(ctx) // record cause of a late cancellation
	r.finalize()
}

// halted reports whether the collection must not continue and records why.
func (r *run) halted(ctx context.Context) bool {
	if r.fatal != nil || r.canceled || r.stopped != "" {
		return true
	}
	if ctx.Err() == nil {
		return false
	}
	if errors.Is(context.Cause(ctx), errTimeLimit) {
		r.hitLimit("max_duration", int64(r.cfg.Limits.MaxDuration.D()/time.Second),
			fmt.Sprintf("time limit of %s reached; collection stopped", r.cfg.Limits.MaxDuration))
		r.stopped = "max_duration"
	} else {
		r.canceled = true
		r.log.Warn("collection interrupted; finalizing archive")
	}
	return true
}

func (r *run) hitLimit(name string, value int64, msg string) {
	if r.limitsHit[name] {
		return
	}
	r.limitsHit[name] = true
	r.res.Limits = append(r.res.Limits, result.LimitEvent{Limit: name, Value: value, Message: msg})
	r.log.Warn("limit reached", "limit", name, "value", value, "message", msg)
}

func (r *run) compileGlobalExcludes() {
	for _, p := range r.cfg.Collect.ExcludePaths {
		if pathspec.Check(p, r.style) != nil {
			continue // pattern for another platform
		}
		pats, err := pathspec.Compile(p, r.style, r.exp)
		if err != nil {
			r.res.Errors = append(r.res.Errors, fmt.Sprintf("exclude_paths %q: %v", p, err))
			continue
		}
		r.globalExcl = append(r.globalExcl, pats...)
	}
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// OutputPath expands placeholders and turns directories into a file path.
// The file name always ends in ".zip": a path that is neither a directory
// nor ends in ".zip" (case-insensitive) gets the extension appended, so an
// archive is never written under a name that extension-based tools ignore.
func OutputPath(p string, job config.Job, host string, t time.Time) (string, error) {
	out, _, err := outputPath(p, job, host, t)
	return out, err
}

// outputPath implements OutputPath and also reports whether ".zip" was appended.
func outputPath(p string, job config.Job, host string, t time.Time) (string, bool, error) {
	if p == "" {
		p = config.DefaultOutputName
	}
	if hasTrailingSeparator(p) {
		p = filepath.Join(p, config.DefaultOutputName)
	} else if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		p = filepath.Join(p, config.DefaultOutputName)
	}
	jobID := job.ID
	if jobID == "" {
		jobID = "nojob"
	}
	repl := strings.NewReplacer(
		"{hostname}", unsafeChars.ReplaceAllString(host, "_"),
		"{timestamp}", t.UTC().Format("20060102T150405Z"),
		"{job_id}", unsafeChars.ReplaceAllString(jobID, "_"),
	)
	p = repl.Replace(p)
	appended := false
	if !HasZipExt(p) {
		p += ".zip"
		appended = true
	}
	abs, err := filepath.Abs(p)
	return abs, appended, err
}

// HasZipExt reports whether the file name of p ends in ".zip" (any case).
func HasZipExt(p string) bool {
	return strings.EqualFold(filepath.Ext(p), ".zip")
}

// hasTrailingSeparator reports whether p ends with a path separator ("/" on every
// platform, also "\" on Windows).
func hasTrailingSeparator(p string) bool {
	return strings.HasSuffix(p, "/") || strings.HasSuffix(p, string(os.PathSeparator))
}

func (r *run) openOutput() error {
	p, appended, err := outputPath(r.cfg.Output.Path, r.cfg.Job, r.res.Host.Hostname, r.res.StartedAt)
	if err != nil {
		return err
	}
	r.outPath = p
	r.outDir = filepath.Dir(p)
	if err := os.MkdirAll(r.outDir, 0o700); err != nil {
		return err
	}
	if r.cfg.Limits.MinFreeSpace > 0 {
		if free, err := hostinfo.FreeSpace(r.outDir); err == nil && free < uint64(r.cfg.Limits.MinFreeSpace) {
			return fmt.Errorf("only %s free on the output volume, min_free_space is %s", config.Size(free), r.cfg.Limits.MinFreeSpace)
		}
	}
	a, err := archive.Create(p, r.cfg.Output.Compression, r.cfg.Output.CompressionLevel, r.cfg.Output.Overwrite)
	if err != nil {
		return err
	}
	r.arch = a
	if r.recFile, err = os.CreateTemp(r.outDir, ".relic-files-*.jsonl"); err != nil {
		a.Abort()
		return err
	}
	r.recBuf = bufio.NewWriterSize(r.recFile, 256<<10)
	r.recEnc = json.NewEncoder(r.recBuf)
	r.recEnc.SetEscapeHTML(false)
	if r.logFile, err = os.CreateTemp(r.outDir, ".relic-log-*.jsonl"); err != nil {
		a.Abort()
		return err
	}
	r.logBuf = bufio.NewWriterSize(r.logFile, 64<<10)
	r.log = logging.Tee(r.log, logging.NewFileHandler(r.logBuf, min(r.opts.ArchiveLogLevel, slog.LevelInfo)))
	if appended {
		r.log.Warn("output path has no .zip extension; appended it (end the path with a path separator to write into a directory)",
			"output", r.cfg.Output.Path, "archive", p)
	}
	return nil
}

func (r *run) removeTemp() {
	for _, f := range []*os.File{r.recFile, r.logFile} {
		if f != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}
}

// finishEarly ends a run before collection started.
func (r *run) finishEarly(st result.Status, code int, reason string) {
	r.res.Status = st
	r.res.ExitCode = code
	r.res.StatusReason = reason
	r.res.FinishedAt = time.Now().UTC()
	r.res.DurationSec = r.res.FinishedAt.Sub(r.res.StartedAt).Seconds()
}

func (r *run) finalize() {
	r.res.FinishedAt = time.Now().UTC()
	r.res.DurationSec = r.res.FinishedAt.Sub(r.res.StartedAt).Seconds()
	r.computeStatus()

	if r.arch == nil {
		r.log.Info("collection finished", "status", r.res.Status, "files", r.res.Summary.FilesCollected, "bytes", r.res.Summary.BytesCollected)
		return
	}
	defer r.removeTemp()
	if r.fatal != nil {
		r.arch.Abort()
		r.log.Error("archive discarded", "error", r.fatal)
		return
	}
	r.log.Info("collection finished", "status", r.res.Status, "reason", r.res.StatusReason,
		"files", r.res.Summary.FilesCollected, "bytes", r.res.Summary.BytesCollected,
		"duration_seconds", int(r.res.DurationSec))

	err := r.writeMetadata()
	if err == nil {
		var size int64
		var sum string
		size, sum, err = r.arch.Close()
		if err == nil {
			r.res.Output = &result.Output{Path: r.outPath, Format: "zip", Size: size, SHA256: sum}
			return
		}
	} else {
		r.arch.Abort()
	}
	r.fatal = fmt.Errorf("finalizing archive: %w", err)
	r.res.Errors = append(r.res.Errors, r.fatal.Error())
	r.computeStatus()
}

func (r *run) writeMetadata() error {
	if r.opts.Doc.Raw != nil {
		if err := r.arch.WriteFile(ConfigName, r.opts.Doc.Raw); err != nil {
			return err
		}
	}
	if eff, err := r.cfg.Marshal(); err == nil {
		if err := r.arch.WriteFile(ConfigEffectiveName, eff); err != nil {
			return err
		}
	}
	if err := r.recBuf.Flush(); err != nil {
		return err
	}
	if err := r.arch.CopyFile(FilesName, r.recFile.Name()); err != nil {
		return err
	}
	// The log is closed here; later messages only reach the console.
	if err := r.logBuf.Flush(); err != nil {
		return err
	}
	if err := r.arch.CopyFile(LogName, r.logFile.Name()); err != nil {
		return err
	}
	m := *r.res
	m.Output = &result.Output{Path: r.outPath, Format: "zip"}
	data, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return err
	}
	return r.arch.WriteFile(ManifestName, data)
}

func (r *run) computeStatus() {
	s := r.res.Summary
	collected := s.FilesCollected + s.FilesTruncated
	problems := []string{}
	if s.FilesFailed > 0 {
		problems = append(problems, fmt.Sprintf("%d file(s) failed", s.FilesFailed))
	}
	if s.FilesTruncated > 0 {
		problems = append(problems, fmt.Sprintf("%d file(s) truncated", s.FilesTruncated))
	}
	if r.limitSkips > 0 {
		problems = append(problems, fmt.Sprintf("%d file(s) skipped by limits", r.limitSkips))
	}
	if s.EnumerationErrors > 0 {
		problems = append(problems, fmt.Sprintf("%d enumeration error(s)", s.EnumerationErrors))
	}
	for _, l := range r.res.Limits {
		problems = append(problems, "limit "+l.Limit+" reached")
	}
	if n := len(r.res.Errors); n > 0 {
		problems = append(problems, fmt.Sprintf("%d error(s)", n))
	}
	var st result.Status
	reason := ""
	switch {
	case r.fatal != nil:
		st, reason = result.StatusFailed, r.fatal.Error()
	case r.canceled:
		st, reason = result.StatusCanceled, "interrupted by signal"
	case collected == 0 && s.FilesFailed > 0 && r.stopped == "" && len(r.res.Limits) == 0:
		st, reason = result.StatusFailed, "no file could be collected: "+strings.Join(problems, ", ")
	case len(problems) > 0:
		st, reason = result.StatusPartial, strings.Join(problems, ", ")
	default:
		st = result.StatusComplete
	}
	r.res.Status = st
	r.res.ExitCode = st.ExitCode()
	r.res.StatusReason = reason
}

// elevationHint names the privileges a complete collection needs.
func elevationHint(goos string) string {
	switch goos {
	case "windows":
		return "elevated (administrator or LocalSystem)"
	case "darwin":
		return "as root (and Full Disk Access is needed for TCC-protected locations)"
	}
	return "as root"
}

func keyHash(k string) [16]byte {
	h := fnv.New128a()
	h.Write([]byte(k))
	var out [16]byte
	copy(out[:], h.Sum(nil))
	return out
}
