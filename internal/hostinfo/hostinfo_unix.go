//go:build !windows

package hostinfo

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
)

func elevated() bool { return os.Geteuid() == 0 }

func platformExpander(v pathspec.Vars) *pathspec.Expander {
	// No environment variables on Unix: patterns must not depend on the
	// environment of the collector process.
	return &pathspec.Expander{Vars: v}
}

// Home directories that belong to system accounts and are never user homes.
var notHomes = map[string]bool{
	"/": true, "/bin": true, "/sbin": true, "/usr": true, "/usr/bin": true, "/usr/sbin": true,
	"/dev": true, "/proc": true, "/sys": true, "/etc": true, "/var": true, "/run": true,
	"/nonexistent": true, "/dev/null": true, "/var/empty": true, "/tmp": true,
}

// UserHomes returns home directories of local users.
//
// Linux: /root, every directory below /home and every existing home of
// /etc/passwd (service accounts like www-data included).
// macOS: every directory below /Users and /var/root.
func UserHomes() []string {
	var cands []string
	if runtime.GOOS == "darwin" {
		cands = append(cands, globDirs("/Users/*")...)
		cands = append(cands, "/var/root")
		return dedupeDirs(cands, true)
	}
	cands = append(cands, "/root")
	cands = append(cands, globDirs("/home/*")...)
	cands = append(cands, passwdHomes("/etc/passwd")...)
	return dedupeDirs(cands, false)
}

func globDirs(pattern string) []string {
	m, _ := filepath.Glob(pattern)
	var out []string
	for _, p := range m {
		if strings.HasPrefix(filepath.Base(p), ".") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func passwdHomes(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		home := filepath.Clean(fields[5])
		if !filepath.IsAbs(home) || notHomes[home] {
			continue
		}
		out = append(out, home)
	}
	return out
}

// FreeSpace returns the bytes available to the process on the file system
// that contains dir.
func FreeSpace(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// LowerPriority reduces CPU (nice 10) and, on Linux, I/O priority
// (best-effort class, lowest level).
func LowerPriority() error {
	if err := unix.Setpriority(unix.PRIO_PROCESS, 0, 10); err != nil {
		return err
	}
	return lowerIOPriority()
}

func osVersion() string {
	if runtime.GOOS == "darwin" {
		return darwinVersion()
	}
	return linuxVersion()
}

func linuxVersion() string {
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
				f.Close()
				return strings.Trim(v, `"'`) + kernel()
			}
		}
		f.Close()
	}
	return "Linux" + kernel()
}

func kernel() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return ""
	}
	return " (kernel " + unix.ByteSliceToString(u.Release[:]) + ")"
}

func darwinVersion() string {
	data, err := os.ReadFile("/System/Library/CoreServices/SystemVersion.plist")
	if err != nil {
		return "macOS"
	}
	s := string(data)
	get := func(key string) string {
		i := strings.Index(s, "<key>"+key+"</key>")
		if i < 0 {
			return ""
		}
		rest := s[i:]
		a := strings.Index(rest, "<string>")
		b := strings.Index(rest, "</string>")
		if a < 0 || b < a {
			return ""
		}
		return rest[a+len("<string>") : b]
	}
	v := strings.TrimSpace(get("ProductName") + " " + get("ProductVersion"))
	if b := get("ProductBuildVersion"); b != "" {
		v += " (" + b + ")"
	}
	return v
}
