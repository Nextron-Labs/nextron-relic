package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nextron-Labs/nextron-relic/pkg/config"
)

func TestOutputPath(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 11, 7, 21, 54, 24, 0, time.UTC)
	job := config.Job{ID: "mc/7"}
	sep := string(os.PathSeparator)
	for _, tc := range []struct {
		in, want string
		appended bool
	}{
		// Regression for issue #3: a path that does not exist and has no
		// trailing separator is a file name and gets ".zip" appended.
		{filepath.Join(dir, "relic-out"), filepath.Join(dir, "relic-out.zip"), true},
		{filepath.Join(dir, "case.v2"), filepath.Join(dir, "case.v2.zip"), true},
		{filepath.Join(dir, "{job_id}"), filepath.Join(dir, "mc_7.zip"), true},
		{filepath.Join(dir, "a.zip"), filepath.Join(dir, "a.zip"), false},
		{filepath.Join(dir, "A.ZIP"), filepath.Join(dir, "A.ZIP"), false},
		// Directories: existing, or marked by a trailing separator.
		{existing, filepath.Join(existing, "relic_host_20261107T215424Z.zip"), false},
		{filepath.Join(dir, "new") + sep, filepath.Join(dir, "new", "relic_host_20261107T215424Z.zip"), false},
		{filepath.Join(dir, "{hostname}_{timestamp}.zip"), filepath.Join(dir, "host_20261107T215424Z.zip"), false},
	} {
		got, appended, err := outputPath(tc.in, job, "host", ts)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want || appended != tc.appended {
			t.Errorf("outputPath(%q) = %q, %v; want %q, %v", tc.in, got, appended, tc.want, tc.appended)
		}
	}
}
