package tool

import (
	"debug/buildinfo"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
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

// TestVerify_PassesInstalledBasenameToPredicate proves the v1.9.2 wiring fix
// through the production Verify() path on any host: it observes which filename
// Verify hands to the executable predicate and requires the installed
// basename, not the logical tool name. Unlike TestVerify_UsesInstalledFilename
// (which only discriminates on Windows, where POSIX-blind names fail the .exe
// policy), this test fails on every platform if Verify regresses to t.Name().
func TestVerify_PassesInstalledBasenameToPredicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool.exe")
	if err := os.WriteFile(path, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}

	prev := executablePolicy
	var gotName string
	executablePolicy = func(name string, mode os.FileMode) bool {
		gotName = name
		return true
	}
	defer func() { executablePolicy = prev }()

	results := Verify([]Tool{
		{name: "tool", path: path, info: validBuildInfo()},
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if gotName != "tool.exe" {
		t.Errorf("predicate received filename %q, want installed basename %q", gotName, "tool.exe")
	}
	if !results[0].Healthy {
		t.Errorf("expected healthy when the predicate accepts the file, got: %s", results[0].Error)
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

// TestIsExecutable_DiscoverySemantics exercises the real discovery loop with
// an explicit platform policy, so Windows discovery semantics are covered on
// any host without duplicating the production loop in a test helper.
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

	windowsPolicy := func(name string, _ os.FileMode) bool {
		return isExecutableWindows(name)
	}
	posixPolicy := func(_ string, mode os.FileMode) bool {
		return isExecutablePOSIX(mode)
	}
	candidateNames := func(t *testing.T, cands []candidate) []string {
		t.Helper()
		names := make([]string, 0, len(cands))
		for _, c := range cands {
			names = append(names, c.name)
		}
		return names
	}

	// Windows policy: only .exe files are candidates; directories are skipped.
	windowsCandidates, err := discoverWithPolicy(tmpDir, windowsPolicy)
	if err != nil {
		t.Fatalf("discoverWithPolicy failed: %v", err)
	}
	if got, want := candidateNames(t, windowsCandidates), []string{"tool.exe"}; !slices.Equal(got, want) {
		t.Errorf("windows candidates = %v, want %v", got, want)
	}

	// POSIX policy: files with execute bits are candidates regardless of
	// extension, in sorted order; directories are skipped.
	posixCandidates, err := discoverWithPolicy(tmpDir, posixPolicy)
	if err != nil {
		t.Fatalf("discoverWithPolicy failed: %v", err)
	}
	if got, want := candidateNames(t, posixCandidates), []string{"tool", "tool.bat", "tool.cmd"}; !slices.Equal(got, want) {
		t.Errorf("posix candidates = %v, want %v", got, want)
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
