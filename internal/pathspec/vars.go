package pathspec

import (
	"fmt"
	"strings"
)

// part is a piece of a pattern. Literal parts come from variable values and
// are never interpreted as wildcards, so a user directory called "a[1]" is
// matched verbatim.
type part struct {
	text string
	lit  bool
}

// Vars maps lower-cased variable names to one or more values. Multi-valued
// variables (such as UserHome) expand a pattern into one pattern per value.
type Vars map[string][]string

// Set stores values under the case-insensitive name.
func (v Vars) Set(name string, values ...string) { v[strings.ToLower(name)] = values }

// Expander resolves %Name% references.
type Expander struct {
	Vars Vars
	// Env is consulted for names not in Vars (Windows environment variables).
	// May be nil.
	Env func(name string) (string, bool)
}

// Known built-in variable names. Windows additionally accepts any
// environment variable of the collector process.
var (
	PosixVariables   = []string{"UserHome"}
	WindowsVariables = []string{"UserHome", "SystemDrive", "SystemRoot", "WinDir", "ProgramData", "ProgramFiles", "ProgramFiles(x86)"}
)

func validVarName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '(', r == ')', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

// expand resolves variables in s and returns all resulting alternatives.
func (e *Expander) expand(s string) ([][]part, error) {
	results := [][]part{{}}
	appendAll := func(p part) {
		for i := range results {
			results[i] = append(results[i], p)
		}
	}
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			appendAll(part{text: buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '%' {
			buf.WriteByte(c)
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' {
			flush()
			appendAll(part{text: "%", lit: true})
			i++
			continue
		}
		end := strings.IndexByte(s[i+1:], '%')
		if end < 0 {
			return nil, fmt.Errorf("unterminated variable reference at offset %d (use %%%% for a literal %%)", i)
		}
		name := s[i+1 : i+1+end]
		if !validVarName(name) {
			return nil, fmt.Errorf("invalid variable name %q (use %%%% for a literal %%)", name)
		}
		flush()
		values, err := e.lookup(name)
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			// Variable known but without values (e.g. no user profiles):
			// the pattern expands to nothing.
			return nil, nil
		}
		next := make([][]part, 0, len(results)*len(values))
		for _, r := range results {
			for _, v := range values {
				alt := append(append([]part(nil), r...), part{text: v, lit: true})
				next = append(next, alt)
			}
		}
		results = next
		i += end + 1
	}
	flush()
	return results, nil
}

func (e *Expander) lookup(name string) ([]string, error) {
	if e != nil && e.Vars != nil {
		if v, ok := e.Vars[strings.ToLower(name)]; ok {
			return v, nil
		}
	}
	if e != nil && e.Env != nil {
		if v, ok := e.Env(name); ok && v != "" {
			return []string{v}, nil
		}
	}
	return nil, &UnknownVariableError{Name: name}
}

// UnknownVariableError is returned when a pattern references an undefined variable.
type UnknownVariableError struct{ Name string }

func (e *UnknownVariableError) Error() string {
	return fmt.Sprintf("unknown variable %%%s%%", e.Name)
}

// checkExpander returns an expander with placeholder values for syntax checks
// independent of the host the check runs on.
func checkExpander(style Style) *Expander {
	v := Vars{}
	if style.Windows {
		v.Set("UserHome", `C:\Users\user`)
		v.Set("SystemDrive", `C:`)
		v.Set("SystemRoot", `C:\Windows`)
		v.Set("WinDir", `C:\Windows`)
		v.Set("ProgramData", `C:\ProgramData`)
		v.Set("ProgramFiles", `C:\Program Files`)
		v.Set("ProgramFiles(x86)", `C:\Program Files (x86)`)
		v.Set("AllUsersProfile", `C:\ProgramData`)
		v.Set("Public", `C:\Users\Public`)
		// Any other environment variable is accepted on Windows; it is
		// resolved at collection time.
		return &Expander{Vars: v, Env: func(string) (string, bool) { return `C:\x`, true }}
	}
	v.Set("UserHome", "/home/user")
	return &Expander{Vars: v}
}
