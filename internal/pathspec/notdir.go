package pathspec

import (
	"errors"
	"syscall"
)

// isNotDir reports "not a directory" style errors which only mean that a
// pattern did not match (e.g. /etc/passwd/x).
func isNotDir(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == errNotDir
	}
	return false
}
