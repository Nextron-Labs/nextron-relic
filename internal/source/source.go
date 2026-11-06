// Package source opens files for collection using the configured access
// method: the regular operating system API ("os"), raw NTFS parsing
// ("ntfs_raw", Windows only) or automatic fallback from os to raw ("auto").
package source

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/Nextron-Labs/nextron-relic/internal/ntfsraw"
	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

// Method names recorded in file records.
const (
	MethodOS  = "os"
	MethodRaw = "ntfs_raw"
)

// ErrNotRegular is returned for devices, FIFOs, sockets and directories.
var ErrNotRegular = errors.New("not a regular file")

// ErrRawUnsupported is returned when raw access is requested where it is
// not available.
var ErrRawUnsupported = errors.New("raw NTFS access is only supported on Windows")

// Meta is the metadata of an opened file.
type Meta struct {
	Size  int64
	Mode  fs.FileMode
	Times result.Times
	UID   *int64
	GID   *int64
	Inode string
}

// File is an opened source ready to be streamed.
type File struct {
	// R is positioned at DataOffset.
	R io.Reader
	// Size is the number of bytes expected from R (-1 if unknown).
	Size           int64
	DataOffset     int64
	Method         string
	FallbackReason string
	Meta           Meta
	close          func() error
	recheck        func() (int64, time.Time, bool)
}

// Close releases the file.
func (f *File) Close() error {
	if f.close != nil {
		return f.close()
	}
	return nil
}

// Changed reports whether size or modification time differ from the values
// observed when the file was opened (only available for the os method).
func (f *File) Changed() bool {
	if f.recheck == nil {
		return false
	}
	size, mtime, ok := f.recheck()
	if !ok {
		return false
	}
	if size != f.Meta.Size {
		return true
	}
	return f.Meta.Times.Modified != nil && !mtime.Equal(*f.Meta.Times.Modified)
}

// Options control how a file is opened.
type Options struct {
	Method     string // config.MethodAuto, MethodOS or MethodRaw
	TrimSparse bool
	// Follow opens the target of a symbolic link instead of failing.
	Follow bool
}

// Accessor opens files and provides the matching file system view for
// pattern resolution.
type Accessor struct {
	raw *ntfsraw.FS
}

// New returns an Accessor for the local system.
func New() *Accessor {
	a := &Accessor{}
	if ntfsraw.Supported {
		a.raw = ntfsraw.NewFS()
	}
	platformInit()
	return a
}

// NewWithRaw returns an Accessor that uses the given raw file system (tests).
func NewWithRaw(raw *ntfsraw.FS) *Accessor { return &Accessor{raw: raw} }

// Close releases cached volume handles.
func (a *Accessor) Close() {
	if a.raw != nil {
		a.raw.Close()
	}
}

// ResolverFS returns the file system used to resolve patterns for a method.
func (a *Accessor) ResolverFS(method string) (pathspec.FS, error) {
	if method == config.MethodRaw {
		if a.raw == nil {
			return nil, ErrRawUnsupported
		}
		return a.raw, nil
	}
	return pathspec.OSFS{}, nil
}

// Open opens path with the requested method.
func (a *Accessor) Open(path string, o Options) (*File, error) {
	switch o.Method {
	case config.MethodRaw:
		return a.openRaw(path, o.TrimSparse)
	case config.MethodOS:
		return openOS(path, o.Follow)
	}
	f, err := openOS(path, o.Follow)
	if err == nil || a.raw == nil || !shouldFallback(err) {
		return f, err
	}
	rf, rerr := a.openRaw(path, o.TrimSparse)
	if rerr != nil {
		return nil, fmt.Errorf("%v; raw NTFS fallback failed: %v", err, rerr)
	}
	rf.FallbackReason = err.Error()
	return rf, nil
}

func (a *Accessor) openRaw(path string, trim bool) (*File, error) {
	if a.raw == nil {
		return nil, ErrRawUnsupported
	}
	s, err := a.raw.Open(path)
	if err != nil {
		return nil, err
	}
	start := int64(0)
	if trim {
		start = s.DataStart()
	}
	size := s.Size() - start
	i := s.Info
	return &File{
		R:          io.NewSectionReader(s, start, size),
		Size:       size,
		DataOffset: start,
		Method:     MethodRaw,
		Meta: Meta{
			Size:  s.Size(),
			Mode:  0o444,
			Inode: fmt.Sprintf("mft:%d-%d", i.MFTEntry, i.Sequence),
			Times: result.Times{
				Modified:  timePtr(i.Mtime),
				Accessed:  timePtr(i.Atime),
				Changed:   timePtr(i.Ctime),
				Created:   timePtr(i.Btime),
				FNCreated: timePtr(i.FNBtime),
			},
		},
	}, nil
}

// ntfsEpoch filters zero/unset NTFS timestamps (1601-01-01).
var ntfsEpoch = time.Date(1601, 1, 2, 0, 0, 0, 0, time.UTC)

func timePtr(t time.Time) *time.Time {
	if t.IsZero() || t.Before(ntfsEpoch) || t.Unix() <= 0 {
		return nil
	}
	u := t.UTC()
	return &u
}
