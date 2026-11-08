package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document is a parsed configuration together with its source and position
// information for error reporting.
type Document struct {
	Config *Config
	// Raw is the original file content. For built-in defaults (FromBuiltin)
	// it is their YAML form, so that every archive carries its input
	// configuration.
	Raw []byte
	// SHA256 of Raw (hex). Empty if Raw is nil (FromConfig).
	SHA256 string
	// Source describes where the configuration came from ("file:<path>",
	// "stdin" or "builtin").
	Source string
	root   *yaml.Node
}

// MaxConfigSize bounds the size of configuration files.
const MaxConfigSize = 4 << 20

// LoadFile reads and parses a configuration file. "-" reads from stdin.
func LoadFile(path string) (*Document, Issues) {
	var (
		data   []byte
		err    error
		source = "file:" + path
	)
	if path == "-" {
		source = "stdin"
		data, err = io.ReadAll(io.LimitReader(os.Stdin, MaxConfigSize+1))
	} else {
		var f *os.File
		f, err = os.Open(path)
		if err == nil {
			data, err = io.ReadAll(io.LimitReader(f, MaxConfigSize+1))
			f.Close()
		}
	}
	if err != nil {
		var is Issues
		is.Errorf("", "cannot read configuration: %v", err)
		return nil, is
	}
	if len(data) > MaxConfigSize {
		var is Issues
		is.Errorf("", "configuration exceeds %d bytes", MaxConfigSize)
		return nil, is
	}
	return Parse(data, source)
}

// FromConfig wraps a programmatically built configuration. The document has
// no raw form; collections from it carry no config.yaml.
func FromConfig(c *Config, source string) *Document {
	return &Document{Config: c, Source: source}
}

// BuiltinHeader starts the raw form of built-in configurations.
const BuiltinHeader = "# Built-in configuration (relic collect without --config).\n" +
	"# Command line flags change it (manifest.json: config.overrides); the result\n" +
	"# is config.effective.yaml.\n"

// FromBuiltin wraps built-in defaults that were not read from a file. Raw is
// BuiltinHeader followed by the YAML form of c, and SHA256 its hash, so that
// the input configuration of a run is archived and hashed like a file.
// Parsing Raw yields c again.
func FromBuiltin(c *Config) (*Document, error) {
	data, err := c.Marshal()
	if err != nil {
		return nil, err
	}
	raw := append([]byte(BuiltinHeader), data...)
	sum := sha256.Sum256(raw)
	return &Document{Config: c, Raw: raw, SHA256: hex.EncodeToString(sum[:]), Source: "builtin"}, nil
}

var yamlLineRe = regexp.MustCompile(`^(?:yaml: )?line (\d+): (.*)$`)

// Parse parses and validates a YAML configuration. The returned document is
// nil only if the YAML could not be parsed at all.
func Parse(data []byte, source string) (*Document, Issues) {
	var issues Issues
	sum := sha256.Sum256(data)
	doc := &Document{Raw: data, SHA256: hex.EncodeToString(sum[:]), Source: source}

	var root yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			issues.Errorf("", "configuration is empty")
			return nil, issues
		}
		issues = append(issues, yamlErrorIssue(err.Error()))
		return nil, issues
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		issues.Errorf("", "configuration must contain a single YAML document")
	}
	body := &root
	if body.Kind == yaml.DocumentNode && len(body.Content) == 1 {
		body = body.Content[0]
	}
	if body.Kind != yaml.MappingNode {
		issues = append(issues, Issue{Severity: SeverityError, Line: body.Line, Column: body.Column,
			Message: "configuration must be a YAML mapping (key: value)"})
		return nil, issues
	}
	dropNulls(body)
	doc.root = body

	// Unknown keys and structural shape.
	checkKeys(body, reflect.TypeOf(Config{}), "", &issues)

	cfg := Default()
	if err := body.Decode(cfg); err != nil {
		var te *yaml.TypeError
		if errors.As(err, &te) {
			for _, msg := range te.Errors {
				issues = append(issues, doc.typeIssue(msg))
			}
		} else {
			issues = append(issues, yamlErrorIssue(err.Error()))
		}
	}
	if !hasKey(body, "version") {
		issues = append(issues, Issue{Severity: SeverityError, Path: "version", Line: body.Line, Column: body.Column,
			Message: fmt.Sprintf("is required (current version: %d)", CurrentVersion)})
		cfg.Version = CurrentVersion // reported above; avoid a second finding
	}
	cfg.applyDefaults()
	doc.Config = cfg

	for _, is := range cfg.Validate() {
		doc.locate(&is)
		issues = append(issues, is)
	}
	sortIssues(issues)
	return doc, issues
}

func sortIssues(is Issues) {
	sort.SliceStable(is, func(i, j int) bool {
		if is[i].Line != is[j].Line {
			if is[i].Line == 0 || is[j].Line == 0 {
				return is[i].Line != 0
			}
			return is[i].Line < is[j].Line
		}
		return false
	})
}

func yamlErrorIssue(msg string) Issue {
	msg = strings.TrimPrefix(msg, "yaml: ")
	if m := yamlLineRe.FindStringSubmatch(msg); m != nil {
		line, _ := strconv.Atoi(m[1])
		return Issue{Severity: SeverityError, Line: line, Message: "invalid YAML: " + m[2]}
	}
	return Issue{Severity: SeverityError, Message: "invalid YAML: " + msg}
}

var unmarshalRe = regexp.MustCompile("^cannot unmarshal (!!\\w+)(?: `(.*)`)? into (.+)$")

func (d *Document) typeIssue(msg string) Issue {
	is := Issue{Severity: SeverityError, Message: msg}
	if m := yamlLineRe.FindStringSubmatch(msg); m != nil {
		is.Line, _ = strconv.Atoi(m[1])
		is.Message = m[2]
	}
	if m := unmarshalRe.FindStringSubmatch(is.Message); m != nil {
		is.Message = fmt.Sprintf("expected %s, got %s", describeGoType(m[3]), describeYAMLTag(m[1], m[2]))
	}
	if is.Line > 0 && d.root != nil {
		is.Path, is.Column = pathAtLine(d.root, is.Line)
	}
	return is
}

func describeGoType(t string) string {
	switch {
	case strings.HasPrefix(t, "[]"):
		return "a list"
	case strings.HasPrefix(t, "map["):
		return "a mapping"
	case t == "int" || t == "int64":
		return "an integer"
	case t == "bool":
		return "true or false"
	case t == "string":
		return "a string"
	case strings.HasPrefix(t, "config."):
		return "a mapping"
	}
	return t
}

func describeYAMLTag(tag, val string) string {
	switch tag {
	case "!!seq":
		return "a list"
	case "!!map":
		return "a mapping"
	}
	if val != "" {
		return strconv.Quote(val)
	}
	return tag
}

// dropNulls removes mapping entries with null values so that "limits:" with
// no content behaves like an absent section instead of resetting defaults.
func dropNulls(n *yaml.Node) {
	switch n.Kind {
	case yaml.MappingNode:
		out := n.Content[:0]
		for i := 0; i+1 < len(n.Content); i += 2 {
			v := n.Content[i+1]
			if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
				continue
			}
			dropNulls(v)
			out = append(out, n.Content[i], v)
		}
		n.Content = out
	case yaml.SequenceNode:
		for _, c := range n.Content {
			dropNulls(c)
		}
	}
}

func hasKey(n *yaml.Node, key string) bool {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return true
		}
	}
	return false
}

var unmarshalerType = reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()

// checkKeys reports keys that do not exist in the target type.
func checkKeys(n *yaml.Node, t reflect.Type, path string, issues *Issues) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if reflect.PointerTo(t).Implements(unmarshalerType) {
		return
	}
	if n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	switch t.Kind() {
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return // reported by the decoder
		}
		fields := map[string]reflect.Type{}
		var names []string
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			fields[name] = f.Type
			names = append(names, name)
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			ft, ok := fields[k.Value]
			if !ok {
				msg := fmt.Sprintf("unknown field %q", k.Value)
				if s := suggest(k.Value, names); s != "" {
					msg += fmt.Sprintf(" (did you mean %q?)", s)
				} else {
					msg += fmt.Sprintf(" (allowed: %s)", strings.Join(names, ", "))
				}
				*issues = append(*issues, Issue{Severity: SeverityError, Path: joinPath(path, k.Value),
					Line: k.Line, Column: k.Column, Message: msg})
				continue
			}
			checkKeys(v, ft, joinPath(path, k.Value), issues)
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			return
		}
		for i, c := range n.Content {
			checkKeys(c, t.Elem(), fmt.Sprintf("%s[%d]", path, i), issues)
		}
	case reflect.Map:
		if n.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			checkKeys(n.Content[i+1], t.Elem(), joinPath(path, n.Content[i].Value), issues)
		}
	}
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

// suggest returns the closest allowed name for typos (edit distance <= 2).
func suggest(s string, names []string) string {
	best, bestD := "", 3
	for _, n := range names {
		if d := editDistance(strings.ToLower(s), n); d < bestD {
			best, bestD = n, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

var pathTokenRe = regexp.MustCompile(`([^.\[\]]+)|\[(\d+)\]`)

// locate fills line and column of an issue from its YAML path. If the exact
// node does not exist (e.g. a missing required field), the closest existing
// parent is used.
func (d *Document) locate(is *Issue) {
	if d.root == nil || is.Line > 0 {
		return
	}
	n := d.root
	line, col := n.Line, n.Column
	for _, m := range pathTokenRe.FindAllStringSubmatch(is.Path, -1) {
		var next *yaml.Node
		if m[1] != "" && n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == m[1] {
					next = n.Content[i+1]
					line, col = n.Content[i].Line, n.Content[i].Column
					break
				}
			}
		} else if m[2] != "" && n.Kind == yaml.SequenceNode {
			idx, _ := strconv.Atoi(m[2])
			if idx < len(n.Content) {
				next = n.Content[idx]
			}
		}
		if next == nil {
			break
		}
		n = next
		if n.Kind != yaml.MappingNode {
			line, col = n.Line, n.Column
		}
	}
	is.Line, is.Column = line, col
}

// pathAtLine returns the YAML path of the first value node on a line.
func pathAtLine(root *yaml.Node, line int) (string, int) {
	var walk func(n *yaml.Node, path string) (string, int, bool)
	walk = func(n *yaml.Node, path string) (string, int, bool) {
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				p := joinPath(path, k.Value)
				if k.Line == line {
					return p, v.Column, true
				}
				if r, c, ok := walk(v, p); ok {
					return r, c, true
				}
			}
		case yaml.SequenceNode:
			for i, c := range n.Content {
				p := fmt.Sprintf("%s[%d]", path, i)
				if c.Line == line && c.Kind == yaml.ScalarNode {
					return p, c.Column, true
				}
				if r, col, ok := walk(c, p); ok {
					return r, col, true
				}
			}
		}
		return "", 0, false
	}
	p, c, _ := walk(root, "")
	return p, c
}

// Marshal renders the configuration as YAML.
func (c *Config) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}

// Locate fills line and column of an issue from its YAML path.
func (d *Document) Locate(is *Issue) {
	if d != nil {
		d.locate(is)
	}
}
