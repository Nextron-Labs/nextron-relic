package ntfsraw

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"
)

// FS exposes NTFS volumes addressed by Windows drive paths ("C:\Windows\...")
// for pattern resolution and reading. Volumes are opened on first use and
// cached until Close.
type FS struct {
	// Opener opens the volume for a drive ("C:"). Defaults to OpenDevice.
	Opener func(drive string) (*Volume, error)

	mu   sync.Mutex
	vols map[string]volEntry
}

type volEntry struct {
	v   *Volume
	err error
}

// NewFS returns an FS that opens local volume devices.
func NewFS() *FS { return &FS{Opener: OpenDevice} }

// SplitDrive splits `C:\a\b` (or `\\?\C:\a\b`) into "C:" and "a/b".
func SplitDrive(p string) (drive, inner string, err error) {
	p = strings.TrimPrefix(p, `\\?\`)
	if len(p) < 2 || p[1] != ':' || !((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
		return "", "", fmt.Errorf("raw NTFS access needs a path on a local drive letter, got %q", p)
	}
	inner = strings.ReplaceAll(p[2:], `\`, "/")
	return strings.ToUpper(p[:2]), strings.Trim(inner, "/"), nil
}

// Volume returns the (cached) volume of a drive path.
func (f *FS) Volume(p string) (*Volume, string, error) {
	drive, inner, err := SplitDrive(p)
	if err != nil {
		return nil, "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.vols == nil {
		f.vols = map[string]volEntry{}
	}
	e, ok := f.vols[drive]
	if !ok {
		opener := f.Opener
		if opener == nil {
			opener = OpenDevice
		}
		v, err := opener(drive)
		e = volEntry{v, err}
		f.vols[drive] = e
	}
	return e.v, inner, e.err
}

// Close closes all opened volumes.
func (f *FS) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var first error
	for _, e := range f.vols {
		if e.v != nil {
			if err := e.v.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	f.vols = nil
	return first
}

func (f *FS) Lstat(p string) (fs.FileInfo, error) {
	v, inner, err := f.Volume(p)
	if err != nil {
		return nil, err
	}
	info, err := v.Stat(inner)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: p, Err: err}
	}
	return FileInfo{info}, nil
}

// Stat is identical to Lstat: the raw view does not follow reparse points.
func (f *FS) Stat(p string) (fs.FileInfo, error) { return f.Lstat(p) }

func (f *FS) ReadDir(p string) ([]fs.DirEntry, error) {
	v, inner, err := f.Volume(p)
	if err != nil {
		return nil, err
	}
	infos, err := v.ReadDir(inner)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: p, Err: err}
	}
	out := make([]fs.DirEntry, 0, len(infos))
	for _, i := range infos {
		out = append(out, dirEntry{FileInfo{i}})
	}
	return out, nil
}

// Open opens a file or "file:stream" on its volume.
func (f *FS) Open(p string) (*Stream, error) {
	v, inner, err := f.Volume(p)
	if err != nil {
		return nil, err
	}
	s, err := v.OpenStream(inner)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: p, Err: err}
	}
	return s, nil
}

// FileInfo adapts Info to fs.FileInfo. Sys returns *Info.
type FileInfo struct{ I *Info }

func (fi FileInfo) Name() string { return fi.I.Name }
func (fi FileInfo) Size() int64  { return fi.I.Size }
func (fi FileInfo) Mode() fs.FileMode {
	if fi.I.IsDir && fi.I.Stream == "" {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (fi FileInfo) ModTime() time.Time { return fi.I.Mtime }
func (fi FileInfo) IsDir() bool        { return fi.Mode().IsDir() }
func (fi FileInfo) Sys() any           { return fi.I }

type dirEntry struct{ fi FileInfo }

func (d dirEntry) Name() string               { return d.fi.Name() }
func (d dirEntry) IsDir() bool                { return d.fi.IsDir() }
func (d dirEntry) Type() fs.FileMode          { return d.fi.Mode().Type() }
func (d dirEntry) Info() (fs.FileInfo, error) { return d.fi, nil }
