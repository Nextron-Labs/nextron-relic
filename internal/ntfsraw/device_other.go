//go:build !windows

package ntfsraw

import (
	"errors"
	"fmt"
)

// Supported reports whether raw volume access is available on this platform.
const Supported = false

// ErrUnsupported is returned when raw NTFS volume access is requested on a
// platform other than Windows.
var ErrUnsupported = errors.New("raw NTFS access is only supported on Windows")

// OpenDevice is not available on this platform.
func OpenDevice(drive string) (*Volume, error) {
	return nil, fmt.Errorf("volume %s: %w", drive, ErrUnsupported)
}
