package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"helm/internal/tool"
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
