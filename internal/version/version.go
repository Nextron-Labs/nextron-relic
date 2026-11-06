// Package version holds build information. Version and Commit are set at
// build time:
//
//	go build -ldflags "-X github.com/Nextron-Labs/nextron-relic/internal/version.Version=0.1.0"
package version

import (
	"runtime"
	"runtime/debug"

	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

var (
	Version = "0.1.0-dev"
	Commit  = ""
)

// Tool returns the tool description for results.
func Tool() result.Tool {
	commit := Commit
	if commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 12 {
					commit = s.Value[:12]
				}
			}
		}
	}
	return result.Tool{Name: "relic", Version: Version, Commit: commit, GoVersion: runtime.Version()}
}
