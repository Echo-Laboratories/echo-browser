//go:build !windows

package chrome

// DisplaySize is unknown off Windows without extra deps.
func DisplaySize() (int, int) { return 0, 0 }
