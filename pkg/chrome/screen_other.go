//go:build !windows && !darwin && !linux

package chrome

// DisplaySize is unknown on this OS without extra deps.
func DisplaySize() (int, int) { return 0, 0 }
