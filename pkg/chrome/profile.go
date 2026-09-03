package chrome

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultProfileRoot is where named persistent profiles live.
func DefaultProfileRoot() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base, _ = os.UserCacheDir()
		}
		return filepath.Join(base, "EchoBrowser", "profiles")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "EchoBrowser", "profiles")
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "echobrowser", "profiles")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "echobrowser", "profiles")
	}
}

// ResolveUserDataDir returns the Chrome --user-data-dir path.
// name is a profile folder (default "default"). It is sanitized against traversal.
func ResolveUserDataDir(name string, ephemeral bool) (string, error) {
	if ephemeral {
		return os.MkdirTemp("", "echo-chrome-*")
	}
	if name == "" {
		name = "default"
	}
	// Treat both / and \ as separators so a Windows-style path cannot
	// slip through on Unix (CI) or the reverse.
	normalized := strings.ReplaceAll(name, `\`, "/")
	clean := filepath.Base(filepath.FromSlash(normalized))
	clean = strings.TrimSpace(clean)
	if clean == "" || clean == "." || clean == ".." {
		return "", fmt.Errorf("chrome: invalid profile name %q", name)
	}
	if strings.ContainsAny(clean, `/\`) {
		return "", fmt.Errorf("chrome: invalid profile name %q", name)
	}
	dir := filepath.Join(DefaultProfileRoot(), clean)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("chrome: profile dir: %w", err)
	}
	return dir, nil
}
