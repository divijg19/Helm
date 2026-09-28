package app

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/divijg19/Helm/internal/testutil"
	"github.com/divijg19/Helm/internal/tool"
)

// TestNewAppPropagatesGobinResolutionError proves the first link of the
// environment-failure propagation chain with the real NewApp code:
//
//	GetGobin() failure
//	    ↓
//	NewApp() failure
//
// The failing resolver simulates the exact error tool.GetGobin returns when
// GOBIN and GOPATH are unset, so this exercises the genuine production path
// rather than mocking NewApp itself.
func TestNewAppPropagatesGobinResolutionError(t *testing.T) {
	prev := gobinResolver
	gobinResolver = func() (string, error) {
		return "", fmt.Errorf("GOPATH is not set and GOBIN is empty: %w", tool.ErrGobinResolution)
	}
	defer func() { gobinResolver = prev }()

	result, err := NewApp(NewRenderer(ModeTerminal, false), tool.DefaultRunner{})
	if err == nil {
		t.Fatalf("expected NewApp to fail when GOBIN resolution fails, got nil (app=%v)", result)
	}
	if !errors.Is(err, tool.ErrGobinResolution) {
		t.Fatalf("expected ErrGobinResolution, got %v", err)
	}
}

// TestInventoryReport_ConservesTools pins the inventory conservation
// invariant: every discovered tool lands in exactly one of Healthy, Local,
// or Unhealthy, so the summary counts always reconcile with the tool list.
// Invalid binaries are a separate bucket and must not be folded into
// Unhealthy, which would break that invariant and double-count the issue
// total every renderer reports.
func TestInventoryReport_ConservesTools(t *testing.T) {
	dir := t.TempDir()
	writeExec := func(name string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("dummy"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	updatable := &buildinfo.BuildInfo{
		Path: "example.com/healthy/cmd/healthy",
		Main: debug.Module{Path: "example.com/healthy", Version: "v1.0.0"},
	}
	local := &buildinfo.BuildInfo{
		Path: "example.com/local",
		Main: debug.Module{Path: "example.com/local", Version: "(devel)"},
	}
	tools := []tool.Tool{
		tool.NewTool("healthy", writeExec("healthy"), updatable),
		tool.NewTool("local", writeExec("local"), local),
		tool.NewTool("missing", filepath.Join(dir, "missing"), updatable),
	}
	invalid := []tool.InvalidBinary{
		{Path: filepath.Join(dir, "notgo"), Error: tool.ErrMissingBuildInfo},
		{Path: filepath.Join(dir, "nopath"), Error: tool.ErrMissingPackagePath},
	}

	report := (&App{}).inventoryReport(tool.LoadResult{Tools: tools, Invalid: invalid})

	if report.Summary.Healthy != 1 || report.Summary.Local != 1 || report.Summary.Unhealthy != 1 {
		t.Errorf("summary = %+v, want {Healthy:1 Local:1 Unhealthy:1}", report.Summary)
	}
	if report.Summary.Invalid != len(invalid) {
		t.Errorf("summary.Invalid = %d, want %d", report.Summary.Invalid, len(invalid))
	}
	if total := report.Summary.Healthy + report.Summary.Local + report.Summary.Unhealthy; total != len(tools) {
		t.Errorf("summary total = %d, want %d (one bucket per tool, invalid binaries excluded)", total, len(tools))
	}
	if len(report.Invalid) != len(invalid) {
		t.Errorf("report.Invalid = %d entries, want %d", len(report.Invalid), len(invalid))
	}
	if report.Success {
		t.Error("report.Success = true, want false when unhealthy tools and invalid binaries are present")
	}
	// The renderers report Unhealthy+Invalid as the issue total, so this is
	// the number a user sees. Invalid binaries must be counted exactly once.
	if got, want := report.Summary.Unhealthy+report.Summary.Invalid, 1+len(invalid); got != want {
		t.Errorf("issue total = %d, want %d", got, want)
	}
	got := map[string]string{}
	for _, item := range report.Tools {
		got[item.Name] = item.Status
	}
	want := map[string]string{"healthy": "Healthy", "local": "Local", "missing": "Unhealthy"}
	for name, status := range want {
		if got[name] != status {
			t.Errorf("tool %s status = %q, want %q", name, got[name], status)
		}
	}
}

// TestUpdateReport_OnlySuccessfulCandidatesInDetail is the regression test for
// the v1.9.2 updateReport fix: only candidates whose install succeeded may
// appear in UpdatedDetail (and therefore the Installations tree). A failed
// install must stay confined to the Failed list, and a candidate without any
// matching result must not be silently classified as installed.
func TestUpdateReport_OnlySuccessfulCandidatesInDetail(t *testing.T) {
	okTool := makeTool("good", "example.com/good", "v1.0.0")
	failTool := makeTool("bad", "example.com/bad", "v1.0.0")
	ghostTool := makeTool("ghost", "example.com/ghost", "v1.0.0")

	set := tool.CandidateSet{
		Candidates: []tool.UpdateCandidate{
			{Tool: okTool, Version: "v1.2.0"},
			{Tool: failTool, Version: "v1.3.0"},
			{Tool: ghostTool, Version: "v1.4.0"},
		},
		UpToDate: []tool.Tool{makeTool("meh", "example.com/meh", "v1.0.0")},
	}

	results := []tool.ToolUpdateResult{
		{Tool: okTool, Success: true},
		{Tool: failTool, Success: false, Error: errors.New("simulated install failure")},
	}

	report := (&App{}).updateReport(results, tool.LoadResult{}, set, 0, nil)

	if got := strings.Join(report.Updated, ","); got != "good" {
		t.Errorf("Updated = %q, want [good]", got)
	}
	if got := strings.Join(report.Failed, ","); got != "bad" {
		t.Errorf("Failed = %q, want [bad]", got)
	}
	if len(report.UpdatedDetail) != 1 {
		t.Fatalf("UpdatedDetail = %+v, want exactly the one successful installation", report.UpdatedDetail)
	}
	d := report.UpdatedDetail[0]
	if d.Name != "good" {
		t.Errorf("UpdatedDetail[0].Name = %q, want good", d.Name)
	}
	if d.Previous != "v1.0.0" || d.Resolved != "v1.2.0" {
		t.Errorf("UpdatedDetail[0] version transition = %q -> %q, want v1.0.0 -> v1.2.0", d.Previous, d.Resolved)
	}
	if d.PackagePath != "example.com/good/cmd/good" || d.ModulePath != "example.com/good" {
		t.Errorf("UpdatedDetail[0] paths = %q / %q", d.PackagePath, d.ModulePath)
	}
}

// TestUpdateReport_EmptyResultsLeaveDetailEmpty covers the empty and partial
// result-set edge cases: with no install results, no candidate may appear as
// installed, and no tool may be misclassified as failed.
func TestUpdateReport_EmptyResultsLeaveDetailEmpty(t *testing.T) {
	set := tool.CandidateSet{
		Candidates: []tool.UpdateCandidate{
			{Tool: makeTool("good", "example.com/good", "v1.0.0"), Version: "v1.2.0"},
		},
	}

	report := (&App{}).updateReport(nil, tool.LoadResult{}, set, 0, nil)

	if len(report.UpdatedDetail) != 0 {
		t.Errorf("UpdatedDetail = %+v, want empty when no results exist", report.UpdatedDetail)
	}
	if len(report.Updated) != 0 || len(report.Failed) != 0 {
		t.Errorf("expected empty Updated/Failed, got %v / %v", report.Updated, report.Failed)
	}
}

// TestUpdateReport_ResolutionFailuresReported proves resolution-vetoed tools
// remain in the failure path with an Outdated diagnostic, independent of the
// install-result scan.
func TestUpdateReport_ResolutionFailuresReported(t *testing.T) {
	resTool := makeTool("res", "example.com/res", "v1.0.0")
	set := tool.CandidateSet{
		Failed: []tool.OutdatedResult{
			{Tool: resTool, Error: errors.New("resolution vetoed install")},
		},
	}

	report := (&App{}).updateReport(nil, tool.LoadResult{}, set, 0, nil)

	if got := strings.Join(report.Failed, ","); got != "res" {
		t.Errorf("Failed = %q, want [res]", got)
	}
	if len(report.Diagnostics) != 1 || report.Diagnostics[0].Category != "Outdated" {
		t.Errorf("expected one Outdated diagnostic, got %+v", report.Diagnostics)
	}
	if report.Success {
		t.Error("report must be unsuccessful while any tool failed")
	}
}

// TestUpdateReport_PreservesDuration pins the duration plumbing: the install
// phase wall time measured by UpdateCandidates must reach the report
// unchanged. Duration semantics (total install-phase wall time, resolution
// excluded) are defined on UpdateCandidates; this test guards the handoff.
func TestUpdateReport_PreservesDuration(t *testing.T) {
	report := (&App{}).updateReport(nil, tool.LoadResult{}, tool.CandidateSet{}, 42, nil)
	if report.Duration != 42 {
		t.Errorf("report Duration = %v, want 42 (measured install-phase wall time)", report.Duration)
	}
}

// TestOutdatedReport_ResolutionFailuresAreNotUpToDate pins the H2 invariant:
// a tool whose update state could not be determined is neither outdated nor
// up-to-date. Failures occupy their own counter, flip operation success, and
// never inflate the up-to-date count.
func TestOutdatedReport_ResolutionFailuresAreNotUpToDate(t *testing.T) {
	current := makeTool("hello", "example.com/hello", "v1.0.0")
	outdated := makeTool("world", "example.com/world", "v1.2.0")
	broken := makeTool("broken", "example.com/broken", "v1.0.0")

	results := []tool.OutdatedResult{
		{Tool: current, Current: "v1.0.0", Latest: "v1.0.0", Outdated: false},
		{Tool: outdated, Current: "v1.2.0", Latest: "v1.3.0", Outdated: true},
		{Tool: broken, Current: "v1.0.0", Error: errors.New("simulated resolution failure")},
	}

	report := (&App{}).outdatedReport(results)

	if report.Summary.Outdated != 1 || report.Summary.UpToDate != 1 || report.Summary.Failed != 1 {
		t.Errorf("summary = %+v, want {Outdated:1 UpToDate:1 Failed:1}", report.Summary)
	}
	// Total is what both human renderers print as "Checked", so it must equal
	// the number of results and reconcile with the three buckets.
	if report.Summary.Total != len(report.Results) {
		t.Errorf("Summary.Total = %d, want len(Results) = %d", report.Summary.Total, len(report.Results))
	}
	if sum := report.Summary.Outdated + report.Summary.UpToDate + report.Summary.Failed; sum != report.Summary.Total {
		t.Errorf("Outdated+UpToDate+Failed = %d, want Total = %d", sum, report.Summary.Total)
	}
	if report.Success {
		t.Error("report must be unsuccessful while any resolution failed")
	}
	if got := report.Results[2].Error; got == "" {
		t.Error("failed result must carry its error message")
	}

	clean := (&App{}).outdatedReport(results[:2])
	if clean.Summary.Failed != 0 || !clean.Success {
		t.Errorf("clean report = %+v, want Failed:0 and success", clean.Summary)
	}
	empty := (&App{}).outdatedReport(nil)
	if empty.Summary != (OutdatedSummary{}) || !empty.Success {
		t.Errorf("empty report = %+v, want zero summary and success", empty)
	}
}

// TestUpdateReport_DuplicateResultsKeepFirstMatch pins the legacy first-match
// behavior for duplicated tool names: candidate matching must use the first
// result with a given name, exactly as the pre-v1.9.2 linear scan did.
// Production cannot produce duplicates (names stay unique 1:1 from discovery
// through resolution and installation), so this guards the function contract
// for direct callers rather than a reachable runtime path.
func TestUpdateReport_DuplicateResultsKeepFirstMatch(t *testing.T) {
	dupTool := makeTool("dup", "example.com/dup", "v1.0.0")
	set := tool.CandidateSet{
		Candidates: []tool.UpdateCandidate{
			{Tool: dupTool, Version: "v1.2.0"},
		},
	}
	results := []tool.ToolUpdateResult{
		{Tool: dupTool, Success: true},
		{Tool: dupTool, Success: false, Error: errors.New("stale duplicate result")},
	}

	report := (&App{}).updateReport(results, tool.LoadResult{}, set, 0, nil)

	if len(report.UpdatedDetail) != 1 {
		t.Fatalf("UpdatedDetail = %+v, want the first (successful) match for a duplicated tool name", report.UpdatedDetail)
	}
	if got := report.UpdatedDetail[0].Previous; got != "v1.0.0" {
		t.Errorf("UpdatedDetail[0].Previous = %q, want v1.0.0 from the first matching result", got)
	}
}

// TestUpdateReport_FailedDetailFromRealInstallFailure proves the report
// builder carries an install failure's cause through from the tool layer,
// which is the only place the reason exists for non-terminal renderers.
func TestUpdateReport_FailedDetailFromRealInstallFailure(t *testing.T) {
	failing := makeTool("world", "example.com/world", "v1.2.0")
	set := tool.CandidateSet{
		Candidates: []tool.UpdateCandidate{{Tool: failing, Version: "v1.3.0"}},
	}
	results := []tool.ToolUpdateResult{
		{Tool: failing, Success: false, Error: errors.New("go: module lookup disabled")},
	}

	report := (&App{}).updateReport(results, tool.LoadResult{}, set, 0, nil)

	if len(report.Failed) != 1 || report.Failed[0] != "world" {
		t.Fatalf("Failed = %v, want [world]", report.Failed)
	}
	if len(report.FailedDetail) != 1 {
		t.Fatalf("FailedDetail = %+v, want exactly 1 entry", report.FailedDetail)
	}
	if report.FailedDetail[0].Name != "world" || report.FailedDetail[0].Error != "go: module lookup disabled" {
		t.Errorf("FailedDetail[0] = %+v, want the tool name and install error", report.FailedDetail[0])
	}
	if report.Success {
		t.Error("a failed install must make the report unsuccessful")
	}
	if err := report.Err(); err == nil || err.Error() != "1 update failed" {
		t.Errorf("Err() = %v, want 1 update failed", err)
	}

	// A result with no error still appears in Failed without a fabricated cause.
	bare := (&App{}).updateReport([]tool.ToolUpdateResult{{Tool: failing}}, tool.LoadResult{}, set, 0, nil)
	if len(bare.Failed) != 1 || len(bare.FailedDetail) != 0 {
		t.Errorf("a failure without an error must list the name only, got %+v / %+v", bare.Failed, bare.FailedDetail)
	}
}

// TestAppLoadIsMemoized proves a second load() returns the identical result
// without re-reading the directory. App.load backs every operation, so a
// regression that dropped the cache would re-run `go env` and a full GOBIN
// read per call, doubling startup work with no visible symptom. It also
// proves a failed load is never memoized, which would otherwise make a later
// retry silently succeed with an empty result set.
func TestAppLoadIsMemoized(t *testing.T) {
	f := testutil.NewFixture(t)
	a := &App{Gobin: f.GobinDir}

	first, err := a.load()
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}
	if len(first.Tools) == 0 {
		t.Fatal("fixture must yield at least one loadable tool")
	}
	if !a.loadResValid {
		t.Error("a successful load must be memoized")
	}

	second, err := a.load()
	if err != nil {
		t.Fatalf("second load failed: %v", err)
	}
	if len(second.Tools) != len(first.Tools) {
		t.Errorf("memoized load returned %d tools, want the first load's %d", len(second.Tools), len(first.Tools))
	}
	for i := range first.Tools {
		if second.Tools[i].Name() != first.Tools[i].Name() {
			t.Errorf("tool %d = %q on reload, want %q (order and identity must be stable)", i, second.Tools[i].Name(), first.Tools[i].Name())
		}
	}

	missing := &App{Gobin: filepath.Join(f.GobinDir, "nope")}
	if _, err := missing.load(); err == nil {
		t.Error("expected an error for a non-existent GOBIN")
	}
	if missing.loadResValid {
		t.Error("a failed load must not be memoized")
	}
}
