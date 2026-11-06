//go:build windows

package ntfsraw

import (
	"io"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// TestLiveSystemDrive reads the live system volume. It needs an elevated
// process (GitHub Actions Windows runners are elevated) and is skipped
// otherwise.
func TestLiveSystemDrive(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires an elevated process")
	}
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	fsys := NewFS()
	defer fsys.Close()

	mft, err := fsys.Open(drive + `\$MFT`)
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 4)
	if _, err := mft.ReadAt(head, 0); err != nil || string(head) != "FILE" {
		t.Fatalf("$MFT: %q %v", head, err)
	}
	// The SYSTEM hive is locked by the kernel; raw access must read it.
	hive, err := fsys.Open(os.Getenv("SystemRoot") + `\System32\config\SYSTEM`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(io.NewSectionReader(hive, 0, 4), head); err != nil || string(head) != "regf" {
		t.Fatalf("SYSTEM hive: %q %v", head, err)
	}
	j, err := fsys.Open(drive + `\$Extend\$UsnJrnl:$J`)
	if err != nil {
		t.Logf("$UsnJrnl:$J not available: %v", err)
	} else {
		t.Logf("$J logical size %d, data starts at %d", j.Size(), j.DataStart())
	}
}
