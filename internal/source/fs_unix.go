//go:build !windows

package source

import "github.com/Nextron-Labs/nextron-relic/internal/pathspec"

// osFS returns the OS file system view used for pattern resolution.
func osFS() pathspec.FS { return pathspec.OSFS{} }
