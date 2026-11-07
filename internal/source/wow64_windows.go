//go:build windows

package source

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"

	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
)

// isWow64 is true for a 32-bit binary running on 64-bit Windows.
var isWow64 = func() bool {
	var wow64 bool
	if err := windows.IsWow64Process(windows.CurrentProcess(), &wow64); err != nil {
		return false
	}
	return wow64
}()

// nativePath bypasses the WoW64 file system redirection for System32.
// Recorded paths keep the original System32 spelling.
func nativePath(p string) string {
	if !isWow64 {
		return p
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return sysnativePath(p, root)
}

// osFS returns the OS file system view used for pattern resolution.
func osFS() pathspec.FS {
	if isWow64 {
		return wow64FS{}
	}
	return pathspec.OSFS{}
}

// wow64FS resolves patterns like OSFS but reaches the real System32 from a
// 32-bit process.
type wow64FS struct{}

func (wow64FS) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(nativePath(name)) }
func (wow64FS) Stat(name string) (fs.FileInfo, error)  { return os.Stat(nativePath(name)) }
func (wow64FS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(nativePath(name))
}
