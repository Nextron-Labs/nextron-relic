//go:build linux

package hostinfo

import (
	"golang.org/x/sys/unix"
)

const (
	ioprioWhoProcess = 1
	ioprioClassBE    = 2
	ioprioClassShift = 13
)

// lowerIOPriority sets the I/O scheduling class best-effort with the lowest
// priority level (7), like "ionice -c2 -n7".
func lowerIOPriority() error {
	prio := ioprioClassBE<<ioprioClassShift | 7
	_, _, errno := unix.Syscall(unix.SYS_IOPRIO_SET, ioprioWhoProcess, 0, uintptr(prio))
	if errno != 0 {
		return errno
	}
	return nil
}
