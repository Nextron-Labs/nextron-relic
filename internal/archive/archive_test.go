package archive

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nextron-Labs/nextron-relic/internal/pathspec"
)

func TestEntryName(t *testing.T) {
	cases := []struct {
		style pathspec.Style
		in    string
		want  string
	}{
		{pathspec.WindowsStyle, `C:\Windows\System32\config\SAM`, "files/C/Windows/System32/config/SAM"},
		{pathspec.WindowsStyle, `c:\$Extend\$UsnJrnl:$J`, "files/C/$Extend/$UsnJrnl%3A$J"},
		{pathspec.WindowsStyle, `\\srv\share\a b\x.txt`, "files/UNC/srv/share/a b/x.txt"},
		{pathspec.PosixStyle, "/var/log/syslog", "files/var/log/syslog"},
		{pathspec.PosixStyle, "/tmp/we|rd:na*me?.", "files/tmp/we%7Crd%3Ana%2Ame%3F%2E"},
		{pathspec.PosixStyle, "/tmp/100%/back\\slash", "files/tmp/100%25/back%5Cslash"},
	}
	for _, c := range cases {
		if got := EntryName(c.style, c.in); got != c.want {
			t.Errorf("%s: got %s want %s", c.in, got, c.want)
		}
	}
}

func TestWriterAtomicRenameAndHash(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.zip")
	w, err := Create(out, "auto", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("final archive must not exist before Close")
	}
	mod := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	ew, err := w.Create("files/var/log/syslog", mod, "/var/log/syslog")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(ew, "hello world\n")
	ew, _ = w.Create("files/var/log/syslog.2.gz", mod, "/var/log/syslog.2.gz")
	io.WriteString(ew, "\x1f\x8b fake gzip")
	if err := w.WriteFile("manifest.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	size, sum, err := w.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out + PartialSuffix); !os.IsNotExist(err) {
		t.Fatal("partial file should be gone")
	}
	data, _ := os.ReadFile(out)
	h := sha256.Sum256(data)
	if int64(len(data)) != size || hex.EncodeToString(h[:]) != sum {
		t.Fatal("reported size/hash do not match the file")
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(zr.File))
	}
	if !zr.File[0].Modified.Equal(mod) {
		t.Errorf("modification time not preserved: %v", zr.File[0].Modified)
	}
	if zr.File[0].Method != zip.Deflate || zr.File[1].Method != zip.Store {
		t.Errorf("auto compression should store already compressed files")
	}

	if _, err := Create(out, "auto", 5, false); err == nil {
		t.Fatal("expected ErrExists")
	}
}
