// Package archive writes the collection ZIP archive in a single streaming
// pass. The archive is written to "<path>.partial" and renamed to its final
// name only after it was closed successfully, so a file at the final path is
// always a complete archive.
package archive

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/klauspost/compress/flate"

	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
)

// PartialSuffix is appended to the archive name while it is being written.
const PartialSuffix = ".partial"

// Writer is a streaming ZIP writer.
type Writer struct {
	Path        string
	tmp         string
	f           *os.File
	cw          *countingWriter
	zw          *zip.Writer
	compression string
	closed      bool
}

type countingWriter struct {
	w io.Writer
	h hash.Hash
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.h.Write(p[:n])
	c.n += int64(n)
	return n, err
}

// ErrExists is returned when the archive exists and overwrite is disabled.
var ErrExists = errors.New("output file already exists (set output.overwrite or use --overwrite)")

// Create starts a new archive.
func Create(path, compression string, level int, overwrite bool) (*Writer, error) {
	if _, err := os.Stat(path); err == nil && !overwrite {
		return nil, fmt.Errorf("%s: %w", path, ErrExists)
	}
	tmp := path + PartialSuffix
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	cw := &countingWriter{w: f, h: sha256.New()}
	zw := zip.NewWriter(cw)
	if level < flate.BestSpeed || level > flate.BestCompression {
		level = 5
	}
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, level)
	})
	return &Writer{Path: path, tmp: tmp, f: f, cw: cw, zw: zw, compression: compression}, nil
}

// TempPath is the path the archive is written to until Close.
func (w *Writer) TempPath() string { return w.tmp }

// Written returns the number of archive bytes written so far.
func (w *Writer) Written() int64 { return w.cw.n }

// Create adds an entry. sourceName is used to decide whether the data is
// already compressed (output.compression: auto).
func (w *Writer) Create(name string, modified time.Time, sourceName string) (io.Writer, error) {
	method := zip.Deflate
	switch w.compression {
	case config.CompressionStore:
		method = zip.Store
	case config.CompressionAuto:
		if isCompressed(sourceName) {
			method = zip.Store
		}
	}
	h := &zip.FileHeader{Name: name, Method: method}
	if !modified.IsZero() {
		h.Modified = modified.UTC()
	} else {
		h.Modified = time.Now().UTC()
	}
	h.SetMode(0o644)
	return w.zw.CreateHeader(h)
}

// WriteFile adds a small entry from memory.
func (w *Writer) WriteFile(name string, data []byte) error {
	ew, err := w.Create(name, time.Now(), name)
	if err != nil {
		return err
	}
	_, err = ew.Write(data)
	return err
}

// CopyFile adds an entry from a file on disk.
func (w *Writer) CopyFile(name, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	ew, err := w.Create(name, time.Now(), name)
	if err != nil {
		return err
	}
	_, err = io.Copy(ew, f)
	return err
}

// Close finishes the archive, syncs it to disk and renames it to its final
// path. It returns the size and SHA-256 of the archive file.
func (w *Writer) Close() (int64, string, error) {
	if w.closed {
		return 0, "", errors.New("archive already closed")
	}
	w.closed = true
	if err := w.zw.Close(); err != nil {
		w.f.Close()
		return 0, "", err
	}
	if err := w.f.Sync(); err != nil {
		w.f.Close()
		return 0, "", err
	}
	if err := w.f.Close(); err != nil {
		return 0, "", err
	}
	if err := os.Rename(w.tmp, w.Path); err != nil {
		return 0, "", err
	}
	return w.cw.n, hex.EncodeToString(w.cw.h.Sum(nil)), nil
}

// Abort discards the partial archive.
func (w *Writer) Abort() {
	if w.closed {
		return
	}
	w.closed = true
	w.f.Close()
	os.Remove(w.tmp)
}

var compressedExt = map[string]bool{
	".zip": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true, ".zst": true, ".7z": true,
	".rar": true, ".cab": true, ".lz4": true, ".jsonlz4": true, ".mozlz4": true, ".br": true,
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".heic": true,
	".mp3": true, ".mp4": true, ".mov": true, ".mkv": true, ".avi": true,
	".docx": true, ".xlsx": true, ".pptx": true, ".jar": true, ".apk": true, ".dmg": true,
}

func isCompressed(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(name, `\`, "/"))
	ext := path.Ext(name)
	if compressedExt[ext] {
		return true
	}
	// Rotated logs: syslog.2.gz is covered above; journal~ files are not compressed.
	return false
}

// EntryName maps a source path to its archive entry below "files/".
//
//	C:\Windows\System32\config\SAM  -> files/C/Windows/System32/config/SAM
//	C:\$Extend\$UsnJrnl:$J          -> files/C/$Extend/$UsnJrnl%3A$J
//	\\srv\share\x                   -> files/UNC/srv/share/x
//	/var/log/syslog                 -> files/var/log/syslog
//
// Characters that are invalid in Windows file names, control characters,
// '%' itself and trailing dots or spaces are percent-encoded, so archives
// extract on every platform and the mapping is reversible.
func EntryName(style pathspec.Style, p string) string {
	root, comps := style.Split(p)
	parts := []string{"files"}
	switch {
	case strings.HasPrefix(root, `\\`):
		parts = append(parts, "UNC")
		for _, c := range strings.Split(strings.TrimPrefix(root, `\\`), `\`) {
			parts = append(parts, encode(c))
		}
	case root != "":
		parts = append(parts, encode(strings.TrimSuffix(root, ":")))
	}
	for _, c := range comps {
		parts = append(parts, encode(c))
	}
	return strings.Join(parts, "/")
}

func encode(c string) string {
	var b strings.Builder
	for i := 0; i < len(c); i++ {
		ch := c[i]
		trailing := i == len(c)-1 && (ch == '.' || ch == ' ')
		if ch < 0x20 || ch == 0x7f || strings.IndexByte(`<>:"|?*\%/`, ch) >= 0 || trailing {
			fmt.Fprintf(&b, "%%%02X", ch)
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
