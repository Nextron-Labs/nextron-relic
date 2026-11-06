package cli

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(args, &out, &errb)
	return code, out.String(), errb.String()
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCollectJSONContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths")
	}
	dir := t.TempDir()
	data := writeFile(t, dir, "evidence.log", "evidence\n")
	cfg := writeFile(t, dir, "c.yaml", `version: 1
job: {id: "asgard-task-7"}
collect:
  custom_artifacts:
    - name: custom.evidence
      sources:
        - os: [linux, darwin]
          paths: ['`+data+`']
limits: {min_free_space: 0, priority: normal}
`)
	outDir := filepath.Join(dir, "out") + string(os.PathSeparator)
	status := filepath.Join(dir, "status.json")
	code, stdout, stderr := runCLI(t, "collect", "-c", cfg, "-o", outDir, "--json", "--status-file", status, "-q")
	if code != result.ExitComplete {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	var res result.Result
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("stdout is not a JSON result: %v\n%s", err, stdout)
	}
	if res.Status != result.StatusComplete || res.Job.ID != "asgard-task-7" || res.Output == nil || res.Output.SHA256 == "" {
		t.Fatalf("result: %+v", res)
	}
	// -o <dir> keeps the default file name pattern.
	if filepath.Dir(res.Output.Path) != filepath.Clean(outDir) || !strings.HasPrefix(filepath.Base(res.Output.Path), "relic_") {
		t.Errorf("output path %s", res.Output.Path)
	}
	if !strings.Contains(strings.Join(res.Config.Overrides, " "), "output=") {
		t.Errorf("overrides not recorded: %v", res.Config.Overrides)
	}
	st, err := os.ReadFile(status)
	if err != nil || !bytes.Contains(st, []byte(`"status": "complete"`)) {
		t.Errorf("status file: %v %s", err, st)
	}
	zr, err := zip.OpenReader(res.Output.Path)
	if err != nil {
		t.Fatal(err)
	}
	zr.Close()
}

func TestCollectInvalidConfigStillPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFile(t, dir, "bad.yaml", "version: 1\ncollect:\n  profile: nope\n  methd: os\n")
	code, stdout, stderr := runCLI(t, "collect", "-c", cfg, "--json")
	if code != result.ExitUsage {
		t.Fatalf("exit %d", code)
	}
	var res result.Result
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("no JSON on stdout: %s", stdout)
	}
	if res.Status != result.StatusFailed || res.ExitCode != result.ExitUsage || len(res.Issues) == 0 {
		t.Errorf("result: %+v", res)
	}
	if !strings.Contains(stderr, `unknown field "methd"`) {
		t.Errorf("stderr: %s", stderr)
	}
	// Catalog errors (unknown profile) are found once the structure is valid.
	cfg = writeFile(t, dir, "bad2.yaml", "version: 1\ncollect:\n  profile: nope\n")
	code, stdout, _ = runCLI(t, "collect", "-c", cfg, "--json", "-q")
	if code != result.ExitUsage || !strings.Contains(stdout, `unknown profile`) {
		t.Errorf("exit %d: %s", code, stdout)
	}
}

func TestValidateForOtherOS(t *testing.T) {
	dir := t.TempDir()
	cfg := writeFile(t, dir, "win.yaml", `version: 1
collect:
  profile: minimal
  artifacts: [windows.ntfs.mft, linux.accounts]
  custom_artifacts:
    - name: custom.app
      sources:
        - os: [windows]
          paths: ['%ProgramData%\App\*.log']
`)
	code, stdout, _ := runCLI(t, "validate", cfg, "--os", "windows", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stdout)
	}
	var out struct {
		Valid     bool
		Issues    []map[string]interface{}
		Artifacts []string
	}
	json.Unmarshal([]byte(stdout), &out)
	joined := strings.Join(out.Artifacts, ",")
	if !out.Valid || !strings.Contains(joined, "windows.registry.system") || !strings.Contains(joined, "custom.app") || strings.Contains(joined, "linux.") {
		t.Errorf("selection: %+v", out)
	}
	if !strings.Contains(stdout, `linux.accounts\" has no source for windows`) {
		t.Errorf("expected not-applicable warning: %s", stdout)
	}

	bad := writeFile(t, dir, "bad.yaml", "version: 1\ncollect:\n  artifacts: [windows.eventlog]\n")
	code, stdout, stderr := runCLI(t, "validate", bad)
	if code != result.ExitUsage || !strings.Contains(stderr, `did you mean "windows.eventlogs"`) || !strings.Contains(stderr, "line 3") {
		t.Errorf("exit %d\n%s\n%s", code, stdout, stderr)
	}
}

func TestUsageErrorsAndInfoCommands(t *testing.T) {
	if code, _, _ := runCLI(t, "collect", "--no-such-flag"); code != result.ExitUsage {
		t.Errorf("unknown flag: exit %d", code)
	}
	if code, _, _ := runCLI(t, "collect", "--max-duration", "soon"); code != result.ExitUsage {
		t.Errorf("bad duration: exit %d", code)
	}
	code, stdout, _ := runCLI(t, "list", "artifacts", "--os", "windows", "--json")
	if code != 0 || !strings.Contains(stdout, `"windows.ntfs.mft"`) || strings.Contains(stdout, `"linux.accounts"`) {
		t.Errorf("list artifacts: %d", code)
	}
	code, stdout, _ = runCLI(t, "show", "artifact", "windows.ntfs.usnjrnl")
	if code != 0 || !strings.Contains(stdout, "trim_sparse: true") {
		t.Errorf("show artifact: %s", stdout)
	}
	code, stdout, _ = runCLI(t, "config", "show", "--profile", "minimal")
	if code != 0 || !strings.Contains(stdout, "profile: minimal") || !strings.Contains(stdout, "max_file_size: 4GiB") {
		t.Errorf("config show: %s", stdout)
	}
	// The printed configuration must itself be valid.
	dir := t.TempDir()
	p := writeFile(t, dir, "shown.yaml", stdout)
	if code, out, errs := runCLI(t, "validate", p); code != 0 {
		t.Errorf("config show output is not valid: %s %s", out, errs)
	}
	if code, stdout, _ = runCLI(t, "config", "schema"); code != 0 || !strings.Contains(stdout, `"$schema"`) {
		t.Errorf("config schema: %d", code)
	}
	if code, stdout, _ = runCLI(t, "version", "--json"); code != 0 || !strings.Contains(stdout, `"name": "relic"`) {
		t.Errorf("version: %s", stdout)
	}
}
