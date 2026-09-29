package main

import (
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/divijg19/Helm/v2/internal/testutil"
)

var (
	binaryPath       string
	compatBinaryPath string
	upperBinaryPath  string
	updateFlag       = flag.Bool("update", false, "update golden files")
)

func TestMain(m *testing.M) {
	flag.Parse()
	if *updateFlag {
		assertCleanTree()
	}
	tmp, err := os.MkdirTemp("", "helm-cli-test-*")
	if err != nil {
		panic(err)
	}

	binaryPath = filepath.Join(tmp, "helm")
	build := exec.Command("go", "build", "-ldflags=-X=github.com/divijg19/Helm/v2/internal/cli.version=v1.9.0-test -X=github.com/divijg19/Helm/v2/internal/cli.commitHash=abc1234 -X=github.com/divijg19/Helm/v2/internal/cli.buildDate=2026-08-16", "-o", binaryPath, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(tmp)
		panic("go build failed: " + err.Error())
	}

	// The canonical binary is the single implementation. Aliases are exercised
	// by copying that one executable to alias names so invocation-name
	// resolution (basename extraction) is what differs, not the binary.
	compatBinaryPath = filepath.Join(tmp, "update-go-tools")
	if err := copyFile(binaryPath, compatBinaryPath); err != nil {
		os.RemoveAll(tmp)
		panic("copy failed: " + err.Error())
	}
	upperBinaryPath = filepath.Join(tmp, "Helm")
	if err := copyFile(binaryPath, upperBinaryPath); err != nil {
		os.RemoveAll(tmp)
		panic("copy failed: " + err.Error())
	}

	// The temp dir holds the three built binaries and must be removed after
	// the run. It cannot be deferred: os.Exit below discards deferred calls,
	// which would leak a directory per test run.
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

func runCLI(t *testing.T, fixtureEnv []string, args ...string) cliResult {
	return runBinary(t, binaryPath, fixtureEnv, args...)
}

func runBinary(t *testing.T, binPath string, fixtureEnv []string, args ...string) cliResult {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), fixtureEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	code := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}

	return cliResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		code:   code,
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

var (
	goVersionRe = regexp.MustCompile(`go1\.\d+(\.\d+)?(-[A-Za-z0-9:\.]+)?`)
	// durationRe masks only the summary Duration line (label width 14, so
	// "Duration" plus six spaces). A broad N.Ns pattern would also mask
	// version-like or path substrings elsewhere and hide real drift. The
	// optional minutes group is required: formatDuration switches to "XmY.Zs"
	// at one minute, so a mask that only matched seconds would leave the
	// minute form unmasked and fail the golden on any slow run.
	durationRe = regexp.MustCompile(`(?m)^Duration\s+(\d+m)?\d+\.\d+s`)
)

func normalizeOutput(t *testing.T, gobinDir, s string) string {
	t.Helper()
	s = goVersionRe.ReplaceAllString(s, "goVERSION")
	s = durationRe.ReplaceAllString(s, "Duration      0.0s")
	parent := filepath.Dir(gobinDir)
	escaped := regexp.QuoteMeta(parent)
	s = regexp.MustCompile(escaped).ReplaceAllString(s, "<TMP>")
	return strings.TrimSpace(s)
}

func checkGolden(t *testing.T, got, gotErr, goldenPath, goldenErrPath string) {
	t.Helper()
	if *updateFlag {
		if goldenPath != "" {
			writeGolden(t, goldenPath, got)
		}
		if goldenErrPath != "" {
			writeGolden(t, goldenErrPath, gotErr)
		}
		return
	}
	if goldenPath != "" {
		golden := readGolden(t, goldenPath)
		if got != golden {
			t.Errorf("stdout mismatch:\ngot:\n%s\nwant:\n%s", got, golden)
		}
	}
	if goldenErrPath != "" {
		goldenErr := readGolden(t, goldenErrPath)
		if gotErr != goldenErr {
			t.Errorf("stderr mismatch:\ngot:\n%s\nwant:\n%s", gotErr, goldenErr)
		}
	}
}

func goldenPath(name string) string {
	return filepath.Join("..", "..", "testdata", "golden", name+".txt")
}

// assertCleanTree refuses golden rewrites on a dirty worktree, so `-update`
// can never launder a behavior regression into the expected files alongside
// unrelated local changes. If git is unavailable the rewrite still proceeds,
// because the safety net is best-effort rather than a hard dependency, but it
// says so instead of failing silently.
func assertCleanTree() {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: -update could not verify a clean worktree (%v); goldens will be rewritten anyway\n", err)
		return
	}
	if len(bytes.TrimSpace(out)) > 0 {
		panic("-update refused: worktree has uncommitted changes; commit or stash them before rewriting goldens")
	}
}

func jsonGoldenPath(name string) string {
	return filepath.Join("..", "..", "testdata", "json", name+".json")
}

func readGolden(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read golden %s: %v", path, err)
	}
	return strings.TrimSpace(string(data))
}

func writeGolden(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHelp(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--help")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("help"), "")
}

func TestVersion(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--version")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("version"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

func TestList(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--list")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("list"), "")
	if result.code != 1 {
		t.Errorf("exit code: expected 1 (issues found), got %d", result.code)
	}
}

func TestListCI(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--list", "--ci")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("list-ci"), "")
	if result.code != 1 {
		t.Errorf("exit code: expected 1 (issues found), got %d", result.code)
	}
}

func TestOutdated(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--outdated")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("outdated"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

func TestInfoTool(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--info", "hello")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("info"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

func TestDefaultUpdate(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env())
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("update"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
	// The hermetic module cache guarantees a real fetch on every run, so
	// the golden would silently lose the transparency feature if the
	// toolchain stopped emitting fetch events or Helm filtered them again.
	if !strings.Contains(result.stdout, "go: downloading example.com/world v1.3.0") {
		t.Errorf("expected live fetch subtree in update output:\n%s", result.stdout)
	}
}

// TestDefaultUpdateInstallsOnlyOutdated is the end-to-end proof of the
// outdated-first contract: after a default update, the outdated fixture tool
// (world v1.2.0 -> v1.3.0) is installed at the resolved version while the
// already-current tool (hello v1.0.0) keeps its exact binary version.
func TestDefaultUpdateInstallsOnlyOutdated(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env())
	if result.code != 0 {
		t.Fatalf("exit code: expected 0, got %d\nstdout:\n%s\nstderr:\n%s", result.code, result.stdout, result.stderr)
	}
	if got := installedVersion(t, f.Gobin("world")); got != "v1.3.0" {
		t.Errorf("world version = %q, want v1.3.0 (resolved latest must be installed)", got)
	}
	if got := installedVersion(t, f.Gobin("hello")); got != "v1.0.0" {
		t.Errorf("hello version = %q, want v1.0.0 (already-current tool must not be reinstalled)", got)
	}
}

// TestDefaultUpdateFiltered proves the filter-scope contract end to end:
// `helm world` must install only the named tool and must not even evaluate the
// others. Filter validation and candidate filtering were each covered in
// isolation; this is the composition a user actually runs, and the assertion
// that matters is scope, not versions: the JSON report must not mention any
// unselected tool, so an unselected tool that is merely up-to-date cannot hide
// a filter that stopped filtering.
func TestDefaultUpdateFiltered(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "world", "--json")
	if result.code != 0 {
		t.Fatalf("exit code: expected 0, got %d\nstdout:\n%s\nstderr:\n%s", result.code, result.stdout, result.stderr)
	}

	var report struct {
		Updated  []string `json:"updated"`
		UpToDate []string `json:"up_to_date"`
		Failed   []string `json:"failed"`
		Skipped  []string `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &report); err != nil {
		t.Fatalf("cannot parse update JSON: %v\n%s", err, result.stdout)
	}

	if !slices.Equal(report.Updated, []string{"world"}) {
		t.Errorf("updated = %v, want [world] only", report.Updated)
	}
	// The scope assertion: hello is current, so if the filter were ignored it
	// would be evaluated and reported as up-to-date. It must be absent.
	if len(report.UpToDate) != 0 {
		t.Errorf("up_to_date = %v, want empty: an unselected tool must not be evaluated", report.UpToDate)
	}
	if strings.Contains(result.stdout, "hello") {
		t.Errorf("report must not mention the unselected tool hello, got:\n%s", result.stdout)
	}
	if len(report.Failed) != 0 {
		t.Errorf("failed = %v, want empty", report.Failed)
	}
	// Skipped carries invalid binaries, which are always reported. It must not
	// carry localdev: that tool is ineligible, but it was not selected either,
	// so reporting it would mean the selection had been widened.
	if !slices.Contains(report.Skipped, f.Gobin("notgo")) {
		t.Errorf("skipped = %v, want it to contain the invalid binary path", report.Skipped)
	}
	if slices.Contains(report.Skipped, f.Gobin("localdev")) {
		t.Errorf("skipped = %v, want it to exclude the unselected local tool", report.Skipped)
	}

	// The install itself must have happened, at the resolved version.
	if got := installedVersion(t, f.Gobin("world")); got != "v1.3.0" {
		t.Errorf("world version = %q, want v1.3.0 (the selected tool must be updated)", got)
	}
	if got := installedVersion(t, f.Gobin("hello")); got != "v1.0.0" {
		t.Errorf("hello version = %q, want v1.0.0 (unselected tool must not be touched)", got)
	}
}

func installedVersion(t *testing.T, binPath string) string {
	t.Helper()
	bi, err := buildinfo.ReadFile(binPath)
	if err != nil {
		t.Fatalf("cannot read buildinfo for %s: %v", binPath, err)
	}
	return bi.Main.Version
}

func TestPlanCheck(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--check")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("check"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

func TestPlanDryRunAlias(t *testing.T) {
	f := testutil.NewFixture(t)
	check := runCLI(t, f.Env(), "--check")
	dry := runCLI(t, f.Env(), "--dry-run")
	if check.stdout != dry.stdout {
		t.Errorf("--dry-run must be an alias of --check:\n--check:\n%s\n--dry-run:\n%s", check.stdout, dry.stdout)
	}
	if check.code != dry.code {
		t.Errorf("exit code mismatch: --check=%d --dry-run=%d", check.code, dry.code)
	}
}

func TestPlanVerbose(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--check", "--verbose")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("check-verbose"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

func TestPlanVerboseShortFlag(t *testing.T) {
	f := testutil.NewFixture(t)
	long := runCLI(t, f.Env(), "--check", "--verbose")
	short := runCLI(t, f.Env(), "--check", "-V")
	gotLong := normalizeOutput(t, f.GobinDir, long.stdout)
	gotShort := normalizeOutput(t, f.GobinDir, short.stdout)
	if gotLong != gotShort {
		t.Errorf("-V must match --verbose:\n--verbose:\n%s\n-V:\n%s", gotLong, gotShort)
	}
}

func TestQuietUpdate(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "-q")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("quiet"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

func TestQuietLongFlag(t *testing.T) {
	// Each invocation gets an identical starting fixture: update mutates the
	// GOBIN, so sharing one fixture would make the second run observe the
	// first run's completed update rather than testing flag equivalence.
	fShort := testutil.NewFixture(t)
	fLong := testutil.NewFixture(t)
	short := runCLI(t, fShort.Env(), "-q")
	long := runCLI(t, fLong.Env(), "--quiet")
	gotShort := normalizeOutput(t, fShort.GobinDir, short.stdout)
	gotLong := normalizeOutput(t, fLong.GobinDir, long.stdout)
	if gotShort != gotLong {
		t.Errorf("--quiet must match -q output:\n-q:\n%s\n--quiet:\n%s", gotShort, gotLong)
	}
}

func TestQuietListSuppressesHeader(t *testing.T) {
	f := testutil.NewFixture(t)
	quiet := runCLI(t, f.Env(), "--list", "-q")
	normal := runCLI(t, f.Env(), "--list")
	if strings.Contains(quiet.stdout, "Discovery") || strings.Contains(quiet.stdout, "Go:") {
		t.Errorf("quiet --list must suppress the discovery header:\n%s", quiet.stdout)
	}
	if !strings.Contains(quiet.stdout, "NAME") || !strings.Contains(quiet.stdout, "Summary") {
		t.Errorf("quiet --list must still emit the table and summary:\n%s", quiet.stdout)
	}
	if !strings.Contains(normal.stdout, "Discovery") {
		t.Errorf("non-quiet --list must include the discovery header (sanity):\n%s", normal.stdout)
	}
}

func TestQuietOutdatedSuppressesHeader(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--outdated", "-q")
	if strings.Contains(result.stdout, "Discovery") || strings.Contains(result.stdout, "Go:") {
		t.Errorf("quiet --outdated must suppress the discovery header:\n%s", result.stdout)
	}
	if !strings.Contains(result.stdout, "NAME") || !strings.Contains(result.stdout, "Summary") {
		t.Errorf("quiet --outdated must still emit the table and summary:\n%s", result.stdout)
	}
}

func TestUpdateCI(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--ci")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", goldenPath("update-ci"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

func TestListJSON(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--list", "--json")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", jsonGoldenPath("list"), "")
	if result.code != 1 {
		t.Errorf("exit code: expected 1 (issues found), got %d", result.code)
	}
}

func TestPlanJSON(t *testing.T) {
	f := testutil.NewFixture(t)
	check := runCLI(t, f.Env(), "--check", "--json")
	dry := runCLI(t, f.Env(), "--dry-run", "--json")
	if check.stdout != dry.stdout {
		t.Errorf("--dry-run --json must match --check --json:\n--check:\n%s\n--dry-run:\n%s", check.stdout, dry.stdout)
	}
	got := normalizeOutput(t, f.GobinDir, check.stdout)
	checkGolden(t, got, "", jsonGoldenPath("plan"), "")
	if check.code != 0 {
		t.Errorf("exit code: expected 0, got %d", check.code)
	}
}

func TestOutdatedJSON(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--outdated", "--json")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", jsonGoldenPath("outdated"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

func TestInfoJSON(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--info", "hello", "--json")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", jsonGoldenPath("info"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
	if strings.Contains(got, `"operation"`) || strings.Contains(got, `"success"`) {
		t.Errorf("--info --json must stay a bare ToolReport without the operation envelope:\n%s", got)
	}
}

func TestUpdateJSON(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--json")
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	checkGolden(t, got, "", jsonGoldenPath("update"), "")
	if result.code != 0 {
		t.Errorf("exit code: expected 0, got %d", result.code)
	}
}

// TestDiscardedModifiersWarn documents that silently discarded flags now
// warn on stderr without changing the operation outcome: plan flags with an
// explicit operation, verbose with JSON or quiet output, and shadowed output
// modes. A clean invocation stays silent on stderr.
func TestDiscardedModifiersWarn(t *testing.T) {
	f := testutil.NewFixture(t)

	plan := runCLI(t, f.Env(), "--check", "--list")
	if !strings.Contains(plan.stderr, "Warning: --check/--dry-run has no effect with --list.") {
		t.Errorf("expected dropped-plan warning, got stderr:\n%s", plan.stderr)
	}
	if plan.code != 1 {
		t.Errorf("--check --list must still run the list operation (exit 1 on fixture issues), got %d", plan.code)
	}

	verbose := runCLI(t, f.Env(), "--check", "--json", "--verbose")
	if !strings.Contains(verbose.stderr, "Warning: --verbose has no effect with --json.") {
		t.Errorf("expected dropped-verbose warning, got stderr:\n%s", verbose.stderr)
	}
	if verbose.code != 0 {
		t.Errorf("--check --json --verbose must still plan cleanly, got exit %d", verbose.code)
	}

	quiet := runCLI(t, f.Env(), "--check", "--quiet", "--verbose")
	if !strings.Contains(quiet.stderr, "Warning: --verbose has no effect with --quiet.") {
		t.Errorf("expected dropped-verbose warning for --quiet, got stderr:\n%s", quiet.stderr)
	}
	if quiet.code != 0 {
		t.Errorf("--check --quiet --verbose must still plan cleanly, got exit %d", quiet.code)
	}

	quietShort := runCLI(t, f.Env(), "--check", "-q", "-V")
	if !strings.Contains(quietShort.stderr, "Warning: --verbose has no effect with --quiet.") {
		t.Errorf("expected the same warning for the -q -V aliases, got stderr:\n%s", quietShort.stderr)
	}

	modes := runCLI(t, f.Env(), "--json", "--ci")
	if !strings.Contains(modes.stderr, "Warning: --ci ignored; --json takes precedence.") {
		t.Errorf("expected mode-precedence warning, got stderr:\n%s", modes.stderr)
	}

	clean := runCLI(t, f.Env(), "--check")
	if strings.TrimSpace(clean.stderr) != "" {
		t.Errorf("clean invocation must stay silent on stderr, got:\n%s", clean.stderr)
	}
}

func TestUnknownOption(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--unknown")
	checkGolden(t, "", strings.TrimSpace(result.stderr), "", goldenPath("unknown-option-stderr"))
	if result.code != 2 {
		t.Errorf("exit code: expected 2, got %d", result.code)
	}
}

// offlineEnv returns the fixture environment with upstream module resolution
// disabled, so `go list -m` deterministically fails for every tool. Proxy and
// sum-database variables are removed rather than appended so no earlier entry
// can take precedence; GOBIN is preserved.
func offlineEnv(t *testing.T, fixtureEnv []string) []string {
	t.Helper()
	var env []string
	for _, kv := range append(append([]string{}, os.Environ()...), fixtureEnv...) {
		if strings.HasPrefix(kv, "GOPROXY=") || strings.HasPrefix(kv, "GOSUMDB=") ||
			strings.HasPrefix(kv, "GONOSUMDB=") || strings.HasPrefix(kv, "GONOSUMCHECK=") ||
			strings.HasPrefix(kv, "GOFLAGS=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GOPROXY=off")
}

// TestOutdatedResolutionFailureExitsNonZero pins the H2 contract end to end:
// when upstream resolution fails, tools are reported as failed (never
// up-to-date), the summary reconciles, and the operation exits non-zero.
func TestOutdatedResolutionFailureExitsNonZero(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runBinary(t, binaryPath, offlineEnv(t, f.Env()), "--outdated")
	if result.code == 0 {
		t.Errorf("exit code: expected non-zero for failed resolution, got 0\nstdout:\n%s", result.stdout)
	}
	got := normalizeOutput(t, f.GobinDir, result.stdout)
	for _, want := range []string{"Failed        2", "Up-to-date    0", "Outdated      0"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in outdated failure output:\n%s", want, got)
		}
	}
}

// TestUpdateResolutionFailureJSONExitsNonZero pins the H1 contract end to
// end: a failed update must exit non-zero under --json while stdout stays a
// complete, valid JSON document reporting success false and the failed tools.
func TestUpdateResolutionFailureJSONExitsNonZero(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runBinary(t, binaryPath, offlineEnv(t, f.Env()), "--json")
	if result.code == 0 {
		t.Errorf("exit code: expected non-zero for failed update, got 0\nstdout:\n%s", result.stdout)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
		t.Fatalf("stdout must stay valid JSON on failure, got error %v:\n%s", err, result.stdout)
	}
	if doc["success"] != false {
		t.Errorf("JSON success = %v, want false:\n%s", doc["success"], result.stdout)
	}
	failed, _ := doc["failed"].([]any)
	if len(failed) != 2 {
		t.Errorf("JSON failed = %v, want both tools listed:\n%s", doc["failed"], result.stdout)
	}
}

// TestForeignCompletionInvocationFailsSafely is the regression test for the
// Fish/Kubernetes-Helm completion collision: a foreign shell integration may
// invoke `helm completion fish`, whose words must never become an update
// filter. The invocation must fail with a stderr diagnostic, emit no normal
// report on stdout, perform no installs, and leave installed versions alone.
func TestForeignCompletionInvocationFailsSafely(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "completion", "fish")
	checkGolden(t, "", strings.TrimSpace(result.stderr), "", goldenPath("unknown-tool-stderr"))
	if result.code == 0 {
		t.Errorf("exit code: expected non-zero for unknown tool names, got 0\nstdout:\n%s", result.stdout)
	}
	if strings.TrimSpace(result.stdout) != "" {
		t.Errorf("stdout must carry no report for a foreign invocation, got:\n%s", result.stdout)
	}
	if !strings.Contains(result.stderr, "Unknown tool") {
		t.Errorf("stderr must name the unknown tools, got:\n%s", result.stderr)
	}
	if got := installedVersion(t, f.Gobin("world")); got != "v1.2.0" {
		t.Errorf("world version = %q, want v1.2.0 (no install must run for unknown names)", got)
	}
	if got := installedVersion(t, f.Gobin("hello")); got != "v1.0.0" {
		t.Errorf("hello version = %q, want v1.0.0 (no install must run for unknown names)", got)
	}
}

// TestUnknownToolFilterFailsFast proves the general contract behind the
// collision fix: any unmatched filter name is rejected, whether alone or
// mixed with valid names, while valid filters keep working.
func TestUnknownToolFilterFailsFast(t *testing.T) {
	f := testutil.NewFixture(t)
	for _, args := range [][]string{{"nosuchtool"}, {"hello", "nosuchtool"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			result := runCLI(t, f.Env(), args...)
			if result.code == 0 {
				t.Errorf("helm %v: expected non-zero exit, got 0", args)
			}
			if strings.TrimSpace(result.stdout) != "" {
				t.Errorf("helm %v: expected empty stdout, got:\n%s", args, result.stdout)
			}
		})
	}
}

func TestInfoMissing(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--info", "nonexistent")
	checkGolden(t, "", strings.TrimSpace(result.stderr), "", goldenPath("info-missing-stderr"))
	if result.code != 1 {
		t.Errorf("exit code: expected 1 (operational lookup failure), got %d", result.code)
	}
	if strings.TrimSpace(result.stdout) != "" {
		t.Errorf("lookup failure must render no report on stdout, got:\n%s", result.stdout)
	}
}

func TestInfoNoName(t *testing.T) {
	f := testutil.NewFixture(t)
	result := runCLI(t, f.Env(), "--info")
	checkGolden(t, "", strings.TrimSpace(result.stderr), "", goldenPath("info-noname-stderr"))
	if result.code != 2 {
		t.Errorf("exit code: expected 2 (usage error), got %d", result.code)
	}
	if strings.TrimSpace(result.stdout) != "" {
		t.Errorf("usage error must render no report on stdout, got:\n%s", result.stdout)
	}
}

// TestExplicitOpsRejectPositionals pins the v1.9.5 filter-scope contract:
// --list and --outdated accept no tool names, and --info accepts exactly
// one. Extra positionals are usage errors with no report on stdout, so a
// filtered update command can never be pasted onto an explicit operation
// and silently widen scope.
func TestExplicitOpsRejectPositionals(t *testing.T) {
	f := testutil.NewFixture(t)
	cases := [][]string{
		{"--list", "hello"},
		{"--list", "nosuchtool"},
		{"--outdated", "hello"},
		{"--outdated", "nosuchtool"},
		{"--info", "hello", "extra"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			result := runCLI(t, f.Env(), args...)
			if result.code != 2 {
				t.Errorf("helm %v: expected exit 2, got %d", args, result.code)
			}
			if strings.TrimSpace(result.stdout) != "" {
				t.Errorf("helm %v: expected empty stdout, got:\n%s", args, result.stdout)
			}
			if strings.TrimSpace(result.stderr) == "" {
				t.Errorf("helm %v: expected stderr diagnostic, got none", args)
			}
		})
	}
}

func TestAliasUpdateGoToolsMatchesHelm(t *testing.T) {
	f := testutil.NewFixture(t)
	helm := runCLI(t, f.Env(), "--version")
	compat := runBinary(t, compatBinaryPath, f.Env(), "--version")
	if helm.code != compat.code {
		t.Errorf("exit code mismatch: helm=%d update-go-tools=%d", helm.code, compat.code)
	}
	if helm.stdout != compat.stdout {
		t.Errorf("update-go-tools alias must produce identical --version output to helm:\nhelm:\n%s\nupdate-go-tools:\n%s", helm.stdout, compat.stdout)
	}
}

func TestAliasUpperCaseHelmMatchesHelm(t *testing.T) {
	f := testutil.NewFixture(t)
	helm := runCLI(t, f.Env(), "--version")
	upper := runBinary(t, upperBinaryPath, f.Env(), "--version")
	if helm.code != upper.code {
		t.Errorf("exit code mismatch: helm=%d Helm=%d", helm.code, upper.code)
	}
	if helm.stdout != upper.stdout {
		t.Errorf("Helm alias must produce identical --version output to helm:\nhelm:\n%s\nHelm:\n%s", helm.stdout, upper.stdout)
	}
}

// TestAliasUpdateExecutesSameProduct pins the secondary-alias contract: the
// update-go-tools invocation executes the same product as helm, producing
// identical update output and exit behavior on identical starting fixtures.
func TestAliasUpdateExecutesSameProduct(t *testing.T) {
	fHelm := testutil.NewFixture(t)
	fAlias := testutil.NewFixture(t)
	helmRes := runCLI(t, fHelm.Env())
	aliasRes := runBinary(t, compatBinaryPath, fAlias.Env())
	if helmRes.code != aliasRes.code {
		t.Errorf("exit code mismatch: helm=%d update-go-tools=%d", helmRes.code, aliasRes.code)
	}
	gotHelm := normalizeOutput(t, fHelm.GobinDir, helmRes.stdout)
	gotAlias := normalizeOutput(t, fAlias.GobinDir, aliasRes.stdout)
	if gotHelm != gotAlias {
		t.Errorf("update-go-tools must produce identical update output to helm:\nhelm:\n%s\nupdate-go-tools:\n%s", gotHelm, gotAlias)
	}
}

// TestLoadFailureIsACleanError pins the tool-loading failure path end to end:
// a GOBIN that cannot be read must exit 1 with a diagnostic on stderr and no
// report on stdout. Nothing asserted this, so a regression here could surface
// as a raw panic or a stack trace instead of a clean error.
func TestLoadFailureIsACleanError(t *testing.T) {
	f := testutil.NewFixture(t)
	missing := filepath.Join(t.TempDir(), "no-such-gobin")
	env := append(f.Env(), "GOBIN="+missing)

	for _, op := range [][]string{{"--list"}, {"--outdated"}, {"--check"}, {"--json"}} {
		t.Run(strings.Join(op, " "), func(t *testing.T) {
			result := runCLI(t, env, op...)
			if result.code != 1 {
				t.Errorf("exit code = %d, want 1 (operational failure), got stderr:\n%s", result.code, result.stderr)
			}
			if !strings.Contains(result.stderr, "Error loading tools:") {
				t.Errorf("expected the loading diagnostic on stderr, got:\n%s", result.stderr)
			}
			if strings.TrimSpace(result.stdout) != "" {
				t.Errorf("expected no report on stdout, got:\n%s", result.stdout)
			}
			if strings.Contains(result.stderr, "goroutine ") || strings.Contains(result.stderr, "panic:") {
				t.Errorf("a load failure must not panic, got:\n%s", result.stderr)
			}
		})
	}
}
