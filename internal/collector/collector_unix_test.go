//go:build !windows

package collector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Nextron-Labs/nextron-relic/internal/source"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

// fixture creates a small directory tree and returns its root.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel string, data string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("logs/app.log", "line1\nline2\n")
	write("logs/app.log.1", "old\n")
	write("logs/big.log", strings.Repeat("x", 10000))
	write("logs/sub/deep.log", "deep\n")
	write("logs/skip/ignored.log", "ignored\n")
	write("etc/conf", "key=value\n")
	if err := os.Symlink(filepath.Join(root, "etc/conf"), filepath.Join(root, "logs/link.log")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "logs/fifo.log"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCollectEndToEnd(t *testing.T) {
	root := fixture(t)
	out := filepath.Join(t.TempDir(), "out.zip")
	c := baseConfig(t, out,
		custom("custom.logs", root+"/logs/**"),
		custom("custom.again", root+"/logs/app.log", root+"/etc/conf", root+"/logs/link.log"),
	)
	c.Collect.ExcludePaths = []string{root + "/logs/skip/**"}
	c.Limits.MaxFileSize = 5000
	res := runCollect(t, context.Background(), c, nil)

	if res.Output == nil {
		t.Fatalf("no archive: %+v", res)
	}
	// big.log is skipped by max_file_size -> partial.
	if res.Status != result.StatusPartial || res.ExitCode != result.ExitPartial {
		t.Fatalf("status %s (%s)", res.Status, res.StatusReason)
	}
	v := readArchive(t, out)
	if v.manifest.Status != res.Status || v.manifest.Summary != res.Summary {
		t.Errorf("manifest differs from result")
	}
	for _, r := range v.records {
		if r.Status != result.FileCollected {
			continue
		}
		data := v.files[r.ArchivePath]
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != r.Hashes["sha256"] || int64(len(data)) != r.BytesStored || r.Hashes["md5"] == "" {
			t.Errorf("hash/size mismatch for %s", r.SourcePath)
		}
		if r.Method != source.MethodOS || r.Times.Modified == nil {
			t.Errorf("missing method/times for %s", r.SourcePath)
		}
	}
	if r := v.record(t, "logs/big.log"); r.Status != result.FileSkipped || !strings.Contains(r.Reason, "max_file_size") {
		t.Errorf("big.log: %+v", r)
	}
	if r := v.record(t, "logs/fifo.log"); r.Status != result.FileSkipped || !strings.Contains(r.Reason, "named pipe") {
		t.Errorf("fifo must be skipped without blocking: %+v", r)
	}
	// Wildcard-matched symlink is skipped, explicitly named one is followed.
	if r := v.record(t, "logs/link.log"); r.Status != result.FileSkipped || r.SymlinkTarget == "" {
		t.Errorf("link.log via wildcard: %+v", r)
	}
	for _, r := range v.records {
		if strings.Contains(r.SourcePath, "/skip/") {
			t.Errorf("excluded path collected: %s", r.SourcePath)
		}
	}
	// Duplicates across artifacts are collected once.
	if res.Summary.FilesDuplicate < 1 {
		t.Errorf("expected duplicates, got %+v", res.Summary)
	}
	if r := v.record(t, "etc/conf"); r.Artifact != "custom.again" || r.Status != result.FileCollected {
		t.Errorf("etc/conf: %+v", r)
	}
	want := "files" + filepath.ToSlash(root) + "/logs/sub/deep.log"
	if string(v.files[want]) != "deep\n" {
		t.Errorf("missing %s in archive", want)
	}
	for _, name := range []string{ManifestName, FilesName, LogName, ConfigEffectiveName} {
		if _, ok := v.files[name]; !ok {
			t.Errorf("archive misses %s", name)
		}
	}
	if _, err := os.Stat(out + ".partial"); !os.IsNotExist(err) {
		t.Errorf("partial file left behind")
	}
	if entries, _ := filepath.Glob(filepath.Join(filepath.Dir(out), ".relic-*")); len(entries) != 0 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

func TestCompleteStatusAndTruncate(t *testing.T) {
	root := fixture(t)
	out := filepath.Join(t.TempDir(), "out.zip")
	c := baseConfig(t, out, custom("custom.logs", root+"/logs/{app.log,big.log}"))
	c.Limits.MaxFileSize = 4096
	c.Limits.Oversize = config.OversizeTruncate
	res := runCollect(t, context.Background(), c, nil)
	if res.Summary.FilesTruncated != 1 || res.Status != result.StatusPartial {
		t.Fatalf("expected one truncated file and partial status: %+v %s", res.Summary, res.StatusReason)
	}
	v := readArchive(t, out)
	r := v.record(t, "big.log")
	if r.BytesStored != 4096 || len(v.files[r.ArchivePath]) != 4096 {
		t.Errorf("truncated size: %+v", r)
	}

	out2 := filepath.Join(t.TempDir(), "out.zip")
	c2 := baseConfig(t, out2, custom("custom.logs", root+"/logs/app.log"))
	res = runCollect(t, context.Background(), c2, nil)
	if res.Status != result.StatusComplete || res.ExitCode != 0 {
		t.Fatalf("expected complete: %s %s", res.Status, res.StatusReason)
	}
}

func TestLimitsStopCollection(t *testing.T) {
	root := fixture(t)
	out := filepath.Join(t.TempDir(), "out.zip")
	c := baseConfig(t, out, custom("custom.logs", root+"/logs/**"), custom("custom.etc", root+"/etc/*"))
	c.Limits.MaxFiles = 2
	res := runCollect(t, context.Background(), c, nil)
	if res.Summary.FilesCollected != 2 || len(res.Limits) != 1 || res.Limits[0].Limit != "max_files" {
		t.Fatalf("max_files: %+v %+v", res.Summary, res.Limits)
	}
	if res.Artifacts[1].Status != result.ArtifactSkipped {
		t.Errorf("second artifact should be skipped after the limit: %+v", res.Artifacts[1])
	}

	out = filepath.Join(t.TempDir(), "out.zip")
	c = baseConfig(t, out, custom("custom.logs", root+"/logs/**"))
	c.Limits.MaxTotalSize = 20
	res = runCollect(t, context.Background(), c, nil)
	if res.Summary.BytesCollected > 20 || res.Limits[0].Limit != "max_total_size" || res.Status != result.StatusPartial {
		t.Fatalf("max_total_size: %+v %+v", res.Summary, res.Limits)
	}
}

func TestCancelAndTimeout(t *testing.T) {
	root := fixture(t)
	big := filepath.Join(root, "huge.bin")
	if err := os.WriteFile(big, bytes.Repeat([]byte{1}, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	// Interrupted while reading a throttled file: archive is finalized,
	// the file is recorded as aborted, status canceled.
	out := filepath.Join(t.TempDir(), "out.zip")
	c := baseConfig(t, out, custom("custom.big", big), custom("custom.logs", root+"/logs/*.log"))
	c.Limits.ReadRate = 1 << 20 // 1 MiB/s -> ~4s for the file
	ctx, cancel := context.WithCancelCause(context.Background())
	time.AfterFunc(700*time.Millisecond, func() { cancel(ErrInterrupted) })
	res := runCollect(t, ctx, c, nil)
	if res.Status != result.StatusCanceled || res.ExitCode != result.ExitCanceled || res.Output == nil {
		t.Fatalf("cancel: %s %s %+v", res.Status, res.StatusReason, res.Output)
	}
	v := readArchive(t, out)
	if r := v.record(t, "huge.bin"); r.Status != result.FileAborted || r.BytesStored == 0 || r.BytesStored >= 4<<20 {
		t.Errorf("aborted record: %+v", r)
	}
	if v.manifest.Status != result.StatusCanceled {
		t.Errorf("manifest status %s", v.manifest.Status)
	}
	if res.Artifacts[1].Status != result.ArtifactSkipped {
		t.Errorf("remaining artifacts should be skipped: %+v", res.Artifacts[1])
	}

	// max_duration: partial with a limit event.
	out = filepath.Join(t.TempDir(), "out.zip")
	c = baseConfig(t, out, custom("custom.big", big))
	c.Limits.ReadRate = 1 << 20
	c.Limits.MaxDuration = config.Duration(500 * time.Millisecond)
	res = runCollect(t, context.Background(), c, nil)
	if res.Status != result.StatusPartial || len(res.Limits) == 0 || res.Limits[0].Limit != "max_duration" {
		t.Fatalf("timeout: %s %s %+v", res.Status, res.StatusReason, res.Limits)
	}
}

func TestDryRunAndFailures(t *testing.T) {
	root := fixture(t)
	dir := t.TempDir()
	c := baseConfig(t, filepath.Join(dir, "out.zip"), custom("custom.logs", root+"/logs/*.log"))
	res := runCollect(t, context.Background(), c, func(o *Options) { o.DryRun = true })
	if res.Output != nil || res.Summary.FilesCollected == 0 || !res.DryRun {
		t.Fatalf("dry run: %+v", res)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("dry run must not write files: %v", entries)
	}

	// Unreadable output location -> failed.
	c = baseConfig(t, "/proc/nonexistent/relic/out.zip", custom("custom.logs", root+"/logs/*.log"))
	if runtime.GOOS == "darwin" {
		c.Output.Path = "/System/relic-test/out.zip"
	}
	res = runCollect(t, context.Background(), c, nil)
	if res.Status != result.StatusFailed || res.ExitCode != result.ExitFailed {
		t.Fatalf("expected failed: %s %s", res.Status, res.StatusReason)
	}

	// Unknown artifact -> invalid configuration (exit 4).
	c = baseConfig(t, filepath.Join(dir, "x.zip"), custom("custom.logs", root+"/logs/app.log"))
	c.Collect.Artifacts = []string{"no.such.artifact"}
	res = runCollect(t, context.Background(), c, nil)
	if res.ExitCode != result.ExitUsage || len(res.Issues) == 0 {
		t.Fatalf("expected usage error: %+v", res)
	}

	// Existing output without overwrite -> failed, original untouched.
	existing := filepath.Join(dir, "exists.zip")
	os.WriteFile(existing, []byte("keep"), 0o644)
	c = baseConfig(t, existing, custom("custom.logs", root+"/logs/app.log"))
	res = runCollect(t, context.Background(), c, nil)
	if data, _ := os.ReadFile(existing); res.Status != result.StatusFailed || string(data) != "keep" {
		t.Fatalf("overwrite protection: %s", res.Status)
	}
}

func TestUnreadableFileMakesPartialAndRecordsReason(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := fixture(t)
	locked := filepath.Join(root, "logs", "locked.log")
	os.WriteFile(locked, []byte("secret"), 0o000)
	out := filepath.Join(t.TempDir(), "out.zip")
	c := baseConfig(t, out, custom("custom.logs", root+"/logs/{app,locked}.log"))
	res := runCollect(t, context.Background(), c, nil)
	if res.Status != result.StatusPartial || res.Summary.FilesFailed != 1 {
		t.Fatalf("%s %+v", res.Status, res.Summary)
	}
	v := readArchive(t, out)
	if r := v.record(t, "locked.log"); r.Status != result.FileFailed || !strings.Contains(r.Reason, "permission denied") || r.ArchivePath != "" {
		t.Errorf("locked: %+v", r)
	}
	if res.Artifacts[0].Status != result.ArtifactPartial {
		t.Errorf("artifact status %s", res.Artifacts[0].Status)
	}
}
