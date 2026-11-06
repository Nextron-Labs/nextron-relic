package pathspec

import (
	"context"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"time"
)

// memFS is an in-memory file system keyed by native paths of a style.
type memFS struct {
	style Style
	files map[string]memEntry // key: style.Key(path)
}

type memEntry struct {
	name    string // real-case path
	dir     bool
	symlink string
	irr     bool // junction
}

func newMemFS(style Style, paths ...string) *memFS {
	m := &memFS{style: style, files: map[string]memEntry{}}
	for _, p := range paths {
		dir := strings.HasSuffix(p, "/") || strings.HasSuffix(p, `\`)
		p = strings.TrimRight(p, `/\`)
		m.add(p, memEntry{dir: dir})
	}
	return m
}

func (m *memFS) add(p string, e memEntry) {
	e.name = p
	m.files[m.style.Key(p)] = e
	// add parents
	for {
		i := strings.LastIndexAny(p, m.sep())
		if i <= 0 {
			break
		}
		p = p[:i]
		if m.style.Windows && len(p) == 2 {
			break
		}
		k := m.style.Key(p)
		if _, ok := m.files[k]; !ok {
			m.files[k] = memEntry{name: p, dir: true}
		}
	}
}

func (m *memFS) sep() string {
	if m.style.Windows {
		return `\`
	}
	return "/"
}

type memInfo struct {
	n string
	e memEntry
}

func (i memInfo) Name() string { return i.n }
func (i memInfo) Size() int64  { return 1 }
func (i memInfo) Mode() fs.FileMode {
	switch {
	case i.e.symlink != "":
		return fs.ModeSymlink
	case i.e.irr:
		return fs.ModeDir | fs.ModeIrregular
	case i.e.dir:
		return fs.ModeDir
	}
	return 0
}
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.Mode().IsDir() }
func (i memInfo) Sys() any           { return nil }

type memDirEntry struct{ memInfo }

func (d memDirEntry) Type() fs.FileMode          { return d.Mode().Type() }
func (d memDirEntry) Info() (fs.FileInfo, error) { return d.memInfo, nil }

func (m *memFS) norm(p string) string {
	p = strings.TrimRight(p, `/\`)
	k := m.style.Key(p)
	// Resolve symlinks in parent components like a real file system.
	for lk, e := range m.files {
		if e.symlink != "" && strings.HasPrefix(k, lk+m.style.Key(m.sep())) {
			return m.norm(e.symlink + k[len(lk):])
		}
	}
	return k
}

func (m *memFS) Lstat(p string) (fs.FileInfo, error) {
	e, ok := m.files[m.norm(p)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return memInfo{base(e.name), e}, nil
}

func (m *memFS) Stat(p string) (fs.FileInfo, error) {
	e, ok := m.files[m.norm(p)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if e.symlink != "" {
		return m.Stat(e.symlink)
	}
	if e.irr {
		e.irr = false
	}
	return memInfo{base(e.name), e}, nil
}

func (m *memFS) ReadDir(p string) ([]fs.DirEntry, error) {
	if s, err := m.Stat(p); err != nil || !s.IsDir() {
		return nil, fs.ErrNotExist
	}
	prefix := m.norm(p) + m.style.Key(m.sep())
	if m.style.Windows && len(m.norm(p)) == 2 {
		prefix = m.norm(p) + `\`
	}
	var out []fs.DirEntry
	for k, e := range m.files {
		if strings.HasPrefix(k, prefix) && !strings.Contains(k[len(prefix):], m.style.Key(m.sep())) && k != prefix {
			out = append(out, memDirEntry{memInfo{base(e.name), e}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func base(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	return p[i+1:]
}

func resolveAll(t *testing.T, fsys FS, style Style, exp *Expander, pattern string) []string {
	t.Helper()
	pats, err := Compile(pattern, style, exp)
	if err != nil {
		t.Fatalf("compile %q: %v", pattern, err)
	}
	r := &Resolver{FS: fsys, MaxDepth: 16}
	var out []string
	for _, p := range pats {
		err := r.Resolve(context.Background(), p, func(m Match) error {
			out = append(out, m.Path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestWindowsResolve(t *testing.T) {
	fsys := newMemFS(WindowsStyle,
		`C:\Windows\System32\config\SAM`,
		`C:\Windows\System32\config\SYSTEM`,
		`C:\Windows\System32\config\SYSTEM.LOG1`,
		`C:\Windows\System32\config\SOFTWARE`,
		`C:\Windows\System32\config\COMPONENTS`,
		`C:\Windows\System32\winevt\Logs\Security.evtx`,
		`C:\Windows\System32\winevt\Logs\System.evtx`,
		`C:\Windows\System32\winevt\Logs\notes.txt`,
		`C:\Windows\System32\Tasks\Microsoft\Windows\Defrag\ScheduledDefrag`,
		`C:\Windows\System32\Tasks\Evil`,
		`C:\Users\alice\NTUSER.DAT`,
		`C:\Users\bob [admin]\NTUSER.DAT`,
		`C:\Users\alice\AppData\Local\Google\Chrome\User Data\Default\History`,
		`C:\Users\alice\AppData\Local\Google\Chrome\User Data\Profile 1\History`,
		`C:\Users\alice\AppData\Local\Google\Chrome\User Data\Profile 1\Bookmarks`,
	)
	// Junction loop in a profile must not be followed.
	fsys.add(`C:\Users\alice\Application Data`, memEntry{dir: true, irr: true})
	fsys.add(`C:\Users\alice\Application Data\loop.txt`, memEntry{})

	exp := &Expander{Vars: Vars{}}
	exp.Vars.Set("SystemRoot", `C:\Windows`)
	exp.Vars.Set("UserHome", `C:\Users\alice`, `C:\Users\bob [admin]`)

	cases := []struct {
		pattern string
		want    []string
	}{
		{`%SystemRoot%\System32\config\{SAM,SYSTEM,SOFTWARE}`, []string{
			`C:\Windows\System32\config\SAM`, `C:\Windows\System32\config\SOFTWARE`, `C:\Windows\System32\config\SYSTEM`}},
		{`%systemroot%\system32\CONFIG\system.LOG{,1,2}`, []string{`C:\Windows\System32\config\SYSTEM.LOG1`}},
		{`%SystemRoot%\System32\winevt\Logs\*.evtx`, []string{
			`C:\Windows\System32\winevt\Logs\Security.evtx`, `C:\Windows\System32\winevt\Logs\System.evtx`}},
		{`%SystemRoot%\System32\Tasks\**`, []string{
			`C:\Windows\System32\Tasks\Evil`, `C:\Windows\System32\Tasks\Microsoft\Windows\Defrag\ScheduledDefrag`}},
		// Variable values are literal: "[admin]" is not a character class.
		{`%UserHome%\NTUSER.DAT`, []string{`C:\Users\alice\NTUSER.DAT`, `C:\Users\bob [admin]\NTUSER.DAT`}},
		{`%UserHome%\AppData\Local\Google\Chrome\User Data\*\{History,Bookmarks}`, []string{
			`C:\Users\alice\AppData\Local\Google\Chrome\User Data\Default\History`,
			`C:\Users\alice\AppData\Local\Google\Chrome\User Data\Profile 1\Bookmarks`,
			`C:\Users\alice\AppData\Local\Google\Chrome\User Data\Profile 1\History`}},
		{`C:/Users/*/NTUSER.DAT`, []string{`C:\Users\alice\NTUSER.DAT`, `C:\Users\bob [admin]\NTUSER.DAT`}},
		{`C:\Users\alice\**\*.txt`, nil},
	}
	for _, c := range cases {
		got := resolveAll(t, fsys, WindowsStyle, exp, c.pattern)
		// Literal components keep the pattern's case; Windows is case
		// insensitive, so compare normalized keys.
		for i := range got {
			got[i] = WindowsStyle.Key(got[i])
		}
		for i := range c.want {
			c.want[i] = WindowsStyle.Key(c.want[i])
		}
		sort.Strings(got)
		sort.Strings(c.want)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s\n got: %q\nwant: %q", c.pattern, got, c.want)
		}
	}
}

func TestPosixResolveAndSymlinks(t *testing.T) {
	fsys := newMemFS(PosixStyle,
		"/var/log/syslog", "/var/log/syslog.1", "/var/log/auth.log", "/var/log/journal/abc/system.journal",
		"/var/log/apache2/access.log", "/etc/passwd", "/home/a/.bash_history", "/root/.bash_history",
		"/etc/cron.d/job", "/etc/cron.daily/logrotate",
	)
	fsys.add("/var/log/link", memEntry{symlink: "/etc"})
	exp := &Expander{Vars: Vars{}}
	exp.Vars.Set("UserHome", "/home/a", "/root")

	got := resolveAll(t, fsys, PosixStyle, exp, "/var/log/**")
	want := []string{"/var/log/apache2/access.log", "/var/log/auth.log", "/var/log/journal/abc/system.journal", "/var/log/link", "/var/log/syslog", "/var/log/syslog.1"}
	sort.Strings(got)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q want %q", got, want)
	}
	got = resolveAll(t, fsys, PosixStyle, exp, "%UserHome%/.{bash_history,zsh_history}")
	if len(got) != 2 {
		t.Errorf("expected 2 histories, got %q", got)
	}
	got = resolveAll(t, fsys, PosixStyle, exp, "/etc/cron.{d,daily}/*")
	if len(got) != 2 {
		t.Errorf("cron: %q", got)
	}
	// Case sensitive on Linux.
	if got := resolveAll(t, fsys, PosixStyle, exp, "/ETC/passwd"); len(got) != 0 {
		t.Errorf("linux must be case sensitive: %q", got)
	}
	// Separator in brace group.
	got = resolveAll(t, fsys, PosixStyle, exp, "/{etc/passwd,var/log/auth.log}")
	if len(got) != 2 {
		t.Errorf("brace with separator: %q", got)
	}
	// Symlink traversal with FollowSymlinks.
	pats, _ := Compile("/var/log/**/passwd", PosixStyle, exp)
	r := &Resolver{FS: fsys, FollowSymlinks: true, MaxDepth: 8}
	var follow []string
	_ = r.Resolve(context.Background(), pats[0], func(m Match) error { follow = append(follow, m.Path); return nil })
	if len(follow) != 1 || follow[0] != "/var/log/link/passwd" {
		t.Errorf("follow symlinks: %q", follow)
	}
}

func TestMatchFullPath(t *testing.T) {
	cases := []struct {
		style   Style
		pattern string
		path    string
		want    bool
	}{
		{PosixStyle, "/var/log/journal/**", "/var/log/journal/x/system.journal", true},
		{PosixStyle, "/var/log/journal/**", "/var/log/syslog", false},
		{PosixStyle, "/**/*.gz", "/var/log/a.gz", true},
		{PosixStyle, "/var/**/b", "/var/b", true},
		{PosixStyle, "/var/*.log", "/var/x/a.log", false},
		{WindowsStyle, `C:\Users\*\AppData\**\*.tmp`, `c:\users\Bob\AppData\Local\x\y.TMP`, true},
		{WindowsStyle, `C:\$Extend\$UsnJrnl:$J`, `C:\$Extend\$UsnJrnl:$J`, true},
		{WindowsStyle, `C:\$Extend\*`, `C:\$Extend\$UsnJrnl:$J`, false}, // '*' does not match ':'
	}
	for _, c := range cases {
		pats, err := Compile(c.pattern, c.style, &Expander{})
		if err != nil {
			t.Fatal(err)
		}
		if got := pats[0].Match(c.path); got != c.want {
			t.Errorf("%s ~ %s = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestCheckErrors(t *testing.T) {
	cases := []struct {
		style   Style
		pattern string
		want    string
	}{
		{PosixStyle, "var/log/*", "must be absolute"},
		{PosixStyle, "/var/log/[abc", "unclosed '['"},
		{PosixStyle, "/var/log/{a,b", "unclosed '{'"},
		{PosixStyle, "/var/%Nope%/x", "unknown variable %Nope%"},
		{PosixStyle, "/var/50%/x", "unterminated variable"},
		{WindowsStyle, `\Windows\x`, "drive letter"},
		{WindowsStyle, `*:\x`, "drive letter"},
		{PosixStyle, "/var/../etc/passwd", "'..' is not allowed"},
		{PosixStyle, "/", "must name files"},
	}
	for _, c := range cases {
		err := Check(c.pattern, c.style)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want error containing %q", c.pattern, err, c.want)
		}
	}
	for _, ok := range []string{"/var/100%%/x", "%UserHome%/.ssh/*", "/etc/{a,b}/c[0-9]?"} {
		if err := Check(ok, PosixStyle); err != nil {
			t.Errorf("%q: unexpected error %v", ok, err)
		}
	}
	for _, ok := range []string{`%SystemDrive%\$Extend\$UsnJrnl:$J`, `%LOCALAPPDATA%\x\*.log`, `\\srv\share\x\*.log`} {
		if err := Check(ok, WindowsStyle); err != nil {
			t.Errorf("%q: unexpected error %v", ok, err)
		}
	}
}

func TestSplit(t *testing.T) {
	root, comps := WindowsStyle.Split(`C:\Windows\System32\config\SAM`)
	if root != "C:" || strings.Join(comps, "/") != "Windows/System32/config/SAM" {
		t.Errorf("got %q %q", root, comps)
	}
	root, comps = WindowsStyle.Split(`\\?\d:\x\$UsnJrnl:$J`)
	if root != "D:" || strings.Join(comps, "/") != "x/$UsnJrnl:$J" {
		t.Errorf("got %q %q", root, comps)
	}
	root, comps = PosixStyle.Split("/etc//ssh/./sshd_config")
	if root != "" || strings.Join(comps, "/") != "etc/ssh/sshd_config" {
		t.Errorf("got %q %q", root, comps)
	}
}
