package cli

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Nextron-Labs/nextron-relic/internal/verify"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
}

// usable accepts exit code 0 or 1: the outcome of collecting system
// artifacts depends on the privileges of the test process.
const usable = -1

// collectJSON runs collect with --json and returns the parsed result.
func collectJSON(t *testing.T, wantCode int, args ...string) (result.Result, string) {
	t.Helper()
	code, stdout, stderr := runCLI(t, append([]string{"collect", "--json", "--min-free-space", "0", "--priority", "normal"}, args...)...)
	if code != wantCode && !(wantCode == usable && (code == result.ExitComplete || code == result.ExitPartial)) {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", code, wantCode, stdout, stderr)
	}
	var res result.Result
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("stdout is not a result: %v\n%s", err, stdout)
	}
	return res, stderr
}

func artifactNames(res result.Result) []string {
	var out []string
	for _, a := range res.Artifacts {
		out = append(out, a.Name)
	}
	return out
}

func TestCollectPathFlagSelectsOnlyPaths(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	writeFile(t, dir, "a.log", "a\n")
	writeFile(t, dir, "b.log", "bb\n")
	writeFile(t, dir, "c.gz", "ccc\n")
	out := filepath.Join(dir, "out") + "/"

	res, stderr := collectJSON(t, 0, "--path", filepath.Join(dir, "*"), "--exclude-path", filepath.Join(dir, "*.gz"),
		"-o", out, "--job-id", "IR-42", "--job-name", "triage", "--label", "analyst=jd", "--label", "case=x=y",
		"--hash", "md5,SHA256", "--hash", "sha1", "--compression", "store", "--read-rate", "1GiB", "--max-files", "10")

	if names := artifactNames(res); len(names) != 1 || names[0] != PathsArtifact {
		t.Fatalf("only %s must be selected without --profile, got %v", PathsArtifact, names)
	}
	if res.Summary.FilesCollected != 2 || res.Config.Profile != "" {
		t.Errorf("summary %+v, profile %q", res.Summary, res.Config.Profile)
	}
	if res.Job.ID != "IR-42" || res.Job.Name != "triage" || res.Job.Labels["analyst"] != "jd" || res.Job.Labels["case"] != "x=y" {
		t.Errorf("job: %+v", res.Job)
	}
	if !strings.Contains(stderr, "no profile selected") {
		t.Errorf("expected a note about the missing profile: %s", stderr)
	}
	ov := strings.Join(res.Config.Overrides, " ")
	for _, want := range []string{"path+=", "exclude_path+=", "job_id=IR-42", "label+=analyst=jd", "hashes=md5,sha256,sha1", "compression=store", "read_rate=1GiB", "max_files=10"} {
		if !strings.Contains(ov, want) {
			t.Errorf("override %q not recorded: %v", want, res.Config.Overrides)
		}
	}
	// The archive verifies, and the records carry all three hashes.
	rep, err := verify.Archive(res.Output.Path, verify.Options{Expect: res.Output})
	if err != nil || !rep.OK || rep.FilesVerified != 2 {
		t.Fatalf("verify: %v %+v", err, rep)
	}
	recs := readRecords(t, res.Output.Path)
	if len(recs) != 2 || len(recs[0].Hashes) != 3 {
		t.Errorf("records: %+v", recs)
	}
}

func TestCollectArtifactFlagWithAndWithoutProfile(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	art := "unix.ssh"
	res, _ := collectJSON(t, usable, "-a", art, "--dry-run")
	if names := artifactNames(res); len(names) != 1 || names[0] != art {
		t.Errorf("-a without --profile must select only %s, got %v", art, names)
	}
	res, _ = collectJSON(t, usable, "-a", art, "-p", "minimal", "--dry-run")
	if len(res.Artifacts) < 2 || res.Config.Profile != "minimal" {
		t.Errorf("-a with --profile must add to the profile: %v", artifactNames(res))
	}
	// Without selection flags the default profile is used.
	res, _ = collectJSON(t, usable, "--dry-run", "-o", dir+"/")
	if res.Config.Profile != "standard" {
		t.Errorf("default profile: %q", res.Config.Profile)
	}
}

func TestCollectFlagErrors(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--label", "novalue"}, "expected KEY=VALUE"},
		{[]string{"--hash", "crc32"}, `unsupported hash algorithm "crc32"`},
		{[]string{"--path", "/var/../etc/passwd"}, "'..' is not allowed"},
		{[]string{"--priority", "high"}, `invalid value "high"`},
		{[]string{"--compression-level", "12"}, "between 1"},
		{[]string{"--read-rate", "fast"}, "--read-rate"},
		{[]string{"-v", "--log-level", "warn"}, "--verbose conflicts"},
		{[]string{"--progress-interval", "soon"}, "--progress-interval"},
		{[]string{"--log-file", filepath.Join(dir, "missing", "x.log")}, "--log-file"},
	} {
		code, _, stderr := runCLI(t, append([]string{"collect", "--dry-run", "-p", "minimal"}, tc.args...)...)
		if code != result.ExitUsage || !strings.Contains(stderr, tc.want) {
			t.Errorf("%v: exit %d, stderr %q (want %q)", tc.args, code, stderr, tc.want)
		}
	}
}

func TestCollectLogFile(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	writeFile(t, dir, "a.log", "a\n")
	logPath := filepath.Join(dir, "relic.jsonl")
	writeFile(t, dir, "relic.jsonl", `{"msg":"previous run"}`+"\n")
	res, _ := collectJSON(t, 0, "--path", filepath.Join(dir, "a.log"), "-o", filepath.Join(dir, "out.zip"),
		"-q", "-v", "--log-file", logPath, "--progress-interval", "0")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if lines[0] != `{"msg":"previous run"}` {
		t.Errorf("log file must be appended to: %q", lines[0])
	}
	var sawDebug, sawStarted bool
	for _, l := range lines[1:] {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %s", l)
		}
		ts, _ := m["time"].(string)
		if tm, err := time.Parse(time.RFC3339Nano, ts); err != nil || !strings.HasSuffix(ts, "Z") || tm.IsZero() {
			t.Errorf("time must be UTC RFC 3339: %q", ts)
		}
		sawDebug = sawDebug || m["level"] == "DEBUG"
		sawStarted = sawStarted || m["msg"] == "collection started"
	}
	if !sawDebug || !sawStarted {
		t.Errorf("--log-file must get debug messages despite -q: %s", data)
	}
	// -v also puts debug messages into the archive's relic.log.
	zr, err := zip.OpenReader(res.Output.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "relic.log" {
			continue
		}
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if !strings.Contains(string(b), `"level":"DEBUG"`) {
			t.Errorf("relic.log has no debug messages with -v: %s", b)
		}
	}
}

func readRecords(t *testing.T, archive string) []result.FileRecord {
	t.Helper()
	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var out []result.FileRecord
	for _, f := range zr.File {
		if f.Name != "files.jsonl" {
			continue
		}
		rc, _ := f.Open()
		sc := bufio.NewScanner(rc)
		for sc.Scan() {
			var r result.FileRecord
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				t.Fatal(err)
			}
			out = append(out, r)
		}
		rc.Close()
	}
	return out
}

// Issue #6: runs without --config archive the built-in configuration as
// config.yaml, and the manifest hash resolves to it.
func TestCollectBuiltinConfigArchived(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	writeFile(t, dir, "a.log", "a\n")
	res, _ := collectJSON(t, 0, "--path", filepath.Join(dir, "a.log"), "-o", filepath.Join(dir, "out.zip"))
	if res.Config.Source != "builtin" || len(res.Config.SHA256) != 64 {
		t.Fatalf("config info: %+v", res.Config)
	}
	if !strings.Contains(strings.Join(res.Config.Overrides, " "), "profile= ") {
		t.Errorf("dropping the default profile must be recorded: %v", res.Config.Overrides)
	}
	zr, err := zip.OpenReader(res.Output.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var raw []byte
	for _, f := range zr.File {
		if f.Name == "config.yaml" {
			rc, _ := f.Open()
			raw, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if raw == nil {
		t.Fatal("archive has no config.yaml")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != res.Config.SHA256 {
		t.Error("config.yaml does not match config.sha256 in the result")
	}
	doc, issues := config.Parse(raw, "archive")
	if issues.HasErrors() || doc.Config.Collect.Profile != "standard" {
		t.Errorf("config.yaml must be the built-in defaults: %v %+v", issues, doc)
	}
	rep, err := verify.Archive(res.Output.Path, verify.Options{Expect: res.Output})
	if err != nil || !rep.OK {
		t.Fatalf("verify: %v %+v", err, rep)
	}
}
