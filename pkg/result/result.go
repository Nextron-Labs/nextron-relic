// Package result defines Relic's machine-readable output: the collection
// result (printed with --json, written with --status-file and stored as
// manifest.json in the archive) and the per-file records (files.jsonl).
//
// The JSON field names are a stable interface. Fields are only added, never
// renamed or removed, within a SchemaVersion.
package result

import (
	"time"

	"github.com/Nextron-Labs/nextron-relic/pkg/config"
)

// SchemaVersion of the result and file record format.
const SchemaVersion = 1

// Status is the overall outcome of a run.
type Status string

const (
	// StatusComplete: every selected artifact was processed and every
	// matching file was collected.
	StatusComplete Status = "complete"
	// StatusPartial: the archive is usable but something is missing: read
	// or enumeration errors, files skipped or truncated by limits, or a limit
	// (time, total size, file count, disk space) ended the collection early.
	StatusPartial Status = "partial"
	// StatusFailed: no usable archive was produced (invalid configuration,
	// output not writable, fatal I/O error, or nothing could be read).
	StatusFailed Status = "failed"
	// StatusCanceled: the run was interrupted by a signal; the archive is
	// finalized with everything collected up to that point.
	StatusCanceled Status = "canceled"
)

// Exit codes. They are part of the command line interface contract.
const (
	ExitComplete = 0
	ExitPartial  = 1
	ExitFailed   = 2
	ExitCanceled = 3
	ExitUsage    = 4 // invalid command line or configuration; nothing collected
)

// ExitCode maps a status to the process exit code.
func (s Status) ExitCode() int {
	switch s {
	case StatusComplete:
		return ExitComplete
	case StatusPartial:
		return ExitPartial
	case StatusCanceled:
		return ExitCanceled
	}
	return ExitFailed
}

// Result describes one collection run.
type Result struct {
	SchemaVersion int    `json:"schema_version"`
	Tool          Tool   `json:"tool"`
	Status        Status `json:"status"`
	ExitCode      int    `json:"exit_code"`
	// StatusReason is a short human readable explanation of the status.
	StatusReason string     `json:"status_reason,omitempty"`
	DryRun       bool       `json:"dry_run,omitempty"`
	Job          config.Job `json:"job,omitempty"`
	Host         Host       `json:"host"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   time.Time  `json:"finished_at"`
	DurationSec  float64    `json:"duration_seconds"`
	Config       ConfigInfo `json:"config"`
	Output       *Output    `json:"output,omitempty"`
	Summary      Summary    `json:"summary"`
	// Limits lists limits that were reached.
	Limits    []LimitEvent     `json:"limits,omitempty"`
	Artifacts []ArtifactResult `json:"artifacts"`
	// Issues are configuration warnings and errors.
	Issues []config.Issue `json:"issues,omitempty"`
	// Errors are run-level errors (not tied to a single file).
	Errors []string `json:"errors,omitempty"`
}

// Tool identifies the collector build.
type Tool struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	GoVersion string `json:"go_version"`
}

// Host describes the system the collection ran on.
type Host struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	OSVersion string `json:"os_version,omitempty"`
	Arch      string `json:"arch"`
	User      string `json:"user,omitempty"`
	// Elevated is true for root / an elevated administrator / LocalSystem.
	Elevated bool   `json:"elevated"`
	TimeZone string `json:"time_zone,omitempty"`
}

// ConfigInfo identifies the configuration that was used.
type ConfigInfo struct {
	Source  string `json:"source"`
	SHA256  string `json:"sha256,omitempty"`
	Profile string `json:"profile,omitempty"`
	// Overrides lists command line flags that changed the configuration.
	Overrides []string `json:"overrides,omitempty"`
}

// Output describes the archive.
type Output struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	// Size and SHA256 of the finished archive. They are not available inside
	// the archive's own manifest.json.
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

// Summary aggregates counters over all artifacts.
type Summary struct {
	ArtifactsSelected int `json:"artifacts_selected"`
	FilesMatched      int `json:"files_matched"`
	FilesCollected    int `json:"files_collected"`
	FilesTruncated    int `json:"files_truncated"`
	FilesSkipped      int `json:"files_skipped"`
	FilesFailed       int `json:"files_failed"`
	FilesDuplicate    int `json:"files_duplicate"`
	// BytesCollected is the number of source bytes read into the archive.
	BytesCollected int64 `json:"bytes_collected"`
	// EnumerationErrors counts directories or paths that could not be listed.
	EnumerationErrors int `json:"enumeration_errors"`
}

// ArtifactStatus is the outcome per artifact.
type ArtifactStatus string

const (
	ArtifactCollected     ArtifactStatus = "collected"      // all matches collected
	ArtifactPartial       ArtifactStatus = "partial"        // some matches failed/skipped/truncated or enumeration errors
	ArtifactFailed        ArtifactStatus = "failed"         // matches exist but none could be collected
	ArtifactNotFound      ArtifactStatus = "not_found"      // no file matched
	ArtifactSkipped       ArtifactStatus = "skipped"        // not processed because a limit or cancellation stopped the run
	ArtifactNotApplicable ArtifactStatus = "not_applicable" // explicitly requested, but has no source for this OS
)

// ArtifactResult summarizes one artifact.
type ArtifactResult struct {
	Name           string         `json:"name"`
	Status         ArtifactStatus `json:"status"`
	Custom         bool           `json:"custom,omitempty"`
	Reason         string         `json:"selected_by,omitempty"`
	FilesMatched   int            `json:"files_matched"`
	FilesCollected int            `json:"files_collected"`
	FilesTruncated int            `json:"files_truncated,omitempty"`
	FilesSkipped   int            `json:"files_skipped,omitempty"`
	FilesFailed    int            `json:"files_failed,omitempty"`
	FilesDuplicate int            `json:"files_duplicate,omitempty"`
	BytesCollected int64          `json:"bytes_collected"`
	// Errors holds enumeration and pattern errors (bounded).
	Errors        []string `json:"errors,omitempty"`
	ErrorsDropped int      `json:"errors_dropped,omitempty"`
}

// LimitEvent records a limit that was reached.
type LimitEvent struct {
	Limit   string `json:"limit"`
	Value   int64  `json:"value"`
	Message string `json:"message"`
}

// FileStatus is the outcome per file.
type FileStatus string

const (
	FileCollected FileStatus = "collected"
	// FileTruncated: only the first max_file_size bytes were stored.
	FileTruncated FileStatus = "truncated"
	// FileSkipped: deliberately not collected (limit, symlink, special file,
	// excluded). See Reason.
	FileSkipped FileStatus = "skipped"
	// FileFailed: could not be read. If ArchivePath is set, the archive entry
	// holds the BytesStored bytes read before the error.
	FileFailed FileStatus = "failed"
	// FileAborted: reading stopped because the run was canceled or a limit
	// was reached; the archive entry is incomplete.
	FileAborted FileStatus = "aborted"
)

// FileRecord is one line of files.jsonl.
type FileRecord struct {
	Artifact    string     `json:"artifact"`
	Pattern     string     `json:"pattern,omitempty"`
	SourcePath  string     `json:"source_path"`
	ArchivePath string     `json:"archive_path,omitempty"`
	Status      FileStatus `json:"status"`
	Reason      string     `json:"reason,omitempty"`
	// Method is the access method used: "os" or "ntfs_raw".
	Method string `json:"method,omitempty"`
	// FallbackReason is the OS error that caused a fall back to raw access.
	FallbackReason string `json:"fallback_reason,omitempty"`
	// Size is the size reported at enumeration time.
	Size int64 `json:"size"`
	// BytesStored is the number of bytes written to the archive.
	BytesStored int64 `json:"bytes_stored"`
	// DataOffset is the offset in the source of the first stored byte (non-
	// zero when a leading sparse region was skipped, e.g. $UsnJrnl:$J).
	DataOffset int64             `json:"data_offset,omitempty"`
	Hashes     map[string]string `json:"hashes,omitempty"`
	// ChangedDuringRead is set when size or modification time changed while
	// the file was read (live system).
	ChangedDuringRead bool      `json:"changed_during_read,omitempty"`
	SymlinkTarget     string    `json:"symlink_target,omitempty"`
	Mode              string    `json:"mode,omitempty"`
	UID               *int64    `json:"uid,omitempty"`
	GID               *int64    `json:"gid,omitempty"`
	Inode             string    `json:"inode,omitempty"`
	Times             Times     `json:"times"`
	CollectedAt       time.Time `json:"collected_at"`
}

// Times are the source timestamps (UTC). NTFS times come from
// $STANDARD_INFORMATION; FNCreated from $FILE_NAME (raw access only).
type Times struct {
	Modified  *time.Time `json:"modified,omitempty"`
	Accessed  *time.Time `json:"accessed,omitempty"`
	Changed   *time.Time `json:"changed,omitempty"`
	Created   *time.Time `json:"created,omitempty"`
	FNCreated *time.Time `json:"fn_created,omitempty"`
}
