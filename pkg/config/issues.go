package config

import (
	"fmt"
	"strings"
)

// Severity of a validation issue.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is a single validation finding. Path uses dotted YAML notation,
// e.g. "collect.custom_artifacts[0].sources[1].paths[2]".
type Issue struct {
	Severity Severity `json:"severity"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
	Message  string   `json:"message"`
}

func (i Issue) String() string {
	var b strings.Builder
	b.WriteString(string(i.Severity))
	if i.Line > 0 {
		fmt.Fprintf(&b, " (line %d", i.Line)
		if i.Column > 0 {
			fmt.Fprintf(&b, ", column %d", i.Column)
		}
		b.WriteString(")")
	}
	b.WriteString(": ")
	if i.Path != "" {
		b.WriteString(i.Path)
		b.WriteString(": ")
	}
	b.WriteString(i.Message)
	return b.String()
}

// Issues is a list of validation findings.
type Issues []Issue

func (is *Issues) Errorf(path, format string, args ...interface{}) {
	*is = append(*is, Issue{Severity: SeverityError, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (is *Issues) Warnf(path, format string, args ...interface{}) {
	*is = append(*is, Issue{Severity: SeverityWarning, Path: path, Message: fmt.Sprintf(format, args...)})
}

// HasErrors reports whether any issue is an error.
func (is Issues) HasErrors() bool {
	for _, i := range is {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors returns only the error-level issues.
func (is Issues) Errors() Issues {
	var out Issues
	for _, i := range is {
		if i.Severity == SeverityError {
			out = append(out, i)
		}
	}
	return out
}

func (is Issues) Error() string {
	var lines []string
	for _, i := range is {
		lines = append(lines, i.String())
	}
	return strings.Join(lines, "\n")
}
