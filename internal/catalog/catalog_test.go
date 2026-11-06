package catalog

import (
	"strings"
	"testing"

	"github.com/Nextron-Labs/nextron-relic/pkg/config"
)

func TestBuiltinCatalogIsValid(t *testing.T) {
	c, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Artifacts) < 40 {
		t.Fatalf("expected a substantial catalog, got %d artifacts", len(c.Artifacts))
	}
	for _, name := range []string{"minimal", "standard", "full"} {
		if _, ok := c.Profile(name); !ok {
			t.Fatalf("missing profile %s", name)
		}
	}
}

func TestProfilesHaveArtifactsForEveryOS(t *testing.T) {
	c, _ := Builtin()
	for _, p := range []string{"minimal", "standard", "full"} {
		for _, goos := range []string{"windows", "linux", "darwin"} {
			sel, is := c.Select(config.Collect{Profile: p}, goos)
			if is.HasErrors() {
				t.Fatalf("%s/%s: %v", p, goos, is)
			}
			if len(sel.Artifacts) < 5 {
				t.Errorf("profile %s on %s selects only %d artifacts", p, goos, len(sel.Artifacts))
			}
			for _, a := range sel.Artifacts {
				for _, s := range a.Sources {
					if !s.AppliesTo(goos) {
						t.Errorf("%s: source for %v selected on %s", a.Artifact.Name, s.OS, goos)
					}
				}
			}
		}
	}
}

func TestProfilesAreNested(t *testing.T) {
	c, _ := Builtin()
	minimal, _ := c.ProfileArtifacts("minimal")
	standard, _ := c.ProfileArtifacts("standard")
	full, _ := c.ProfileArtifacts("full")
	if !(len(minimal) < len(standard) && len(standard) < len(full)) {
		t.Fatalf("expected minimal < standard < full, got %d/%d/%d", len(minimal), len(standard), len(full))
	}
	if len(full) != len(c.Artifacts) {
		t.Fatalf("full should contain all %d artifacts, has %d", len(c.Artifacts), len(full))
	}
	// Order: profile includes come first.
	for i, n := range minimal {
		if standard[i] != n {
			t.Fatalf("standard should start with the minimal artifacts in order")
		}
	}
}

func TestSelectExplicitExcludeCustom(t *testing.T) {
	c, _ := Builtin()
	col := config.Collect{
		Profile:          "minimal",
		Artifacts:        []string{"browser.*", "windows.ntfs.mft"},
		ExcludeArtifacts: []string{"browser.safari", "unix.*"},
		CustomArtifacts: []config.Artifact{{
			Name:    "custom.app",
			Sources: []config.Source{{Type: "file", OS: []string{"linux"}, Paths: []string{"/opt/app/*.log"}}},
		}},
	}
	sel, is := c.Select(col, "linux")
	if is.HasErrors() {
		t.Fatal(is)
	}
	names := map[string]bool{}
	for _, a := range sel.Artifacts {
		names[a.Artifact.Name] = true
	}
	for _, want := range []string{"linux.accounts", "browser.chrome", "browser.firefox", "custom.app"} {
		if !names[want] {
			t.Errorf("expected %s to be selected", want)
		}
	}
	for _, unwanted := range []string{"browser.safari", "unix.ssh", "windows.ntfs.mft"} {
		if names[unwanted] {
			t.Errorf("did not expect %s", unwanted)
		}
	}
	if len(sel.NotApplicable) != 1 || sel.NotApplicable[0] != "windows.ntfs.mft" {
		t.Errorf("expected windows.ntfs.mft to be reported as not applicable, got %v", sel.NotApplicable)
	}
	if last := sel.Artifacts[len(sel.Artifacts)-1]; !last.Custom {
		t.Errorf("custom artifacts should be collected last")
	}
}

func TestSelectReportsUnknownNames(t *testing.T) {
	c, _ := Builtin()
	_, is := c.Select(config.Collect{Profile: "standart", Artifacts: []string{"windows.eventlog"},
		CustomArtifacts: []config.Artifact{{Name: "windows.prefetch", Sources: []config.Source{{Paths: []string{`C:\x`}}}}}}, "windows")
	msg := is.Error()
	for _, want := range []string{`unknown profile "standart"`, `unknown artifact "windows.eventlog"`, `did you mean "windows.eventlogs"`, "already exists in the built-in catalog"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected %q in:\n%s", want, msg)
		}
	}
}
