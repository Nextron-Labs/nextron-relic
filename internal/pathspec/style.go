// Package pathspec implements Relic's path patterns: variable expansion
// (%SystemRoot%, %UserHome%, ...), brace alternatives, shell-style wildcards,
// recursive "**" segments and an efficient resolver that only reads the
// directories a pattern actually needs.
package pathspec

import "strings"

// Style describes the path conventions of a target platform. Patterns are
// compiled for a specific style so that Windows patterns can be handled (and
// unit tested) on any host.
type Style struct {
	Windows         bool
	CaseInsensitive bool
}

var (
	PosixStyle   = Style{}
	DarwinStyle  = Style{CaseInsensitive: true}
	WindowsStyle = Style{Windows: true, CaseInsensitive: true}
)

// StyleFor returns the path style for a GOOS value.
func StyleFor(goos string) Style {
	switch goos {
	case "windows":
		return WindowsStyle
	case "darwin":
		return DarwinStyle
	default:
		return PosixStyle
	}
}

// IsSep reports whether r separates path components in this style.
func (s Style) IsSep(r rune) bool { return r == '/' || (s.Windows && r == '\\') }

// Sep returns the native separator.
func (s Style) Sep() string {
	if s.Windows {
		return `\`
	}
	return "/"
}

// Join appends name to dir using the native separator.
func (s Style) Join(dir, name string) string {
	if dir == "" {
		return name
	}
	if strings.HasSuffix(dir, s.Sep()) || (!s.Windows && strings.HasSuffix(dir, "/")) {
		return dir + name
	}
	return dir + s.Sep() + name
}

// Key returns a normalized form of p for de-duplication and comparisons.
func (s Style) Key(p string) string {
	if s.Windows {
		p = strings.ReplaceAll(p, "/", `\`)
	}
	if s.CaseInsensitive {
		p = strings.ToLower(p)
	}
	return p
}

// Split splits an absolute path into its root and components.
//
//	Windows: `C:\Windows\x`     -> "C:", ["Windows", "x"]
//	         `\\srv\share\x`    -> `\\srv\share`, ["x"]
//	Posix:   "/etc/passwd"      -> "", ["etc", "passwd"]
func (s Style) Split(p string) (root string, comps []string) {
	if s.Windows {
		p = strings.ReplaceAll(p, "/", `\`)
		p = strings.TrimPrefix(p, `\\?\`)
		if strings.HasPrefix(p, `\\`) {
			rest := strings.Split(strings.TrimPrefix(p, `\\`), `\`)
			if len(rest) >= 2 {
				root = `\\` + rest[0] + `\` + rest[1]
				rest = rest[2:]
			}
			return root, nonEmpty(rest)
		}
		if len(p) >= 2 && p[1] == ':' {
			root = strings.ToUpper(p[:2])
			p = p[2:]
		}
		return root, nonEmpty(strings.Split(p, `\`))
	}
	return "", nonEmpty(strings.Split(p, "/"))
}

func nonEmpty(in []string) []string {
	out := in[:0]
	for _, c := range in {
		if c != "" && c != "." {
			out = append(out, c)
		}
	}
	return out
}
