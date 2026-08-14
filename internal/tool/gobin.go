package tool

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrGobinResolution = errors.New("gobin resolution failed")

// goEnvFunc runs a `go env` query and returns its raw stdout plus any error.
// It exists as an injectable seam so the resolution logic can be exercised
// deterministically in tests without invoking the real Go toolchain.
type goEnvFunc func(args ...string) (string, error)

func defaultGoEnv(args ...string) (string, error) {
	out, err := exec.Command("go", args...).Output()
	return string(out), err
}

func getGobin(env goEnvFunc) (string, error) {
	out, err := env("env", "GOBIN")
	if err == nil {
		gobin := strings.TrimSpace(out)
		if gobin != "" {
			return gobin, nil
		}
	}

	// GOBIN is either empty or failed to query; fall through to GOPATH
	out, err = env("env", "GOPATH")
	if err != nil {
		return "", fmt.Errorf("failed to determine GOPATH: %w", ErrGobinResolution)
	}
	gopath := strings.TrimSpace(out)
	if gopath == "" {
		return "", fmt.Errorf("GOPATH is not set and GOBIN is empty: %w", ErrGobinResolution)
	}
	return filepath.Join(gopath, "bin"), nil
}

// GetGobin resolves the directory containing Go-installed binaries. It prefers
// the GOBIN environment variable and falls back to $GOPATH/bin. Its externally
// observable behavior is unchanged by the internal test seam: callers continue
// to use the real Go toolchain via defaultGoEnv.
func GetGobin() (string, error) {
	return getGobin(defaultGoEnv)
}
