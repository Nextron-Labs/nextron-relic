// Package catalog holds the built-in artifact definitions and profiles and
// turns a configuration into an ordered artifact selection.
package catalog

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Nextron-Labs/nextron-relic/pkg/config"
)

//go:embed data/*.yaml
var dataFS embed.FS

// Profile is a named selection of artifacts.
type Profile struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	// Include lists other profiles whose artifacts are included first.
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	// Artifacts are artifact names or wildcards (e.g. "windows.*").
	Artifacts []string `yaml:"artifacts" json:"artifacts"`
}

// Catalog is a set of artifacts and profiles.
type Catalog struct {
	Artifacts []*config.Artifact
	Profiles  []*Profile
	byName    map[string]*config.Artifact
	profiles  map[string]*Profile
	custom    map[string]bool
}

type file struct {
	Artifacts []config.Artifact `yaml:"artifacts"`
	Profiles  []Profile         `yaml:"profiles"`
}

var (
	builtinOnce sync.Once
	builtin     *Catalog
	builtinErr  error
)

// DefaultProfile is used when Relic runs without a configuration.
const DefaultProfile = "standard"

// Builtin returns the embedded catalog. It is parsed once.
func Builtin() (*Catalog, error) {
	builtinOnce.Do(func() { builtin, builtinErr = loadFS(dataFS) })
	return builtin, builtinErr
}

func loadFS(fsys fs.FS) (*Catalog, error) {
	files, err := fs.Glob(fsys, "data/*.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	c := &Catalog{byName: map[string]*config.Artifact{}, profiles: map[string]*Profile{}, custom: map[string]bool{}}
	for _, name := range files {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		var f file
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		if err := dec.Decode(&f); err != nil {
			return nil, fmt.Errorf("catalog %s: %w", name, err)
		}
		for i := range f.Artifacts {
			a := f.Artifacts[i]
			for j := range a.Sources {
				if a.Sources[j].Type == "" {
					a.Sources[j].Type = config.SourceTypeFile
				}
			}
			var is config.Issues
			config.ValidateArtifact(&is, a.Name, a)
			if is.HasErrors() {
				return nil, fmt.Errorf("catalog %s: invalid artifact %s:\n%s", name, a.Name, is.Errors().Error())
			}
			if _, dup := c.byName[a.Name]; dup {
				return nil, fmt.Errorf("catalog %s: duplicate artifact %s", name, a.Name)
			}
			c.Artifacts = append(c.Artifacts, &a)
			c.byName[a.Name] = &a
		}
		for i := range f.Profiles {
			p := f.Profiles[i]
			if _, dup := c.profiles[p.Name]; dup {
				return nil, fmt.Errorf("catalog %s: duplicate profile %s", name, p.Name)
			}
			c.Profiles = append(c.Profiles, &p)
			c.profiles[p.Name] = &p
		}
	}
	// Verify profile references.
	for _, p := range c.Profiles {
		if _, err := c.expandProfile(p.Name, nil); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Artifact returns an artifact by exact name.
func (c *Catalog) Artifact(name string) (*config.Artifact, bool) {
	a, ok := c.byName[name]
	return a, ok
}

// Profile returns a profile by name.
func (c *Catalog) Profile(name string) (*Profile, bool) {
	p, ok := c.profiles[name]
	return p, ok
}

// IsCustom reports whether an artifact was added from the configuration.
func (c *Catalog) IsCustom(name string) bool { return c.custom[name] }

// ProfileNames returns all profile names in definition order.
func (c *Catalog) ProfileNames() []string {
	var out []string
	for _, p := range c.Profiles {
		out = append(out, p.Name)
	}
	return out
}

// withCustom returns a copy of the catalog extended by custom artifacts.
func (c *Catalog) withCustom(custom []config.Artifact) *Catalog {
	n := &Catalog{
		Artifacts: append([]*config.Artifact(nil), c.Artifacts...),
		Profiles:  c.Profiles,
		byName:    make(map[string]*config.Artifact, len(c.byName)+len(custom)),
		profiles:  c.profiles,
		custom:    map[string]bool{},
	}
	for k, v := range c.byName {
		n.byName[k] = v
	}
	for i := range custom {
		a := custom[i]
		if _, exists := n.byName[a.Name]; exists {
			continue // reported by Select
		}
		n.Artifacts = append(n.Artifacts, &a)
		n.byName[a.Name] = &a
		n.custom[a.Name] = true
	}
	return n
}

// match returns artifact names matching a name or wildcard in catalog order.
func (c *Catalog) match(pattern string) []string {
	if !strings.ContainsAny(pattern, "*?[") {
		if _, ok := c.byName[pattern]; ok {
			return []string{pattern}
		}
		return nil
	}
	var out []string
	for _, a := range c.Artifacts {
		if ok, _ := path.Match(pattern, a.Name); ok {
			out = append(out, a.Name)
		}
	}
	return out
}

func (c *Catalog) expandProfile(name string, stack []string) ([]string, error) {
	p, ok := c.profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q", name)
	}
	for _, s := range stack {
		if s == name {
			return nil, fmt.Errorf("profile include cycle: %s -> %s", strings.Join(stack, " -> "), name)
		}
	}
	stack = append(stack, name)
	var out []string
	for _, inc := range p.Include {
		names, err := c.expandProfile(inc, stack)
		if err != nil {
			return nil, err
		}
		out = append(out, names...)
	}
	for _, a := range p.Artifacts {
		m := c.match(a)
		if len(m) == 0 {
			return nil, fmt.Errorf("profile %s references unknown artifact %q", name, a)
		}
		out = append(out, m...)
	}
	return out, nil
}

// ProfileArtifacts returns the de-duplicated artifact names of a profile.
func (c *Catalog) ProfileArtifacts(name string) ([]string, error) {
	names, err := c.expandProfile(name, nil)
	if err != nil {
		return nil, err
	}
	return dedupe(names), nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range in {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}
