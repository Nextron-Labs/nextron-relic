//go:build windows

package ntfsraw

import (
	"fmt"
	"os"
)

// Supported reports whether raw volume access is available on this platform.
const Supported = true

// OpenDevice opens the NTFS volume of a drive letter ("C:") for raw reading.
// It requires administrative privileges (LocalSystem is sufficient).
func OpenDevice(drive string) (*Volume, error) {
	f, err := os.Open(`\\.\` + drive)
	if err != nil {
		return nil, fmt.Errorf("open volume %s for raw access: %w (requires administrative privileges)", drive, err)
	}
	v, err := Open(f, f, drive)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("volume %s: %w", drive, err)
	}
	return v, nil
}
