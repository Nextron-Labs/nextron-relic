package collector

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/Nextron-Labs/nextron-relic/internal/archive"
	"github.com/Nextron-Labs/nextron-relic/internal/catalog"
	"github.com/Nextron-Labs/nextron-relic/internal/hostinfo"
	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
	"github.com/Nextron-Labs/nextron-relic/internal/source"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

const maxArtifactErrors = 25

var (
	errDiskSpace    = errors.New("min_free_space reached on the output volume")
	errArchiveWrite = errors.New("archive write failed")
	errNoProgress   = errors.New("read made no progress")
)

type artifactState struct {
	res        result.ArtifactResult
	limitSkips int
	errCount   int
}

func (a *artifactState) addError(msg string) {
	a.errCount++
	if len(a.res.Errors) < maxArtifactErrors {
		a.res.Errors = append(a.res.Errors, msg)
	} else {
		a.res.ErrorsDropped++
	}
}

func (r *run) collectArtifact(ctx context.Context, s catalog.Selected) {
	a := &artifactState{res: result.ArtifactResult{Name: s.Artifact.Name, Custom: s.Custom, Reason: s.Reason}}
	r.log.Debug("collecting artifact", "artifact", s.Artifact.Name)

sources:
	for si, src := range s.Sources {
		method := src.Method
		if method == "" {
			method = r.cfg.Collect.Method
		}
		fsys, err := r.acc.ResolverFS(method)
		if err != nil {
			a.addError(fmt.Sprintf("source %d: %v", si, err))
			continue
		}
		excl := r.compileExcludes(a, src.Exclude)
		res := &pathspec.Resolver{
			FS:             fsys,
			FollowSymlinks: r.cfg.Collect.FollowSymlinks,
			MaxDepth:       r.cfg.Collect.MaxDepth,
			OnError: func(p string, err error) {
				r.res.Summary.EnumerationErrors++
				a.addError(fmt.Sprintf("%s: %v", p, cleanErr(err)))
			},
		}
		for _, p := range src.Paths {
			pats, err := pathspec.Compile(p, r.style, r.exp)
			if err != nil {
				a.addError(fmt.Sprintf("pattern %q: %v", p, err))
				r.res.Errors = append(r.res.Errors, fmt.Sprintf("artifact %s: pattern %q: %v", s.Artifact.Name, p, err))
				continue
			}
			for _, pat := range pats {
				err := res.Resolve(ctx, pat, func(m pathspec.Match) error {
					return r.handle(ctx, a, src, method, fsys, pat, excl, m)
				})
				if err != nil && ctx.Err() == nil {
					a.addError(fmt.Sprintf("pattern %q: %v", pat.Text, err))
				}
				if r.halted(ctx) {
					break sources
				}
			}
		}
	}
	r.finishArtifact(a)
}

func (r *run) finishArtifact(a *artifactState) {
	ar := &a.res
	collected := ar.FilesCollected + ar.FilesTruncated
	problems := ar.FilesFailed > 0 || ar.FilesTruncated > 0 || a.limitSkips > 0 || a.errCount > 0
	interrupted := r.canceled || r.stopped != "" || r.fatal != nil
	switch {
	case ar.FilesMatched == 0 && interrupted:
		ar.Status = result.ArtifactSkipped
	case ar.FilesMatched == 0 && !problems:
		ar.Status = result.ArtifactNotFound
	case collected == 0 && (ar.FilesFailed > 0 || a.errCount > 0 || a.limitSkips > 0):
		ar.Status = result.ArtifactFailed
	case collected == 0:
		ar.Status = result.ArtifactNotFound // only policy skips (symlinks, directories)
	case problems || interrupted:
		ar.Status = result.ArtifactPartial
	default:
		ar.Status = result.ArtifactCollected
	}
	r.res.Artifacts = append(r.res.Artifacts, *ar)
	lvl := r.log.Info
	if ar.Status == result.ArtifactFailed || ar.Status == result.ArtifactPartial {
		lvl = r.log.Warn
	}
	lvl("artifact done", "artifact", ar.Name, "status", ar.Status, "matched", ar.FilesMatched,
		"collected", collected, "failed", ar.FilesFailed, "bytes", ar.BytesCollected, "errors", a.errCount)
}

func (r *run) compileExcludes(a *artifactState, list []string) []*pathspec.Pattern {
	var out []*pathspec.Pattern
	for _, p := range list {
		if pathspec.Check(p, r.style) != nil {
			continue
		}
		pats, err := pathspec.Compile(p, r.style, r.exp)
		if err != nil {
			a.addError(fmt.Sprintf("exclude %q: %v", p, err))
			continue
		}
		out = append(out, pats...)
	}
	return append(out, r.globalExcl...)
}

func excluded(p string, excl []*pathspec.Pattern) bool {
	for _, x := range excl {
		if x.Match(p) {
			return true
		}
	}
	return false
}

// handle processes one matched path. Returning pathspec.ErrStop ends the
// resolution of the current pattern.
func (r *run) handle(ctx context.Context, a *artifactState, src config.Source, method string,
	fsys pathspec.FS, pat *pathspec.Pattern, excl []*pathspec.Pattern, m pathspec.Match) error {

	if excluded(m.Path, excl) {
		return nil
	}
	k := keyHash(r.style.Key(m.Path))
	if _, dup := r.seen[k]; dup {
		a.res.FilesDuplicate++
		r.res.Summary.FilesDuplicate++
		return nil
	}
	r.seen[k] = struct{}{}
	a.res.FilesMatched++
	r.res.Summary.FilesMatched++

	rec := &result.FileRecord{
		Artifact:    a.res.Name,
		Pattern:     pat.Text,
		SourcePath:  m.Path,
		Size:        m.Info.Size(),
		Mode:        m.Info.Mode().String(),
		CollectedAt: time.Now().UTC(),
	}
	if mt := m.Info.ModTime(); !mt.IsZero() {
		u := mt.UTC()
		rec.Times.Modified = &u
	}

	info := m.Info
	follow := false
	if info.Mode()&fs.ModeSymlink != 0 {
		if t, err := os.Readlink(m.Path); err == nil {
			rec.SymlinkTarget = t
		}
		if !m.Literal && !r.cfg.Collect.FollowSymlinks {
			return r.skip(a, rec, "symbolic link not followed (set collect.follow_symlinks to follow)", false)
		}
		st, err := fsys.Stat(m.Path)
		if err != nil {
			return r.skip(a, rec, "broken symbolic link", false)
		}
		info, follow = st, true
		rec.Size = st.Size()
	}
	if info.IsDir() {
		return r.skip(a, rec, "is a directory (append /** to the pattern to collect its contents)", false)
	}
	if !info.Mode().IsRegular() {
		return r.skip(a, rec, fmt.Sprintf("not a regular file (%s)", typeName(info.Mode())), false)
	}

	l := r.cfg.Limits
	if l.MaxFiles > 0 && r.res.Summary.FilesCollected+r.res.Summary.FilesTruncated >= l.MaxFiles {
		r.hitLimit("max_files", int64(l.MaxFiles), fmt.Sprintf("file count limit of %d reached; collection stopped", l.MaxFiles))
		r.stopped = "max_files"
		r.skip(a, rec, "max_files limit reached", true)
		return pathspec.ErrStop
	}
	// For sparse-trimmed raw streams the logical size is meaningless; limits
	// are checked again after opening.
	if !src.TrimSparse {
		if err := r.checkSizeLimits(a, rec, rec.Size); err != nil {
			return nil
		}
	}

	if r.opts.DryRun {
		size := rec.Size
		if l.MaxFileSize > 0 && size > int64(l.MaxFileSize) {
			size = int64(l.MaxFileSize)
		}
		rec.Status = result.FileCollected
		rec.Reason = "dry run"
		a.res.FilesCollected++
		r.res.Summary.FilesCollected++
		a.res.BytesCollected += size
		r.res.Summary.BytesCollected += size
		r.total += size
		r.log.Info("would collect", "artifact", a.res.Name, "path", m.Path, "size", rec.Size)
		return nil
	}

	if err := r.checkDisk(); err != nil {
		r.diskLimit()
		r.skip(a, rec, err.Error(), true)
		return pathspec.ErrStop
	}

	f, err := r.acc.Open(m.Path, source.Options{Method: method, TrimSparse: src.TrimSparse, Follow: follow})
	if err != nil {
		if errors.Is(err, source.ErrNotRegular) {
			return r.skip(a, rec, cleanErr(err), false)
		}
		return r.fail(a, rec, cleanErr(err))
	}
	defer f.Close()
	rec.Method = f.Method
	rec.FallbackReason = f.FallbackReason
	rec.DataOffset = f.DataOffset
	applyMeta(rec, f.Meta)
	if src.TrimSparse {
		if err := r.checkSizeLimits(a, rec, f.Size); err != nil {
			return nil
		}
	}

	name := archive.EntryName(r.style, m.Path)
	var mod time.Time
	if rec.Times.Modified != nil {
		mod = *rec.Times.Modified
	}
	w, err := r.arch.Create(name, mod, m.Path)
	if err != nil {
		r.fatal = fmt.Errorf("%w: %v", errArchiveWrite, err)
		r.res.Errors = append(r.res.Errors, r.fatal.Error())
		return pathspec.ErrStop
	}
	rec.ArchivePath = name

	limit := int64(l.MaxFileSize)
	n, sums, truncated, cerr := r.copy(ctx, w, f.R, limit)
	rec.BytesStored = n
	rec.Hashes = sums
	r.total += n
	a.res.BytesCollected += n
	r.res.Summary.BytesCollected += n
	if f.Changed() {
		rec.ChangedDuringRead = true
	}

	switch {
	case errors.Is(cerr, errArchiveWrite):
		r.fatal = cerr
		r.res.Errors = append(r.res.Errors, cerr.Error())
		rec.Status, rec.Reason = result.FileAborted, cerr.Error()
		r.countFailed(a, rec)
		return pathspec.ErrStop
	case errors.Is(cerr, errDiskSpace):
		r.diskLimit()
		rec.Status, rec.Reason = result.FileAborted, fmt.Sprintf("stopped after %d bytes: %v", n, cerr)
		r.countFailed(a, rec)
		return pathspec.ErrStop
	case cerr != nil && ctx.Err() != nil:
		rec.Status, rec.Reason = result.FileAborted, fmt.Sprintf("collection stopped after %d bytes", n)
		r.countFailed(a, rec)
		return pathspec.ErrStop
	case cerr != nil:
		return r.fail(a, rec, fmt.Sprintf("read error after %d bytes: %s", n, cleanErr(cerr)))
	case truncated:
		rec.Status = result.FileTruncated
		rec.Reason = fmt.Sprintf("only the first %s were stored (max_file_size)", l.MaxFileSize)
		a.res.FilesTruncated++
		r.res.Summary.FilesTruncated++
	default:
		rec.Status = result.FileCollected
		a.res.FilesCollected++
		r.res.Summary.FilesCollected++
	}
	r.writeRecord(rec)
	r.progress(a.res.Name)
	return nil
}

// checkSizeLimits skips files that exceed max_file_size (oversize: skip) or
// would exceed max_total_size.
func (r *run) checkSizeLimits(a *artifactState, rec *result.FileRecord, size int64) error {
	l := r.cfg.Limits
	if l.MaxFileSize > 0 && size > int64(l.MaxFileSize) && l.Oversize == config.OversizeSkip {
		r.skip(a, rec, fmt.Sprintf("size %d exceeds max_file_size %s", size, l.MaxFileSize), true)
		return errSkip
	}
	expected := size
	if l.MaxFileSize > 0 && expected > int64(l.MaxFileSize) {
		expected = int64(l.MaxFileSize)
	}
	if l.MaxTotalSize > 0 && r.total+expected > int64(l.MaxTotalSize) {
		r.hitLimit("max_total_size", int64(l.MaxTotalSize),
			fmt.Sprintf("total size limit of %s reached; remaining files that do not fit are skipped", l.MaxTotalSize))
		r.skip(a, rec, fmt.Sprintf("would exceed max_total_size %s", l.MaxTotalSize), true)
		return errSkip
	}
	return nil
}

var errSkip = errors.New("skipped")

func (r *run) diskLimit() {
	r.hitLimit("min_free_space", int64(r.cfg.Limits.MinFreeSpace),
		fmt.Sprintf("free space on the output volume fell below %s; collection stopped", r.cfg.Limits.MinFreeSpace))
	r.stopped = "min_free_space"
}

func (r *run) checkDisk() error {
	if r.cfg.Limits.MinFreeSpace <= 0 || r.outDir == "" {
		return nil
	}
	free, err := hostinfo.FreeSpace(r.outDir)
	if err != nil {
		return nil // cannot be determined; do not block the collection
	}
	if free < uint64(r.cfg.Limits.MinFreeSpace) {
		return errDiskSpace
	}
	return nil
}

func (r *run) skip(a *artifactState, rec *result.FileRecord, reason string, limit bool) error {
	rec.Status = result.FileSkipped
	rec.Reason = reason
	a.res.FilesSkipped++
	r.res.Summary.FilesSkipped++
	if limit {
		a.limitSkips++
		r.limitSkips++
		r.failLogged++
		if r.failLogged <= 50 {
			r.log.Warn("file skipped", "artifact", a.res.Name, "path", rec.SourcePath, "reason", reason)
		} else {
			r.log.Debug("file skipped", "artifact", a.res.Name, "path", rec.SourcePath, "reason", reason)
		}
	} else {
		r.log.Debug("file skipped", "artifact", a.res.Name, "path", rec.SourcePath, "reason", reason)
	}
	r.writeRecord(rec)
	return nil
}

func (r *run) fail(a *artifactState, rec *result.FileRecord, reason string) error {
	rec.Status = result.FileFailed
	rec.Reason = reason
	r.countFailed(a, rec)
	return nil
}

func (r *run) countFailed(a *artifactState, rec *result.FileRecord) {
	a.res.FilesFailed++
	r.res.Summary.FilesFailed++
	r.failLogged++
	if r.failLogged <= 50 {
		r.log.Warn("file not collected", "artifact", a.res.Name, "path", rec.SourcePath, "status", rec.Status, "reason", rec.Reason)
	} else {
		r.log.Debug("file not collected", "artifact", a.res.Name, "path", rec.SourcePath, "status", rec.Status, "reason", rec.Reason)
	}
	r.writeRecord(rec)
}

func (r *run) writeRecord(rec *result.FileRecord) {
	if r.recEnc == nil {
		return
	}
	if err := r.recEnc.Encode(rec); err != nil && r.fatal == nil {
		r.fatal = fmt.Errorf("writing file records: %w", err)
		r.res.Errors = append(r.res.Errors, r.fatal.Error())
	}
}

func (r *run) progress(artifact string) {
	if time.Since(r.lastProgress) < 15*time.Second {
		return
	}
	r.lastProgress = time.Now()
	r.log.Info("progress", "artifact", artifact, "files", r.res.Summary.FilesCollected,
		"bytes", r.res.Summary.BytesCollected, "failed", r.res.Summary.FilesFailed)
}

// copy streams src into dst, hashing on the fly. With limit > 0 at most
// limit bytes are copied; truncated reports whether more data was available.
func (r *run) copy(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (n int64, sums map[string]string, truncated bool, err error) {
	hs := newHashers(r.cfg.Collect.Hashes)
	defer func() { sums = hs.sums() }()
	reader := src
	if limit > 0 {
		reader = io.LimitReader(src, limit)
	}
	var sinceCheck int64
	stalls := 0
	for {
		if err := ctx.Err(); err != nil {
			return n, nil, false, context.Cause(ctx)
		}
		nr, er := reader.Read(r.buf)
		if nr > 0 {
			stalls = 0
			chunk := r.buf[:nr]
			if _, ew := dst.Write(chunk); ew != nil {
				return n, nil, false, fmt.Errorf("%w: %v", errArchiveWrite, ew)
			}
			hs.write(chunk)
			n += int64(nr)
			sinceCheck += int64(nr)
			r.thr.wait(ctx, nr)
			if sinceCheck >= 64<<20 {
				sinceCheck = 0
				if err := r.checkDisk(); err != nil {
					return n, nil, false, err
				}
			}
		} else if er == nil {
			if stalls++; stalls > 100 {
				return n, nil, false, errNoProgress
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			return n, nil, false, er
		}
	}
	if limit > 0 && n == limit {
		var one [1]byte
		k, _ := src.Read(one[:])
		truncated = k > 0
	}
	return n, nil, truncated, nil
}

type hashers struct {
	names []string
	hs    []hash.Hash
}

func newHashers(names []string) *hashers {
	h := &hashers{}
	for _, n := range names {
		var x hash.Hash
		switch n {
		case "md5":
			x = md5.New()
		case "sha1":
			x = sha1.New()
		case "sha256":
			x = sha256.New()
		case "sha512":
			x = sha512.New()
		default:
			continue
		}
		h.names = append(h.names, n)
		h.hs = append(h.hs, x)
	}
	return h
}

func (h *hashers) write(p []byte) {
	for _, x := range h.hs {
		x.Write(p)
	}
}

func (h *hashers) sums() map[string]string {
	out := make(map[string]string, len(h.hs))
	for i, x := range h.hs {
		out[h.names[i]] = hex.EncodeToString(x.Sum(nil))
	}
	return out
}

func applyMeta(rec *result.FileRecord, m source.Meta) {
	if m.Times.Modified != nil {
		rec.Times = m.Times
	} else {
		mod := rec.Times.Modified
		rec.Times = m.Times
		rec.Times.Modified = mod
	}
	rec.UID, rec.GID, rec.Inode = m.UID, m.GID, m.Inode
	if m.Mode != 0 {
		rec.Mode = m.Mode.String()
	}
}

func typeName(m fs.FileMode) string {
	switch {
	case m&fs.ModeNamedPipe != 0:
		return "named pipe"
	case m&fs.ModeSocket != 0:
		return "socket"
	case m&fs.ModeDevice != 0:
		return "device"
	case m&fs.ModeIrregular != 0:
		return "irregular (e.g. reparse point)"
	}
	return m.Type().String()
}

// cleanErr shortens wrapped path errors to their cause.
func cleanErr(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Op + ": " + pe.Err.Error()
	}
	return strings.TrimSpace(err.Error())
}

// throttle limits the average read rate (limits.read_rate).
type throttle struct {
	rate  float64
	begin time.Time
	bytes int64
}

func (t *throttle) start(rate float64) {
	t.rate, t.begin, t.bytes = rate, time.Now(), 0
}

func (t *throttle) wait(ctx context.Context, n int) {
	if t.rate <= 0 {
		return
	}
	t.bytes += int64(n)
	want := time.Duration(float64(t.bytes) / t.rate * float64(time.Second))
	elapsed := time.Since(t.begin)
	if elapsed > want+time.Second {
		// Idle time (enumeration) must not allow unbounded bursts.
		t.begin, t.bytes = time.Now(), 0
		return
	}
	if want > elapsed {
		timer := time.NewTimer(want - elapsed)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}
}
