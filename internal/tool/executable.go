package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// isExecutable reports whether a file should be considered executable
// based on the platform-specific executable policy.
func isExecutable(name string, mode os.FileMode) bool {
	if runtime.GOOS == "windows" {
		return isExecutableWindows(name)
	}
	return isExecutablePOSIX(mode)
}

// isExecutableWindows reports whether a file is executable on Windows.
// On Windows, only .exe files are considered executable candidates.
// The extension check is case-insensitive.
func isExecutableWindows(name string) bool {
	ext := filepath.Ext(name)
	return strings.EqualFold(ext, ".exe")
}

// isExecutablePOSIX reports whether a file is executable on POSIX systems.
// On POSIX, a file is executable if at least one execute bit is set.
func isExecutablePOSIX(mode os.FileMode) bool {
	return mode&0o111 != 0
}
