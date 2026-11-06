package pathspec

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type segKind int

const (
	segLiteral segKind = iota
	segGlob
	segDoubleStar
)

type segment struct {
	kind segKind
	// explicit is true when the segment only enumerates names ({a,b}) and
	// contains no real wildcard (*, ?, [...]).
	explicit bool
	lit      string         // segLiteral
	body     string         // regexp body (no anchors) for segLiteral/segGlob
	re       *regexp.Regexp // segGlob, anchored
}

func (s segment) match(name string) bool {
	switch s.kind {
	case segLiteral:
		return name == s.lit
	case segGlob:
		return s.re.MatchString(name)
	}
	return false
}

// Pattern is a compiled, fully expanded path pattern.
type Pattern struct {
	// Text is the expanded pattern in native form (for display/records).
	Text  string
	Root  string
	Style Style
	segs  []segment
	full  *regexp.Regexp
}

// HasWildcards reports whether the pattern contains any non-literal segment.
func (p *Pattern) HasWildcards() bool {
	for _, s := range p.segs {
		if s.kind != segLiteral {
			return true
		}
	}
	return false
}

// Match reports whether an absolute native path matches the whole pattern.
func (p *Pattern) Match(path string) bool {
	return p.full.MatchString(normalizeForMatch(path, p.Style))
}

func normalizeForMatch(path string, style Style) string {
	if style.Windows {
		path = strings.ReplaceAll(path, `\`, "/")
		path = strings.TrimPrefix(path, "//?/")
	}
	if style.Windows && len(path) >= 2 && path[1] == ':' {
		path = strings.ToUpper(path[:1]) + path[1:]
	}
	return path
}

// Compile expands braces and variables in pattern and compiles each result.
// A pattern may expand into zero patterns (e.g. %UserHome% with no users).
func Compile(pattern string, style Style, exp *Expander) ([]*Pattern, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, errors.New("empty pattern")
	}
	texts, err := expandSeparatorBraces(pattern, style)
	if err != nil {
		return nil, err
	}
	var out []*Pattern
	for _, t := range texts {
		alts, err := exp.expand(t)
		if err != nil {
			return nil, err
		}
		for _, parts := range alts {
			p, err := compileParts(parts, style)
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		}
	}
	return out, nil
}

// Check validates the syntax of a pattern for a style without touching the
// local system. Variables are replaced with representative placeholder values.
func Check(pattern string, style Style) error {
	_, err := Compile(pattern, style, checkExpander(style))
	return err
}

type rr struct {
	r   rune
	lit bool
}

func compileParts(parts []part, style Style) (*Pattern, error) {
	var runes []rr
	for _, p := range parts {
		for _, r := range p.text {
			runes = append(runes, rr{r, p.lit})
		}
	}
	// Split into segments on separators (literal or not).
	var segsRaw [][]rr
	var cur []rr
	leadingSeps := 0
	seenNonSep := false
	for _, x := range runes {
		if style.IsSep(x.r) {
			if !seenNonSep {
				leadingSeps++
			}
			segsRaw = append(segsRaw, cur)
			cur = nil
			continue
		}
		seenNonSep = true
		cur = append(cur, x)
	}
	segsRaw = append(segsRaw, cur)

	text := runesText(runes)
	p := &Pattern{Style: style}

	// Determine root.
	var rest [][]rr
	if style.Windows {
		switch {
		case leadingSeps >= 2:
			// UNC: \\server\share\...
			var comps [][]rr
			for _, s := range segsRaw {
				if len(s) > 0 {
					comps = append(comps, s)
				}
			}
			if len(comps) < 2 {
				return nil, fmt.Errorf("pattern %q: UNC paths need a server and share", text)
			}
			if hasMeta(comps[0]) || hasMeta(comps[1]) {
				return nil, fmt.Errorf("pattern %q: wildcards are not allowed in the UNC server or share", text)
			}
			p.Root = `\\` + runesText(comps[0]) + `\` + runesText(comps[1])
			rest = comps[2:]
		default:
			first := segsRaw[0]
			ft := runesText(first)
			if len(ft) != 2 || ft[1] != ':' || !isLetter(rune(ft[0])) || hasMeta(first) {
				return nil, fmt.Errorf("pattern %q must be absolute and start with a drive letter (e.g. C:\\ or %%SystemDrive%%\\)", text)
			}
			p.Root = strings.ToUpper(ft[:1]) + `:`
			rest = segsRaw[1:]
		}
	} else {
		if leadingSeps == 0 {
			return nil, fmt.Errorf("pattern %q must be absolute (start with / or %%UserHome%%)", text)
		}
		rest = segsRaw
	}

	full := strings.Builder{}
	if style.CaseInsensitive {
		full.WriteString("(?i)")
	}
	full.WriteString("^")
	full.WriteString(regexp.QuoteMeta(strings.ReplaceAll(p.Root, `\`, "/")))

	for i, s := range rest {
		if len(s) == 0 {
			continue // duplicate separators
		}
		t := runesText(s)
		if t == "." {
			continue
		}
		if t == ".." {
			return nil, fmt.Errorf("pattern %q: '..' is not allowed", text)
		}
		if t == "**" && !s[0].lit && !s[1].lit {
			p.segs = append(p.segs, segment{kind: segDoubleStar})
			if i == len(rest)-1 {
				full.WriteString("(?:/.*)?")
			} else {
				full.WriteString("(?:/[^/]+)*")
			}
			continue
		}
		body, glob, err := globBody(s, style)
		if err != nil {
			return nil, fmt.Errorf("pattern %q: %v", text, err)
		}
		seg := segment{body: body, explicit: !hasWildcard(s)}
		if glob {
			prefix := ""
			if style.CaseInsensitive {
				prefix = "(?i)"
			}
			re, err := regexp.Compile(prefix + "^" + body + "$")
			if err != nil {
				return nil, fmt.Errorf("pattern %q: %v", text, err)
			}
			seg.kind, seg.re = segGlob, re
		} else {
			seg.kind, seg.lit = segLiteral, t
		}
		p.segs = append(p.segs, seg)
		full.WriteString("/")
		full.WriteString(body)
	}
	if len(p.segs) == 0 {
		return nil, fmt.Errorf("pattern %q must name files below the root", text)
	}
	full.WriteString("$")
	re, err := regexp.Compile(full.String())
	if err != nil {
		return nil, fmt.Errorf("pattern %q: %v", text, err)
	}
	p.full = re

	// Native text representation.
	var b strings.Builder
	if style.Windows {
		b.WriteString(p.Root)
	}
	for _, s := range rest {
		if len(s) == 0 {
			continue
		}
		b.WriteString(style.Sep())
		b.WriteString(runesText(s))
	}
	p.Text = b.String()
	return p, nil
}

func isLetter(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }

func runesText(rs []rr) string {
	var b strings.Builder
	for _, x := range rs {
		b.WriteRune(x.r)
	}
	return b.String()
}

func hasWildcard(rs []rr) bool {
	for _, x := range rs {
		if !x.lit && strings.ContainsRune("*?[", x.r) {
			return true
		}
	}
	return false
}

func hasMeta(rs []rr) bool {
	for _, x := range rs {
		if !x.lit && strings.ContainsRune("*?[{", x.r) {
			return true
		}
	}
	return false
}

// globBody converts a segment into a regexp body. glob reports whether the
// segment contains any wildcard.
func globBody(s []rr, style Style) (body string, glob bool, err error) {
	any := "[^/]"
	if style.Windows {
		// Wildcards never match the ':' that separates alternate data streams.
		any = "[^/:]"
	}
	var b strings.Builder
	pos, glob, err := convert(s, 0, false, any, &b)
	if err != nil {
		return "", false, err
	}
	if pos != len(s) {
		return "", false, fmt.Errorf("unexpected '%c'", s[pos].r)
	}
	return b.String(), glob, nil
}

// convert translates runes starting at i. Inside a brace group (inGroup) it
// stops at ',' or '}' and returns their position.
func convert(s []rr, i int, inGroup bool, any string, b *strings.Builder) (int, bool, error) {
	glob := false
	for i < len(s) {
		x := s[i]
		if x.lit {
			b.WriteString(regexp.QuoteMeta(string(x.r)))
			i++
			continue
		}
		switch x.r {
		case '*':
			b.WriteString(any + "*")
			glob = true
			i++
		case '?':
			b.WriteString(any)
			glob = true
			i++
		case '[':
			j, cls, err := convertClass(s, i)
			if err != nil {
				return i, glob, err
			}
			b.WriteString(cls)
			glob = true
			i = j
		case '{':
			glob = true
			b.WriteString("(?:")
			i++
			for {
				next, g, err := convert(s, i, true, any, b)
				if err != nil {
					return next, glob, err
				}
				_ = g
				if next >= len(s) {
					return next, glob, errors.New("unclosed '{'")
				}
				if s[next].r == ',' {
					b.WriteString("|")
					i = next + 1
					continue
				}
				// '}'
				b.WriteString(")")
				i = next + 1
				break
			}
		case ',', '}':
			if inGroup {
				return i, glob, nil
			}
			b.WriteString(regexp.QuoteMeta(string(x.r)))
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(x.r)))
			i++
		}
	}
	return i, glob, nil
}

func convertClass(s []rr, i int) (int, string, error) {
	// s[i] == '['
	j := i + 1
	var b strings.Builder
	b.WriteString("[")
	if j < len(s) && !s[j].lit && (s[j].r == '!' || s[j].r == '^') {
		b.WriteString("^")
		j++
	}
	first := true
	for ; j < len(s); j++ {
		x := s[j]
		if x.r == ']' && !first && !x.lit {
			b.WriteString("]")
			return j + 1, b.String(), nil
		}
		first = false
		switch x.r {
		case '\\', '[', ']', '^':
			b.WriteString(`\`)
			b.WriteRune(x.r)
		default:
			b.WriteRune(x.r)
		}
	}
	return j, "", errors.New("unclosed '['")
}

// expandSeparatorBraces expands brace groups that contain a path separator,
// e.g. "/etc/{ssh/sshd_config,sudoers}". Groups without separators are kept
// and become regexp alternations inside a single segment.
func expandSeparatorBraces(s string, style Style) ([]string, error) {
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		depth, j := 0, i
		for ; j < len(s); j++ {
			if s[j] == '{' {
				depth++
			} else if s[j] == '}' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if j >= len(s) {
			return nil, fmt.Errorf("pattern %q: unclosed '{'", s)
		}
		content := s[i+1 : j]
		if !strings.ContainsAny(content, "/") && !(style.Windows && strings.Contains(content, `\`)) {
			continue
		}
		var out []string
		for _, alt := range splitTopLevel(content) {
			sub, err := expandSeparatorBraces(s[:i]+alt+s[j+1:], style)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
		return out, nil
	}
	return []string{s}, nil
}

func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}
