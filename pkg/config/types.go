// Package config defines the Relic collection configuration (schema version 1),
// its defaults, strict YAML parsing and validation.
//
// The package is public so that other Nextron components (e.g. ASGARD) can
// generate and validate configurations with the exact same types.
package config

import "time"

// CurrentVersion is the configuration schema version understood by this build.
const CurrentVersion = 1

// Config is the root of a Relic configuration file.
type Config struct {
	// Version of the configuration schema. Required. Must be 1.
	Version int `yaml:"version" json:"version"`
	// Job carries optional metadata that is copied verbatim into the result.
	Job Job `yaml:"job,omitempty" json:"job,omitempty"`
	// Collect selects what to collect and how.
	Collect Collect `yaml:"collect" json:"collect"`
	// Limits bounds resource usage and collection size.
	Limits Limits `yaml:"limits" json:"limits"`
	// Output controls the archive that is written.
	Output Output `yaml:"output" json:"output"`
}

// Job is free-form metadata about the collection job (e.g. the ASGARD task).
type Job struct {
	ID     string            `yaml:"id,omitempty" json:"id,omitempty"`
	Name   string            `yaml:"name,omitempty" json:"name,omitempty"`
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

// Collect selects artifacts and defines how files are accessed.
type Collect struct {
	// Profile is the name of a built-in profile (e.g. minimal, standard, full).
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	// Artifacts adds artifacts by name. Shell-style wildcards are allowed
	// (e.g. "windows.registry.*").
	Artifacts []string `yaml:"artifacts,omitempty" json:"artifacts,omitempty"`
	// ExcludeArtifacts removes artifacts (names or wildcards) from the selection.
	ExcludeArtifacts []string `yaml:"exclude_artifacts,omitempty" json:"exclude_artifacts,omitempty"`
	// ExcludePaths are path patterns that are never collected.
	ExcludePaths []string `yaml:"exclude_paths,omitempty" json:"exclude_paths,omitempty"`
	// CustomArtifacts are additional artifact definitions. They are always
	// collected unless excluded.
	CustomArtifacts []Artifact `yaml:"custom_artifacts,omitempty" json:"custom_artifacts,omitempty"`
	// Method is the default file access method: auto, os or raw.
	Method string `yaml:"method,omitempty" json:"method,omitempty"`
	// Hashes lists hash algorithms computed for each file: md5, sha1, sha256, sha512.
	Hashes []string `yaml:"hashes,omitempty" json:"hashes,omitempty"`
	// FollowSymlinks makes wildcard matches follow symbolic links. Literal
	// paths are always followed.
	FollowSymlinks bool `yaml:"follow_symlinks,omitempty" json:"follow_symlinks,omitempty"`
	// MaxDepth limits recursion of "**" patterns.
	MaxDepth int `yaml:"max_depth,omitempty" json:"max_depth,omitempty"`
}

// Artifact is a named, declarative description of forensic data to collect.
// Built-in catalog entries and custom artifacts share this format.
type Artifact struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Category    string   `yaml:"category,omitempty" json:"category,omitempty"`
	References  []string `yaml:"references,omitempty" json:"references,omitempty"`
	Sources     []Source `yaml:"sources" json:"sources"`
}

// Source describes where an artifact is found on a given set of platforms.
type Source struct {
	// Type of the source. Only "file" is supported in schema version 1.
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	// OS restricts the source to platforms: windows, linux, darwin (alias macos).
	// Empty means all platforms.
	OS []string `yaml:"os,omitempty" json:"os,omitempty"`
	// Paths are absolute path patterns (see docs/configuration.md).
	Paths []string `yaml:"paths" json:"paths"`
	// Exclude are path patterns removed from the matches of this source.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	// Method overrides the access method for this source: auto, os or raw.
	Method string `yaml:"method,omitempty" json:"method,omitempty"`
	// TrimSparse skips a leading sparse region when reading raw NTFS streams
	// (used for $UsnJrnl:$J). The skipped offset is recorded in the manifest.
	TrimSparse bool `yaml:"trim_sparse,omitempty" json:"trim_sparse,omitempty"`
}

// Limits bounds what a collection may consume. A value of 0 disables a limit.
type Limits struct {
	MaxFileSize  Size     `yaml:"max_file_size" json:"max_file_size"`
	Oversize     string   `yaml:"oversize" json:"oversize"`
	MaxTotalSize Size     `yaml:"max_total_size" json:"max_total_size"`
	MaxFiles     int      `yaml:"max_files" json:"max_files"`
	MaxDuration  Duration `yaml:"max_duration" json:"max_duration"`
	MinFreeSpace Size     `yaml:"min_free_space" json:"min_free_space"`
	MaxMemory    Size     `yaml:"max_memory" json:"max_memory"`
	ReadRate     Size     `yaml:"read_rate" json:"read_rate"`
	Priority     string   `yaml:"priority" json:"priority"`
}

// Output controls the archive.
type Output struct {
	// Path of the archive. Placeholders: {hostname}, {timestamp}, {job_id}.
	// If the path is an existing directory or ends with a path separator, a
	// default file name is created inside it.
	Path             string `yaml:"path" json:"path"`
	Compression      string `yaml:"compression" json:"compression"`
	CompressionLevel int    `yaml:"compression_level" json:"compression_level"`
	Overwrite        bool   `yaml:"overwrite" json:"overwrite"`
}

// Enumerations.
const (
	MethodAuto = "auto"
	MethodOS   = "os"
	MethodRaw  = "raw"

	OversizeSkip     = "skip"
	OversizeTruncate = "truncate"

	PriorityLow    = "low"
	PriorityNormal = "normal"

	CompressionAuto    = "auto"
	CompressionDeflate = "deflate"
	CompressionStore   = "store"

	SourceTypeFile = "file"

	DefaultOutputName = "relic_{hostname}_{timestamp}.zip"
)

var (
	ValidMethods      = []string{MethodAuto, MethodOS, MethodRaw}
	ValidOversize     = []string{OversizeSkip, OversizeTruncate}
	ValidPriorities   = []string{PriorityLow, PriorityNormal}
	ValidCompression  = []string{CompressionAuto, CompressionDeflate, CompressionStore}
	ValidHashes       = []string{"md5", "sha1", "sha256", "sha512"}
	ValidSourceTypes  = []string{SourceTypeFile}
	ValidOS           = []string{"windows", "linux", "darwin"}
	OutputPlaceholder = []string{"{hostname}", "{timestamp}", "{job_id}"}
)

// Default returns the configuration used when a value is not specified.
func Default() *Config {
	return &Config{
		Version: CurrentVersion,
		Collect: Collect{
			Method:   MethodAuto,
			Hashes:   []string{"sha256"},
			MaxDepth: 32,
		},
		Limits: Limits{
			MaxFileSize:  4 * GiB,
			Oversize:     OversizeSkip,
			MaxTotalSize: 32 * GiB,
			MaxDuration:  Duration(4 * time.Hour),
			MinFreeSpace: 1 * GiB,
			MaxMemory:    512 * MiB,
			Priority:     PriorityLow,
		},
		Output: Output{
			Path:             DefaultOutputName,
			Compression:      CompressionAuto,
			CompressionLevel: 5,
		},
	}
}

// applyDefaults fills empty values that have no meaningful zero value.
func (c *Config) applyDefaults() {
	d := Default()
	if c.Collect.Method == "" {
		c.Collect.Method = d.Collect.Method
	}
	if len(c.Collect.Hashes) == 0 {
		c.Collect.Hashes = d.Collect.Hashes
	}
	if c.Collect.MaxDepth == 0 {
		c.Collect.MaxDepth = d.Collect.MaxDepth
	}
	if c.Limits.Oversize == "" {
		c.Limits.Oversize = d.Limits.Oversize
	}
	if c.Limits.Priority == "" {
		c.Limits.Priority = d.Limits.Priority
	}
	if c.Output.Path == "" {
		c.Output.Path = d.Output.Path
	}
	if c.Output.Compression == "" {
		c.Output.Compression = d.Output.Compression
	}
	if c.Output.CompressionLevel == 0 {
		c.Output.CompressionLevel = d.Output.CompressionLevel
	}
	for i := range c.Collect.CustomArtifacts {
		for j := range c.Collect.CustomArtifacts[i].Sources {
			s := &c.Collect.CustomArtifacts[i].Sources[j]
			if s.Type == "" {
				s.Type = SourceTypeFile
			}
		}
	}
}

// NormalizeOS maps aliases to Go GOOS names. Unknown values are returned lowercased.
func NormalizeOS(s string) string {
	switch s {
	case "macos", "osx", "mac", "macOS", "darwin":
		return "darwin"
	case "Windows", "windows":
		return "windows"
	case "Linux", "linux":
		return "linux"
	}
	return s
}

// AppliesTo reports whether the source applies to the given GOOS.
func (s Source) AppliesTo(goos string) bool {
	if len(s.OS) == 0 {
		return true
	}
	for _, o := range s.OS {
		if NormalizeOS(o) == goos {
			return true
		}
	}
	return false
}

// AppliesTo reports whether any source of the artifact applies to goos.
func (a Artifact) AppliesTo(goos string) bool {
	for _, s := range a.Sources {
		if s.AppliesTo(goos) {
			return true
		}
	}
	return false
}
