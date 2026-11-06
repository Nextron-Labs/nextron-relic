//go:build !windows

package source

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"
)

func platformInit() {}

// shouldFallback is never true on Unix: raw access is Windows only.
func shouldFallback(error) bool { return false }

// openOS opens a regular file without blocking on FIFOs and, on Linux,
// without updating the access time.
func openOS(path string, follow bool) (*File, error) {
	flags := os.O_RDONLY | syscall.O_NONBLOCK | noatimeFlag
	if !follow {
		flags |= syscall.O_NOFOLLOW
	}
	f, err := os.OpenFile(path, flags, 0)
	if err != nil && noatimeFlag != 0 && errors.Is(err, syscall.EPERM) {
		// O_NOATIME requires file ownership or CAP_FOWNER.
		f, err = os.OpenFile(path, flags&^noatimeFlag, 0)
	}
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("%w (%s)", ErrNotRegular, fi.Mode().Type())
	}
	m := Meta{Size: fi.Size(), Mode: fi.Mode()}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		uid, gid := int64(st.Uid), int64(st.Gid)
		m.UID, m.GID = &uid, &gid
		m.Inode = strconv.FormatUint(uint64(st.Ino), 10)
		fillTimes(&m, st)
	}
	if m.Times.Modified == nil {
		mt := fi.ModTime().UTC()
		m.Times.Modified = &mt
	}
	return &File{
		R:      f,
		Size:   fi.Size(),
		Method: MethodOS,
		Meta:   m,
		close:  f.Close,
		recheck: func() (int64, time.Time, bool) {
			fi2, err := f.Stat()
			if err != nil {
				return 0, time.Time{}, false
			}
			return fi2.Size(), fi2.ModTime(), true
		},
	}, nil
}

func ts(sec, nsec int64) *time.Time {
	if sec == 0 && nsec == 0 {
		return nil
	}
	t := time.Unix(sec, nsec).UTC()
	return &t
}
