package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
)

var artifactNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

// Validate checks the configuration structurally (independent of the
// artifact catalog and of the host). Catalog dependent checks, such as
// unknown artifact names, are performed when the collection is planned.
func (c *Config) Validate() Issues {
	var is Issues
	if c.Version != CurrentVersion {
		is.Errorf("version", "unsupported configuration version %d (this build supports version %d)", c.Version, CurrentVersion)
	}

	// collect
	col := c.Collect
	if col.Profile == "" && len(col.Artifacts) == 0 && len(col.CustomArtifacts) == 0 {
		is.Errorf("collect", "nothing selected: set collect.profile, collect.artifacts or collect.custom_artifacts")
	}
	for i, a := range col.Artifacts {
		if strings.TrimSpace(a) == "" {
			is.Errorf(fmt.Sprintf("collect.artifacts[%d]", i), "empty artifact name")
		}
	}
	checkEnum(&is, "collect.method", col.Method, ValidMethods)
	seen := map[string]bool{}
	for i, h := range col.Hashes {
		p := fmt.Sprintf("collect.hashes[%d]", i)
		if !contains(ValidHashes, h) {
			is.Errorf(p, "unsupported hash algorithm %q (allowed: %s)", h, strings.Join(ValidHashes, ", "))
		}
		if seen[h] {
			is.Warnf(p, "duplicate hash algorithm %q", h)
		}
		seen[h] = true
	}
	if col.MaxDepth < 1 || col.MaxDepth > 256 {
		is.Errorf("collect.max_depth", "must be between 1 and 256, got %d", col.MaxDepth)
	}
	for i, p := range col.ExcludePaths {
		checkAnyStylePattern(&is, fmt.Sprintf("collect.exclude_paths[%d]", i), p)
	}
	names := map[string]int{}
	for i, a := range col.CustomArtifacts {
		ValidateArtifact(&is, fmt.Sprintf("collect.custom_artifacts[%d]", i), a)
		if prev, dup := names[a.Name]; dup && a.Name != "" {
			is.Errorf(fmt.Sprintf("collect.custom_artifacts[%d].name", i), "duplicate artifact name %q (also defined at custom_artifacts[%d])", a.Name, prev)
		}
		names[a.Name] = i
	}

	// limits
	l := c.Limits
	nonNeg := func(path string, v int64) {
		if v < 0 {
			is.Errorf(path, "must not be negative")
		}
	}
	nonNeg("limits.max_file_size", int64(l.MaxFileSize))
	nonNeg("limits.max_total_size", int64(l.MaxTotalSize))
	nonNeg("limits.max_files", int64(l.MaxFiles))
	nonNeg("limits.max_duration", int64(l.MaxDuration))
	nonNeg("limits.min_free_space", int64(l.MinFreeSpace))
	nonNeg("limits.max_memory", int64(l.MaxMemory))
	nonNeg("limits.read_rate", int64(l.ReadRate))
	checkEnum(&is, "limits.oversize", l.Oversize, ValidOversize)
	checkEnum(&is, "limits.priority", l.Priority, ValidPriorities)
	if l.MaxMemory > 0 && l.MaxMemory < 32*MiB {
		is.Warnf("limits.max_memory", "very low memory limit %s may slow down collection considerably", l.MaxMemory)
	}
	if l.ReadRate > 0 && l.ReadRate < 64*KiB {
		is.Warnf("limits.read_rate", "very low read rate %s/s", l.ReadRate)
	}

	// output
	o := c.Output
	checkEnum(&is, "output.compression", o.Compression, ValidCompression)
	if o.CompressionLevel < 1 || o.CompressionLevel > 9 {
		is.Errorf("output.compression_level", "must be between 1 (fastest) and 9 (smallest), got %d", o.CompressionLevel)
	}
	for _, ph := range placeholderRe.FindAllString(o.Path, -1) {
		if !contains(OutputPlaceholder, ph) {
			is.Errorf("output.path", "unknown placeholder %s (allowed: %s)", ph, strings.Join(OutputPlaceholder, ", "))
		}
	}
	return is
}

var placeholderRe = regexp.MustCompile(`\{[^{}]*\}`)

// ValidateArtifact checks an artifact definition. It is used for custom
// artifacts and for the built-in catalog.
func ValidateArtifact(is *Issues, path string, a Artifact) {
	if a.Name == "" {
		is.Errorf(path+".name", "is required")
	} else if !artifactNameRe.MatchString(a.Name) {
		is.Errorf(path+".name", "invalid name %q: use lower-case letters, digits, '.', '_' and '-' (e.g. custom.app_logs)", a.Name)
	}
	if len(a.Sources) == 0 {
		is.Errorf(path+".sources", "at least one source is required")
	}
	for j, s := range a.Sources {
		sp := fmt.Sprintf("%s.sources[%d]", path, j)
		if s.Type != "" && !contains(ValidSourceTypes, s.Type) {
			is.Errorf(sp+".type", "unsupported source type %q (supported in version %d: %s)", s.Type, CurrentVersion, strings.Join(ValidSourceTypes, ", "))
		}
		var styles []pathspec.Style
		hasWindows, hasPosix := false, false
		for k, o := range s.OS {
			n := NormalizeOS(o)
			if !contains(ValidOS, n) {
				is.Errorf(fmt.Sprintf("%s.os[%d]", sp, k), "unknown operating system %q (allowed: windows, linux, darwin/macos)", o)
				continue
			}
			if n == "windows" {
				hasWindows = true
			} else {
				hasPosix = true
			}
		}
		if hasWindows {
			styles = append(styles, pathspec.WindowsStyle)
		}
		if hasPosix {
			styles = append(styles, pathspec.PosixStyle)
		}
		if s.Method != "" {
			checkEnum(is, sp+".method", s.Method, ValidMethods)
			if s.Method == MethodRaw && (len(s.OS) == 0 || hasPosix) {
				is.Errorf(sp+".method", "method raw (NTFS) is only supported for sources restricted to os: [windows]")
			}
		}
		if s.TrimSparse && s.Method != MethodRaw {
			is.Warnf(sp+".trim_sparse", "only has an effect with method: raw")
		}
		if len(s.Paths) == 0 {
			is.Errorf(sp+".paths", "at least one path is required")
		}
		for k, p := range s.Paths {
			pp := fmt.Sprintf("%s.paths[%d]", sp, k)
			if len(styles) == 0 {
				checkAllStylesPattern(is, pp, p)
			} else {
				for _, st := range styles {
					if err := pathspec.Check(p, st); err != nil {
						is.Errorf(pp, "%v", err)
						break
					}
				}
			}
		}
		for k, p := range s.Exclude {
			checkAnyStylePattern(is, fmt.Sprintf("%s.exclude[%d]", sp, k), p)
		}
	}
}

// checkAllStylesPattern is used for sources without an os restriction: the
// pattern must be valid on Windows and on Unix-like systems.
func checkAllStylesPattern(is *Issues, path, p string) {
	werr := pathspec.Check(p, pathspec.WindowsStyle)
	perr := pathspec.Check(p, pathspec.PosixStyle)
	switch {
	case werr == nil && perr == nil:
	case werr != nil && perr != nil:
		is.Errorf(path, "invalid on every platform: Windows: %v; Linux/macOS: %v", werr, perr)
	case werr != nil:
		is.Errorf(path, "pattern %q is only valid on Linux/macOS; restrict the source with os: [linux, darwin]", p)
	default:
		is.Errorf(path, "pattern %q is only valid on Windows; restrict the source with os: [windows]", p)
	}
}

// checkAnyStylePattern accepts patterns valid on at least one platform
// (used for exclusions, which simply never match on other platforms).
func checkAnyStylePattern(is *Issues, path, p string) {
	werr := pathspec.Check(p, pathspec.WindowsStyle)
	perr := pathspec.Check(p, pathspec.PosixStyle)
	if werr != nil && perr != nil {
		is.Errorf(path, "invalid on every platform: Windows: %v; Linux/macOS: %v", werr, perr)
	}
}

func checkEnum(is *Issues, path, v string, allowed []string) {
	if !contains(allowed, v) {
		is.Errorf(path, "invalid value %q (allowed: %s)", v, strings.Join(allowed, ", "))
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
