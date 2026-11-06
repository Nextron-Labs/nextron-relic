// Package ntfsraw reads files directly from NTFS volumes, bypassing the
// operating system's file locks and access checks. It is used on Windows for
// files that cannot be opened normally (registry hives, event logs, ESE
// databases) and for NTFS metadata files ($MFT, $LogFile, $UsnJrnl:$J).
//
// Parsing is done with Velocidex go-ntfs (Apache-2.0). The volume logic is
// platform independent and is tested against NTFS image files; only opening
// the volume device (\\.\C:) is Windows specific.
package ntfsraw

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"time"

	"www.velocidex.com/golang/go-ntfs/parser"
)

const (
	// pageSize is a multiple of both 512 and 4096 byte sectors, so reads on
	// volume devices are always sector aligned.
	pageSize = 4096
	// cachePages bounds the read cache per volume (20 MiB).
	cachePages = 5120
)

// Volume is an opened NTFS file system.
type Volume struct {
	Name   string
	mu     sync.Mutex
	ntfs   *parser.NTFSContext
	closer io.Closer
}

// Info describes a file, directory or alternate data stream.
type Info struct {
	// Name is the base name as stored in the directory index; alternate data
	// streams are named "file:stream".
	Name   string
	Stream string
	IsDir  bool
	Size   int64
	// MFTEntry and Sequence identify the file record.
	MFTEntry int64
	Sequence uint16
	// $STANDARD_INFORMATION timestamps.
	Mtime, Atime, Ctime, Btime time.Time
	// FNBtime is the $FILE_NAME creation time, which is harder to tamper with.
	FNBtime time.Time
}

// guard converts panics of the parser (which happen on inconsistent live
// file systems) into errors.
func guard(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("NTFS parser error: %v", r)
	}
}

// Open parses an NTFS file system from r. closer (may be nil) is closed by Close.
func Open(r io.ReaderAt, closer io.Closer, name string) (v *Volume, err error) {
	defer guard(&err)
	paged, err := parser.NewPagedReader(r, pageSize, cachePages)
	if err != nil {
		return nil, err
	}
	ctx, err := parser.GetNTFSContext(paged, 0)
	if err != nil {
		return nil, fmt.Errorf("not an NTFS volume: %w", err)
	}
	return &Volume{Name: name, ntfs: ctx, closer: closer}, nil
}

// Close releases the underlying device or image.
func (v *Volume) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.ntfs != nil {
		v.ntfs.Close()
	}
	if v.closer != nil {
		return v.closer.Close()
	}
	return nil
}

// splitStream separates "dir/file:stream" into path and stream name.
// "file::$DATA" denotes the default stream.
func splitStream(p string) (string, string) {
	p = strings.ReplaceAll(p, `\`, "/")
	i := strings.LastIndex(p, "/")
	base := p[i+1:]
	j := strings.Index(base, ":")
	if j < 0 {
		return p, ""
	}
	stream := base[j+1:]
	stream = strings.TrimSuffix(stream, ":$DATA")
	if stream == "$DATA" {
		stream = ""
	}
	return p[:i+1] + base[:j], stream
}

func baseName(p string) string {
	p = strings.TrimRight(p, "/")
	return p[strings.LastIndex(p, "/")+1:]
}

func (v *Volume) entry(p string) (*parser.MFT_ENTRY, error) {
	root, err := v.ntfs.GetMFT(5)
	if err != nil {
		return nil, err
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return root, nil
	}
	e, err := root.Open(v.ntfs, p)
	if err != nil {
		if strings.Contains(err.Error(), "Not found") {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	return e, nil
}

func toInfo(fi *parser.FileInfo, name, stream string, entry int64) *Info {
	return &Info{
		Name: name, Stream: stream, IsDir: fi.IsDir, Size: fi.Size,
		MFTEntry: entry, Sequence: fi.SequenceNumber,
		Mtime: fi.Mtime, Atime: fi.Atime, Ctime: fi.Ctime, Btime: fi.Btime, FNBtime: fi.FNBtime,
	}
}

func splitName(n string) (string, string) {
	if i := strings.Index(n, ":"); i >= 0 {
		return n[:i], n[i+1:]
	}
	return n, ""
}

// Stat returns information about a path inside the volume ("/" or "\"
// separated, relative to the volume root), optionally with ":stream".
func (v *Volume) Stat(p string) (info *Info, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	defer guard(&err)

	path, stream := splitStream(p)
	e, err := v.entry(path)
	if err != nil {
		return nil, err
	}
	base := baseName(path)
	var candidate *parser.FileInfo
	for _, fi := range parser.Stat(v.ntfs, e) {
		nb, ns := splitName(fi.Name)
		if !strings.EqualFold(ns, stream) {
			continue
		}
		if base == "" || strings.EqualFold(nb, base) {
			candidate = fi
			break
		}
		if candidate == nil {
			candidate = fi
		}
	}
	if candidate == nil {
		return nil, fs.ErrNotExist
	}
	name := base
	if stream != "" {
		name += ":" + stream
	}
	return toInfo(candidate, name, stream, int64(e.Record_number())), nil
}

// ReadDir lists a directory using its index, so every name is reported
// as it appears in this directory (hard links elsewhere are not mixed in).
// Alternate data streams are returned as additional "name:stream" entries.
func (v *Volume) ReadDir(p string) (out []*Info, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	defer guard(&err)

	dir, err := v.entry(p)
	if err != nil {
		return nil, err
	}
	if !dir.IsDir(v.ntfs) {
		return nil, fmt.Errorf("%s: not a directory", p)
	}
	seen := map[string]bool{}
	for _, rec := range dir.Dir(v.ntfs) {
		fn := rec.File()
		if fn.NameType().Name == "DOS" {
			continue // 8.3 short name, the long name is listed separately
		}
		name := fn.Name()
		key := strings.ToLower(name)
		if name == "." || name == "" || seen[key] {
			continue
		}
		seen[key] = true
		ref := int64(rec.MftReference())
		child, err := v.ntfs.GetMFT(ref)
		if err != nil {
			continue
		}
		infos := parser.Stat(v.ntfs, child)
		var main *parser.FileInfo
		for _, fi := range infos {
			if _, s := splitName(fi.Name); s == "" && main == nil {
				main = fi
			}
		}
		if main == nil {
			continue
		}
		out = append(out, toInfo(main, name, "", ref))
		for _, fi := range infos {
			if _, s := splitName(fi.Name); s != "" && !seen[key+":"+strings.ToLower(s)] {
				seen[key+":"+strings.ToLower(s)] = true
				out = append(out, toInfo(fi, name+":"+s, s, ref))
			}
		}
	}
	return out, nil
}

// Stream is an opened data stream.
type Stream struct {
	Info *Info
	v    *Volume
	r    parser.RangeReaderAt
}

// OpenStream opens the default or a named data stream of a file.
func (v *Volume) OpenStream(p string) (*Stream, error) {
	info, err := v.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.IsDir && info.Stream == "" {
		return nil, fmt.Errorf("%s: is a directory", p)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	var rng parser.RangeReaderAt
	err = func() (err error) {
		defer guard(&err)
		path, stream := splitStream(p)
		e, err := v.entry(path)
		if err != nil {
			return err
		}
		rng, err = parser.OpenStream(v.ntfs, e, parser.ATTR_TYPE_DATA, parser.WILDCARD_STREAM_ID, stream)
		return err
	}()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	return &Stream{Info: info, v: v, r: rng}, nil
}

// Size is the logical size of the stream.
func (s *Stream) Size() int64 { return s.Info.Size }

// DataStart returns the offset of the first allocated (non-sparse) byte.
// For $UsnJrnl:$J this skips the typically huge leading sparse region.
func (s *Stream) DataStart() int64 {
	for _, r := range s.r.Ranges() {
		if !r.IsSparse && r.Length > 0 {
			if r.Offset > s.Info.Size {
				return s.Info.Size
			}
			return r.Offset
		}
	}
	return s.Info.Size
}

// ReadAt reads from the stream. Reads never extend past Size.
func (s *Stream) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= s.Info.Size {
		return 0, io.EOF
	}
	if rem := s.Info.Size - off; int64(len(p)) > rem {
		p = p[:rem]
	}
	s.v.mu.Lock()
	defer s.v.mu.Unlock()
	defer guard(&err)
	n, err = s.r.ReadAt(p, off)
	if err == nil && n < len(p) {
		// go-ntfs returns short reads without error at run boundaries;
		// io.SectionReader handles retries.
		return n, nil
	}
	return n, err
}
