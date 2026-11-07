package verify

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

type entry struct{ name, data string }

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func record(src, archivePath, data string) result.FileRecord {
	return result.FileRecord{Artifact: "custom.x", SourcePath: src, ArchivePath: archivePath, Status: result.FileCollected,
		Size: int64(len(data)), BytesStored: int64(len(data)), Hashes: map[string]string{"sha256": sha(data)}}
}

// build writes an archive with the given files, records and manifest summary.
func build(t *testing.T, files []entry, recs []result.FileRecord, collected int, extra ...entry) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	add := func(name, data string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(data))
	}
	for _, e := range files {
		add(e.name, e.data)
	}
	var lines []string
	for _, r := range recs {
		b, _ := json.Marshal(r)
		lines = append(lines, string(b))
	}
	add("files.jsonl", strings.Join(lines, "\n")+"\n")
	add("relic.log", "{}\n")
	add("config.effective.yaml", "version: 1\n")
	m := result.Result{SchemaVersion: result.SchemaVersion, Tool: result.Tool{Name: "relic", Version: "test"},
		Status: result.StatusComplete, Host: result.Host{Hostname: "h", OS: "linux"}}
	m.Summary.FilesCollected = collected
	mb, _ := json.Marshal(m)
	add("manifest.json", string(mb))
	for _, e := range extra {
		add(e.name, e.data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

func TestArchiveOK(t *testing.T) {
	p := build(t,
		[]entry{{"files/etc/passwd", "root:x:0:0\n"}, {"files/var/log/auth.log", "login\n"}},
		[]result.FileRecord{
			record("/etc/passwd", "files/etc/passwd", "root:x:0:0\n"),
			record("/var/log/auth.log", "files/var/log/auth.log", "login\n"),
			{Artifact: "custom.x", SourcePath: "/etc/shadow", Status: result.FileFailed, Reason: "permission denied"},
		}, 2)
	rep, err := Archive(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.FilesVerified != 2 || rep.Records["failed"] != 1 || rep.Collection == nil || rep.Collection.Host != "h" {
		t.Fatalf("report: %+v", rep)
	}
	// The archive hash from the collection result must match.
	rep, _ = Archive(p, Options{Expect: &result.Output{Size: rep.ArchiveSize, SHA256: strings.ToUpper(rep.ArchiveSHA256)}})
	if !rep.OK {
		t.Errorf("expected match: %v", rep.Problems)
	}
	rep, _ = Archive(p, Options{Expect: &result.Output{Size: 1, SHA256: sha("other")}})
	if rep.OK || len(rep.Problems) != 2 {
		t.Errorf("expected size and hash mismatch: %v", rep.Problems)
	}
}

func TestArchiveProblems(t *testing.T) {
	p := build(t,
		[]entry{{"files/etc/passwd", "root:x:0:0 (modified)\n"}, {"files/etc/hosts", "127.0.0.1\n"}},
		[]result.FileRecord{
			record("/etc/passwd", "files/etc/passwd", "root:x:0:0\n"),
			record("/etc/group", "files/etc/group", "root:x:0:\n"),
		}, 3)
	rep, err := Archive(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(rep.Problems, "\n")
	for _, want := range []string{
		"files/etc/passwd (/etc/passwd): 22 bytes stored, record says 11",
		"files/etc/passwd (/etc/passwd): sha256 mismatch",
		"files/etc/group (/etc/group) is recorded but missing",
		"files/etc/hosts is stored but has no record",
		"lists 2 collected files, the manifest summary 3",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
	if rep.OK || rep.FilesVerified != 0 {
		t.Errorf("report: %+v", rep)
	}
}

func TestArchiveNotZip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.zip")
	os.WriteFile(p, []byte("not a zip"), 0o644)
	rep, err := Archive(p, Options{})
	if err != nil || rep.OK || !strings.Contains(rep.Problems[0], "not a readable ZIP") {
		t.Fatalf("%v %+v", err, rep)
	}
	if _, err := Archive(filepath.Join(t.TempDir(), "missing.zip"), Options{}); err == nil {
		t.Error("missing file must be an error")
	}
}
