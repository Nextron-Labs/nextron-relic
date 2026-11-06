//go:build windows

package pathspec

import "syscall"

// ERROR_PATH_NOT_FOUND is mapped to fs.ErrNotExist by Go already; a file used
// as directory yields ERROR_DIRECTORY (267).
const errNotDir = syscall.Errno(267)
