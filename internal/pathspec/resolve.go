package pathspec

import (
	"context"
	"errors"
	"io/fs"
	"os"
)

// FS is the minimal file system view needed to resolve patterns. Paths are
// absolute native paths of the pattern's style.
type FS interface {
	Lstat(name string) (fs.FileInfo, error)
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

// OSFS resolves patterns against the local operating system.
type OSFS struct{}

func (OSFS) Lstat(name string) (fs.FileInfo, error)     { return os.Lstat(name) }
func (OSFS) Stat(name string) (fs.FileInfo, error)      { return os.Stat(name) }
func (OSFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }

// Match is a path produced by resolving a pattern.
type Match struct {
	Path string
	// Info is the Lstat result: symbolic links are not followed.
	Info fs.FileInfo
	// Literal is true when the final path component was named explicitly
	// (a literal or a {a,b} alternative, not a wildcard match). Explicitly
	// named symlinks are followed by the collector.
	Literal bool
}

// ErrStop can be returned by the match callback to end resolution early.
var ErrStop = errors.New("stop resolution")

// ErrDepthLimit is reported via OnError when "**" recursion is cut off.
var ErrDepthLimit = errors.New("recursion depth limit reached")

// Resolver walks the file system for compiled patterns.
type Resolver struct {
	FS             FS
	FollowSymlinks bool
	MaxDepth       int
	// OnError receives enumeration errors other than "not exist".
	OnError func(path string, err error)
}

// Resolve calls fn for every file matching p. Directories are never reported.
// Matches that are directories at the final component are reported only for
// literal patterns (so the caller can explain why nothing was collected).
func (r *Resolver) Resolve(ctx context.Context, p *Pattern, fn func(Match) error) error {
	root := p.Root
	if p.Style.Windows {
		root += `\`
	} else {
		root = "/"
	}
	err := r.walk(ctx, p.Style, root, p.segs, 0, fn)
	if errors.Is(err, ErrStop) {
		return nil
	}
	return err
}

func (r *Resolver) report(path string, err error) {
	if err == nil || errors.Is(err, fs.ErrNotExist) || isNotDir(err) {
		return
	}
	if r.OnError != nil {
		r.OnError(path, err)
	}
}

func (r *Resolver) walk(ctx context.Context, st Style, dir string, segs []segment, depth int, fn func(Match) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	seg := segs[0]
	last := len(segs) == 1

	switch seg.kind {
	case segLiteral:
		next := st.Join(dir, seg.lit)
		if last {
			info, err := r.FS.Lstat(next)
			if err != nil {
				r.report(next, err)
				return nil
			}
			return fn(Match{Path: next, Info: info, Literal: true})
		}
		// Intermediate literal components follow symlinks like a shell does
		// (e.g. /etc -> /private/etc on macOS).
		info, err := r.FS.Stat(next)
		if err != nil {
			r.report(next, err)
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		return r.walk(ctx, st, next, segs[1:], depth, fn)

	case segGlob:
		entries, err := r.FS.ReadDir(dir)
		if err != nil {
			r.report(dir, err)
			return nil
		}
		for _, e := range entries {
			if !seg.match(e.Name()) {
				continue
			}
			next := st.Join(dir, e.Name())
			if last {
				if e.IsDir() && e.Type()&fs.ModeSymlink == 0 {
					continue
				}
				info, err := e.Info()
				if err != nil {
					r.report(next, err)
					continue
				}
				if err := fn(Match{Path: next, Info: info, Literal: seg.explicit}); err != nil {
					return err
				}
				continue
			}
			if r.traversable(e, next) || (seg.explicit && e.Type()&fs.ModeSymlink != 0 && r.isDir(next)) {
				if err := r.walk(ctx, st, next, segs[1:], depth, fn); err != nil {
					return err
				}
			}
		}
		return nil

	case segDoubleStar:
		if r.MaxDepth > 0 && depth > r.MaxDepth {
			r.report(dir, ErrDepthLimit)
			return nil
		}
		if last {
			return r.walkAll(ctx, st, dir, depth, fn)
		}
		// "**" matches zero directories ...
		if err := r.walk(ctx, st, dir, segs[1:], depth, fn); err != nil {
			return err
		}
		// ... or one or more.
		entries, err := r.FS.ReadDir(dir)
		if err != nil {
			r.report(dir, err)
			return nil
		}
		for _, e := range entries {
			next := st.Join(dir, e.Name())
			if r.traversable(e, next) {
				if err := r.walk(ctx, st, next, segs, depth+1, fn); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return nil
}

func (r *Resolver) walkAll(ctx context.Context, st Style, dir string, depth int, fn func(Match) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.MaxDepth > 0 && depth > r.MaxDepth {
		r.report(dir, ErrDepthLimit)
		return nil
	}
	entries, err := r.FS.ReadDir(dir)
	if err != nil {
		r.report(dir, err)
		return nil
	}
	for _, e := range entries {
		next := st.Join(dir, e.Name())
		if r.traversable(e, next) {
			if err := r.walkAll(ctx, st, next, depth+1, fn); err != nil {
				return err
			}
			continue
		}
		if e.IsDir() && e.Type()&fs.ModeSymlink == 0 {
			continue // non-traversable directory (junction / mount point)
		}
		info, err := e.Info()
		if err != nil {
			r.report(next, err)
			continue
		}
		if err := fn(Match{Path: next, Info: info}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Resolver) isDir(path string) bool {
	info, err := r.FS.Stat(path)
	return err == nil && info.IsDir()
}

// traversable decides whether recursion may enter a directory entry.
// Symlinks and Windows junctions/mount points (reported as irregular) are
// only followed with FollowSymlinks, which protects against loops such as
// "Application Data" junctions in Windows profiles.
func (r *Resolver) traversable(e fs.DirEntry, path string) bool {
	t := e.Type()
	if t&fs.ModeSymlink != 0 || (e.IsDir() && t&fs.ModeIrregular != 0) {
		if !r.FollowSymlinks {
			return false
		}
		info, err := r.FS.Stat(path)
		return err == nil && info.IsDir()
	}
	return e.IsDir()
}
