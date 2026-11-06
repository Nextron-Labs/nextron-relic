package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestExamplesAreValid keeps the shipped example configurations valid for
// the platform their file name refers to.
func TestExamplesAreValid(t *testing.T) {
	files, _ := filepath.Glob("../../examples/*.yaml")
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	for _, f := range files {
		goos := "linux"
		switch base := filepath.Base(f); {
		case strings.Contains(base, "windows"):
			goos = "windows"
		case strings.Contains(base, "macos"):
			goos = "darwin"
		}
		if code, out, errs := runCLI(t, "validate", f, "--os", goos); code != 0 {
			t.Errorf("%s (%s): %s %s", f, goos, out, errs)
		}
	}
}
