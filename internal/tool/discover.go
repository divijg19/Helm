package tool

import (
	"os"
	"path/filepath"
	"sort"
)

func discover(gobin string) ([]candidate, error) {
	return discoverWithPolicy(gobin, isExecutable)
}

// discoverWithPolicy is the internal discovery primitive with an injectable
// executable predicate, so tests can exercise the real discovery loop (reads,
// directory skips, path joining, sorting) under any platform policy
// regardless of the host runtime.GOOS. Production passes isExecutable.
func discoverWithPolicy(gobin string, executable func(name string, mode os.FileMode) bool) ([]candidate, error) {
	entries, err := os.ReadDir(gobin)
	if err != nil {
		return nil, err
	}

	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		toolPath := filepath.Join(gobin, entry.Name())

		info, err := entry.Info()
		if err != nil || !executable(entry.Name(), info.Mode()) {
			continue
		}

		candidates = append(candidates, candidate{
			name: entry.Name(),
			path: toolPath,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].name < candidates[j].name
	})

	return candidates, nil
}
