package catalog

import (
	"fmt"
	"strings"

	"github.com/Nextron-Labs/nextron-relic/pkg/config"
)

// Selected is an artifact chosen for collection with the sources that apply
// to the target platform.
type Selected struct {
	Artifact *config.Artifact
	Sources  []config.Source
	Custom   bool
	// Reason explains why the artifact was selected ("profile:standard",
	// "config", "custom").
	Reason string
}

// Selection is the ordered result of resolving a configuration.
type Selection struct {
	Artifacts []Selected
	// NotApplicable lists explicitly requested artifacts without sources for
	// the target platform.
	NotApplicable []string
}

// Select resolves profile, artifact names, exclusions and custom artifacts
// for the target GOOS. Errors refer to configuration paths.
func (c *Catalog) Select(col config.Collect, goos string) (*Selection, config.Issues) {
	var is config.Issues
	for i, a := range col.CustomArtifacts {
		if _, exists := c.byName[a.Name]; exists {
			is.Errorf(fmt.Sprintf("collect.custom_artifacts[%d].name", i), "artifact %q already exists in the built-in catalog; choose another name (e.g. custom.%s)", a.Name, a.Name)
		}
	}
	cat := c.withCustom(col.CustomArtifacts)

	type entry struct {
		name     string
		reason   string
		explicit bool
	}
	var entries []entry
	if col.Profile != "" {
		names, err := cat.ProfileArtifacts(col.Profile)
		if err != nil {
			is.Errorf("collect.profile", "%v (available: %s)", err, strings.Join(cat.ProfileNames(), ", "))
		}
		for _, n := range names {
			entries = append(entries, entry{n, "profile:" + col.Profile, false})
		}
	}
	for i, a := range col.Artifacts {
		m := cat.match(a)
		if len(m) == 0 {
			msg := fmt.Sprintf("unknown artifact %q", a)
			if s := cat.suggestArtifact(a); s != "" {
				msg += fmt.Sprintf(" (did you mean %q?)", s)
			}
			is.Errorf(fmt.Sprintf("collect.artifacts[%d]", i), "%s; run 'relic list artifacts' for available names", msg)
		}
		for _, n := range m {
			entries = append(entries, entry{n, "config", !strings.ContainsAny(a, "*?[")})
		}
	}
	for _, a := range col.CustomArtifacts {
		if cat.custom[a.Name] {
			entries = append(entries, entry{a.Name, "custom", true})
		}
	}

	excluded := map[string]bool{}
	for i, x := range col.ExcludeArtifacts {
		m := cat.match(x)
		if len(m) == 0 {
			is.Warnf(fmt.Sprintf("collect.exclude_artifacts[%d]", i), "exclusion %q matches no artifact", x)
		}
		for _, n := range m {
			excluded[n] = true
		}
	}

	explicit := map[string]bool{}
	for _, e := range entries {
		if e.explicit {
			explicit[e.name] = true
		}
	}
	sel := &Selection{}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.name] || excluded[e.name] {
			continue
		}
		seen[e.name] = true
		a := cat.byName[e.name]
		var srcs []config.Source
		for _, s := range a.Sources {
			if s.AppliesTo(goos) {
				srcs = append(srcs, s)
			}
		}
		if len(srcs) == 0 {
			if explicit[e.name] {
				sel.NotApplicable = append(sel.NotApplicable, e.name)
			}
			continue
		}
		sel.Artifacts = append(sel.Artifacts, Selected{Artifact: a, Sources: srcs, Custom: cat.custom[e.name], Reason: e.reason})
	}
	return sel, is
}

func (c *Catalog) suggestArtifact(name string) string {
	best, bestD := "", 4
	for _, a := range c.Artifacts {
		if d := editDistance(name, a.Name); d < bestD {
			best, bestD = a.Name, d
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
