//go:build !linux && !windows

package hostinfo

// lowerIOPriority is not available without cgo on this platform (macOS
// setiopolicy_np); only the CPU priority is lowered.
func lowerIOPriority() error { return nil }
