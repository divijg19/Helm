package tool

import (
	"debug/buildinfo"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"testing"
)

func validBuildInfo() *buildinfo.BuildInfo {
	return &buildinfo.BuildInfo{
		Path: "example.com/test/cmd/tool",
		Main: debug.Module{Path: "example.com/test", Version: "v1.0.0"},
	}
}

func TestVerify_Executable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "good")
	if err := os.WriteFile(path, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := []Tool{
		{name: "good", path: path, info: validBuildInfo()},
	}
	results := Verify(tools)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Healthy {
		t.Errorf("expected healthy, got: %s", results[0].Error)
	}
}

func TestVerify_NotExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	tools := []Tool{
		{name: "readonly", path: path, info: validBuildInfo()},
	}
	results := Verify(tools)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Healthy {
		t.Error("expected unhealthy for non-executable file")
	}
}

func TestVerify_MissingFile(t *testing.T) {
	tools := []Tool{
		{name: "missing", path: "/nonexistent/gobin/missing", info: validBuildInfo()},
	}
	results := Verify(tools)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Healthy {
		t.Error("expected unhealthy for missing file")
	}
}

func TestVerify_EmptyPackagePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emptypkg")
	if err := os.WriteFile(path, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := []Tool{
		{name: "emptypkg", path: path, info: &buildinfo.BuildInfo{
			Path: "",
			Main: debug.Module{Path: "example.com/emptypkg", Version: "v1.0.0"},
		}},
	}
	results := Verify(tools)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Healthy {
		t.Error("expected unhealthy for empty package path")
	}
}

// TestVerify_EmptyVersionPassesVerify documents the preserved unknown-version
// behavior: Tool.Version() returns "unknown" for missing metadata, which is
// != "", so the former `t.Version() == ""` check in Verify could never
// trigger and was removed in v1.9.2. Missing-version tools remain healthy.
func TestVerify_EmptyVersionPassesVerify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "noversion")
	if err := os.WriteFile(path, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := []Tool{
		{name: "noversion", path: path, info: &buildinfo.BuildInfo{
			Path: "example.com/noversion",
			Main: debug.Module{Path: "example.com/noversion", Version: ""},
		}},
	}
	results := Verify(tools)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Healthy {
		t.Error("established behavior: empty-version tool passes verify (Version() returns 'unknown')")
	}
}

// TestVerify_UsesInstalledFilename is the regression test for the v1.9.2
// verification fix: executability must be judged from the file actually
// present at t.Path() (via filepath.Base), not from the logical tool name,
// which may differ from the installed filename. It exercises the production
// Verify() path. On Windows hosts the .exe basename is required for a healthy
// verdict; this test would fail if Verify() passed t.Name() (extensionless)
// instead of the installed filename.
func TestVerify_UsesInstalledFilename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool.exe")
	if err := os.WriteFile(path, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := []Tool{
		{name: "tool", path: path, info: validBuildInfo()},
	}
	results := Verify(tools)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Healthy {
		t.Errorf("expected healthy: logical name is %q but installed file %q is executable on this platform (%s)", "tool", path, results[0].Error)
	}
}

func TestIsExecutable(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		mode      os.FileMode
		wantWin   bool
		wantPosix bool
	}{
		{"windows: .exe with no execute bits", "tool.exe", 0o644, true, false},
		{"windows: .EXE uppercase", "TOOL.EXE", 0o644, true, false},
		{"windows: .ExE mixed case", "tool.ExE", 0o644, true, false},
		{"windows: no extension", "tool", 0o755, false, true},
		{"windows: .bat extension", "tool.bat", 0o755, false, true},
		{"windows: .cmd extension", "tool.cmd", 0o755, false, true},
		{"posix: execute bit set", "tool", 0o755, false, true},
		{"posix: no execute bits", "tool", 0o644, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWin := isExecutableWindows(tt.filename)
			gotPosix := isExecutablePOSIX(tt.mode)
			if gotWin != tt.wantWin {
				t.Errorf("isExecutableWindows(%q) = %v, want %v", tt.filename, gotWin, tt.wantWin)
			}
			if gotPosix != tt.wantPosix {
				t.Errorf("isExecutablePOSIX(%q, %#o) = %v, want %v", tt.filename, tt.mode, gotPosix, tt.wantPosix)
			}
		})
	}
}

// TestIsExecutable_DiscoverySemantics exercises the discovery candidate
// predicate on a representative GOBIN directory without invoking the real
// discover() (which is bound to the host runtime.GOOS). It is a semantics
// test of the predicate rules, not a discovery integration test.
func TestIsExecutable_DiscoverySemantics(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	exeFile := filepath.Join(tmpDir, "tool.exe")
	if err := os.WriteFile(exeFile, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	batFile := filepath.Join(tmpDir, "tool.bat")
	if err := os.WriteFile(batFile, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmdFile := filepath.Join(tmpDir, "tool.cmd")
	if err := os.WriteFile(cmdFile, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	noExtFile := filepath.Join(tmpDir, "tool")
	if err := os.WriteFile(noExtFile, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	dirFile := filepath.Join(tmpDir, "somedir")
	if err := os.Mkdir(dirFile, 0o755); err != nil {
		t.Fatal(err)
	}

	// Test Windows policy via isExecutableWindows
	windowsCandidates := discoverOS(tmpDir, "windows")
	if len(windowsCandidates) != 1 {
		t.Fatalf("expected 1 candidate on Windows, got %d: %v", len(windowsCandidates), windowsCandidates)
	}
	if windowsCandidates[0].name != "tool.exe" {
		t.Errorf("expected tool.exe, got %s", windowsCandidates[0].name)
	}

	// POSIX policy: files with execute bits are executable regardless of extension
	// tool.bat, tool.cmd, and tool all have 0755 mode, so all 3 are executable
	posixCandidates := discoverOS(tmpDir, "linux")
	if len(posixCandidates) != 3 {
		t.Fatalf("expected 3 candidates on POSIX, got %d: %v", len(posixCandidates), posixCandidates)
	}
	// Verify all three have execute bits
	for _, c := range posixCandidates {
		if c.name != "tool" && c.name != "tool.bat" && c.name != "tool.cmd" {
			t.Errorf("unexpected candidate %s", c.name)
		}
	}
}

// TestIsExecutable_PlatformPolicies exercises the Windows and POSIX predicates
// directly. It does NOT call Verify(); the production Verify() executable check
// is covered by TestVerify_UsesInstalledFilename and TestVerify_Executable.
func TestIsExecutable_PlatformPolicies(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a Windows .exe file with no POSIX execute bits but valid build info
	exeFile := filepath.Join(tmpDir, "tool.exe")
	if err := os.WriteFile(exeFile, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test Windows policy: .exe file with no POSIX execute bits should be executable
	if !isExecutableWindows("tool.exe") {
		t.Error("expected tool.exe to be executable on Windows")
	}

	// Test POSIX policy: file with no execute bits should not be executable
	if isExecutablePOSIX(0o644) {
		t.Error("expected file with 0644 to not be executable on POSIX")
	}

	// Test POSIX policy: file with execute bits should be executable
	if !isExecutablePOSIX(0o755) {
		t.Error("expected file with 0755 to be executable on POSIX")
	}
}

// discoverOS is a test helper that runs discovery with a specific OS policy.
// This allows testing Windows discovery semantics on non-Windows hosts.
func discoverOS(gobin, osName string) []candidate {
	entries, err := os.ReadDir(gobin)
	if err != nil {
		return nil
	}

	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		toolPath := filepath.Join(gobin, entry.Name())

		info, err := entry.Info()
		if err != nil {
			continue
		}

		var executable bool
		if osName == "windows" {
			executable = isExecutableWindows(entry.Name())
		} else {
			executable = isExecutablePOSIX(info.Mode())
		}

		if !executable {
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

	return candidates
}
