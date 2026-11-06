// Package hostinfo gathers information about the local system: host
// identity for the result, path variables (%SystemRoot%, %UserHome%, ...)
// for pattern expansion, free disk space and process priority.
package hostinfo

import (
	"os"
	"os/user"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

// Host returns information about the local system.
func Host() result.Host {
	h := result.Host{OS: runtime.GOOS, Arch: runtime.GOARCH}
	h.Hostname, _ = os.Hostname()
	if u, err := user.Current(); err == nil {
		h.User = u.Username
	}
	h.OSVersion = osVersion()
	h.Elevated = elevated()
	name, offset := time.Now().Zone()
	h.TimeZone = name
	if name == "" || strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
		h.TimeZone = time.Now().Format("-07:00")
	}
	_ = offset
	return h
}

// Expander returns the pattern variable expander for the local system.
func Expander() *pathspec.Expander {
	vars := pathspec.Vars{}
	vars.Set("UserHome", UserHomes()...)
	return platformExpander(vars)
}

// dedupeDirs keeps existing directories, de-duplicated (case-insensitively
// where the platform is case-insensitive), sorted.
func dedupeDirs(candidates []string, caseInsensitive bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range candidates {
		c = strings.TrimRight(c, `/\`)
		if c == "" {
			continue
		}
		key := c
		if caseInsensitive {
			key = strings.ToLower(c)
		}
		if seen[key] {
			continue
		}
		fi, err := os.Stat(c)
		if err != nil || !fi.IsDir() {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
