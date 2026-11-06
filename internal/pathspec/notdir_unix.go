//go:build !windows

package pathspec

import "syscall"

const errNotDir = syscall.ENOTDIR
