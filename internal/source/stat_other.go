//go:build !windows && !linux && !darwin

package source

import "syscall"

const noatimeFlag = 0

// fillTimes is a no-op on other Unix systems; the modification time from
// os.FileInfo is used instead.
func fillTimes(m *Meta, st *syscall.Stat_t) {}
