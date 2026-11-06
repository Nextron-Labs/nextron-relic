//go:build windows

package source

import (
	"errors"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformInit enables SeBackupPrivilege so that files protected by ACLs can
// be read with FILE_FLAG_BACKUP_SEMANTICS (LocalSystem and administrators
// hold the privilege, but it is disabled by default).
func platformInit() {
	_ = enablePrivilege("SeBackupPrivilege")
}

func enablePrivilege(name string) error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	var luid windows.LUID
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	if err := windows.LookupPrivilegeValue(nil, n, &luid); err != nil {
		return err
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil)
}

// shouldFallback reports errors that raw NTFS access can work around.
func shouldFallback(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// fileBasicInfo mirrors FILE_BASIC_INFO.
type fileBasicInfo struct {
	CreationTime, LastAccessTime, LastWriteTime, ChangeTime int64
	FileAttributes                                          uint32
	_                                                       uint32
}

func filetime(v int64) *time.Time {
	if v <= 0 {
		return nil
	}
	ft := windows.Filetime{LowDateTime: uint32(v), HighDateTime: uint32(v >> 32)}
	t := time.Unix(0, ft.Nanoseconds()).UTC()
	return &t
}

func openOS(path string, follow bool) (*File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.FILE_FLAG_BACKUP_SEMANTICS | windows.FILE_FLAG_SEQUENTIAL_SCAN)
	if !follow {
		flags |= windows.FILE_FLAG_OPEN_REPARSE_POINT
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	var bi fileBasicInfo
	if err := windows.GetFileInformationByHandleEx(h, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&bi)), uint32(unsafe.Sizeof(bi))); err != nil {
		windows.CloseHandle(h)
		return nil, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	if bi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("%w (directory)", ErrNotRegular)
	}
	var fi windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &fi); err != nil {
		windows.CloseHandle(h)
		return nil, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)
	size := int64(fi.FileSizeHigh)<<32 | int64(fi.FileSizeLow)
	m := Meta{
		Size:  size,
		Mode:  0o444,
		Inode: fmt.Sprintf("%d:%d", fi.VolumeSerialNumber, uint64(fi.FileIndexHigh)<<32|uint64(fi.FileIndexLow)),
	}
	m.Times.Created = filetime(bi.CreationTime)
	m.Times.Accessed = filetime(bi.LastAccessTime)
	m.Times.Modified = filetime(bi.LastWriteTime)
	m.Times.Changed = filetime(bi.ChangeTime)
	return &File{
		R:      f,
		Size:   size,
		Method: MethodOS,
		Meta:   m,
		close:  f.Close,
		recheck: func() (int64, time.Time, bool) {
			var fi2 windows.ByHandleFileInformation
			if err := windows.GetFileInformationByHandle(h, &fi2); err != nil {
				return 0, time.Time{}, false
			}
			s := int64(fi2.FileSizeHigh)<<32 | int64(fi2.FileSizeLow)
			return s, time.Unix(0, fi2.LastWriteTime.Nanoseconds()), true
		},
	}, nil
}
