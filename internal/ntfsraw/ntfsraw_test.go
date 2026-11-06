package ntfsraw

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"strings"
	"testing"
)

// The test image originates from Velocidex go-ntfs (Apache-2.0), see
// testdata/README.md.
func openTestVolume(t *testing.T) *Volume {
	t.Helper()
	f, err := os.Open("testdata/test.ntfs.dd.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	v, err := Open(bytes.NewReader(data), nil, "C:")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func testFS(t *testing.T) *FS {
	v := openTestVolume(t)
	return &FS{Opener: func(drive string) (*Volume, error) { return v, nil }}
}

func TestRootListingContainsMetafiles(t *testing.T) {
	fsys := testFS(t)
	entries, err := fsys.ReadDir(`C:\`)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	for _, want := range []string{"$MFT", "$LogFile", "$Extend", "$Boot", "Folder A"} {
		if !names[want] {
			t.Errorf("root listing misses %s (got %v)", want, names)
		}
	}
	info, err := fsys.Lstat(`C:\$Extend`)
	if err != nil || !info.IsDir() {
		t.Fatalf("$Extend should be a directory: %v %v", info, err)
	}
}

func TestReadFileAndADS(t *testing.T) {
	fsys := testFS(t)
	dir := `C:\Folder A\Folder B`
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var file, ads string
	for _, e := range entries {
		if strings.Contains(e.Name(), ":") {
			ads = e.Name()
		} else if !e.IsDir() {
			file = e.Name()
		}
	}
	if file != "Hello world text document.txt" || ads != "Hello world text document.txt:goodbye.txt" {
		t.Fatalf("unexpected listing: file=%q ads=%q", file, ads)
	}

	s, err := fsys.Open(dir + `\` + file)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(io.NewSectionReader(s, 0, s.Size()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Hello world") {
		t.Errorf("unexpected content %q", data)
	}
	if s.Info.Mtime.IsZero() || s.Info.Btime.IsZero() || s.Info.MFTEntry == 0 {
		t.Errorf("missing metadata: %+v", s.Info)
	}

	// Case-insensitive lookup of an alternate data stream.
	s2, err := fsys.Open(`c:\folder a\folder b\HELLO WORLD TEXT DOCUMENT.TXT:goodbye.txt`)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(io.NewSectionReader(s2, 0, s2.Size()))
	if len(data) == 0 || s2.Info.Stream != "goodbye.txt" {
		t.Errorf("ADS: %q %+v", data, s2.Info)
	}
}

func TestReadMFT(t *testing.T) {
	fsys := testFS(t)
	s, err := fsys.Open(`C:\$MFT`)
	if err != nil {
		t.Fatal(err)
	}
	if s.Size() < 1024 {
		t.Fatalf("unexpected $MFT size %d", s.Size())
	}
	head := make([]byte, 4)
	if _, err := s.ReadAt(head, 0); err != nil {
		t.Fatal(err)
	}
	if string(head) != "FILE" {
		t.Errorf("$MFT should start with a FILE record, got %q", head)
	}
	if s.DataStart() != 0 {
		t.Errorf("$MFT is not sparse, DataStart=%d", s.DataStart())
	}
}

func TestNotFoundAndBadPaths(t *testing.T) {
	fsys := testFS(t)
	if _, err := fsys.Lstat(`C:\does\not\exist`); err == nil || !os.IsNotExist(err) {
		t.Errorf("expected not-exist, got %v", err)
	}
	if _, err := fsys.Open(`C:\$MFT:nostream`); err == nil {
		t.Errorf("expected error for missing stream")
	}
	if _, _, err := SplitDrive(`\\server\share\x`); err == nil {
		t.Errorf("UNC paths must be rejected")
	}
	if d, in, _ := SplitDrive(`\\?\c:\Windows\System32`); d != "C:" || in != "Windows/System32" {
		t.Errorf("SplitDrive: %q %q", d, in)
	}
}
