package app

import (
	"bytes"
	"debug/buildinfo"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"

	"helm/internal/tool"
)

func captureOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	runErr := fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	r.Close()
	return buf.String(), runErr
}

func makeTool(name, pkg, version string) tool.Tool {
	return tool.NewTool(name, "/gobin/"+name, &buildinfo.BuildInfo{
		Path: pkg + "/cmd/" + name,
		Main: debug.Module{Path: pkg, Version: version},
	})
}

func TestTerminalUpdate_ClassesStatusCorrectly(t *testing.T) {
	r := TerminalRenderer{}
	report := UpdateReport{
		Updated: []string{"good"},
		Failed:  []string{"failed"},
		Skipped: []string{"localdev"},
	}

	out, err := captureOutput(t, func() error {
		return r.Update(report)
	})
	if err == nil {
		t.Error("expected error because one update failed")
	}
	if !bytes.Contains([]byte(out), []byte("Updated")) {
		t.Errorf("expected Updated, got:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("Failed")) {
		t.Errorf("expected Failed, got:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("Skipped")) {
		t.Errorf("expected Skipped, got:\n%s", out)
	}
}

func TestJSONRenderer_SkippedNotInFailed(t *testing.T) {
	r := JSONRenderer{}
	report := UpdateReport{
		Updated: []string{"hello"},
		Skipped: []string{"localdev"},
		Failed:  make([]string, 0),
	}

	out, err := captureOutput(t, func() error {
		return r.Update(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Contains([]byte(out), []byte(`"failed": []`)) {
		t.Errorf("local/devel tool must not appear in failed list (regression):\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte(`"skipped": [`)) {
		t.Errorf("expected skipped list populated:\n%s", out)
	}
}

// TestJSONRenderer_UpdateFailureExitsNonZero pins the H1 invariant: a failed
// update must produce a non-zero process result from every renderer. The JSON
// document on stdout stays complete and valid; only the returned error (which
// the CLI maps to a non-zero exit) signals failure.
func TestJSONRenderer_UpdateFailureExitsNonZero(t *testing.T) {
	r := JSONRenderer{}
	report := UpdateReport{
		OperationEnvelope: OperationEnvelope{Operation: OperationUpdate, Success: false},
		Updated:           []string{"hello"},
		Failed:            []string{"world"},
	}

	out, err := captureOutput(t, func() error {
		return r.Update(report)
	})
	if err == nil {
		t.Fatalf("expected error for failed update, got nil with output:\n%s", out)
	}
	for _, want := range []string{`"operation": "update"`, `"success": false`, `"failed": [`, `"world"`, `"updated": [`} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("expected %s in failed-update JSON:\n%s", want, out)
		}
	}
}

func TestJSONRenderer_InventoryReportsIssues(t *testing.T) {
	r := JSONRenderer{}
	report := InventoryReport{
		Tools: []ToolInventoryItem{
			{Name: "hello", Version: "v1.0.0", PackagePath: "example.com/hello", Status: "Healthy"},
		},
		Invalid: []InvalidReport{
			{Path: "/gobin/notgo", Message: "missing or unreadable build info"},
		},
		Summary: InventorySummary{Healthy: 1, Invalid: 1, Unhealthy: 1},
	}
	_, err := captureOutput(t, func() error {
		return r.Inventory(report)
	})
	if err == nil {
		t.Error("expected error when inventory has issues")
	}
}

func TestJSONRenderer_PlanSchemaStable(t *testing.T) {
	r := JSONRenderer{}
	report := PlanReport{
		OperationEnvelope: OperationEnvelope{Operation: OperationCheck, Success: true},
		WouldUpdate: []PlanItem{
			{Name: "hello", PackagePath: "example.com/hello", InstallTarget: "example.com/hello", Command: "go install example.com/hello@latest"},
		},
		Skipped: []PlanItem{{Name: "localdev"}},
	}
	out, err := captureOutput(t, func() error {
		return r.Plan(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{`"operation": "check"`, `"success": true`, `"would_update"`, `"skipped"`, `"command"`, `"install_target"`} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("expected %s in plan JSON:\n%s", want, out)
		}
	}
}

func TestJSONEnvelope_OperationAndSuccess(t *testing.T) {
	r := JSONRenderer{}

	plan, _ := captureOutput(t, func() error {
		return r.Plan(PlanReport{OperationEnvelope: OperationEnvelope{Operation: OperationCheck, Success: true}})
	})
	if !bytes.Contains([]byte(plan), []byte(`"operation": "check"`)) || !bytes.Contains([]byte(plan), []byte(`"success": true`)) {
		t.Errorf("plan envelope mismatch:\n%s", plan)
	}

	update, _ := captureOutput(t, func() error {
		return r.Update(UpdateReport{
			OperationEnvelope: OperationEnvelope{Operation: OperationUpdate, Success: false},
			Updated:           []string{"hello"},
			Skipped:           []string{},
			Failed:            []string{"world"},
		})
	})
	if !bytes.Contains([]byte(update), []byte(`"operation": "update"`)) || !bytes.Contains([]byte(update), []byte(`"success": false`)) {
		t.Errorf("update envelope mismatch:\n%s", update)
	}
}

func TestJSONRenderer_InfoNotFound(t *testing.T) {
	r := JSONRenderer{}
	loadRes := tool.LoadResult{
		Tools: []tool.Tool{makeTool("hello", "example.com/hello", "v1.0.0")},
	}
	err := r.Info(loadRes, "missing")
	if err == nil {
		t.Error("expected error for missing tool")
	}
}

func TestJSONRenderer_InfoFound(t *testing.T) {
	r := JSONRenderer{}
	loadRes := tool.LoadResult{
		Tools: []tool.Tool{makeTool("hello", "example.com/hello", "v1.0.0")},
	}
	out, err := captureOutput(t, func() error {
		return r.Info(loadRes, "hello")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte(`"name": "hello"`)) {
		t.Errorf("expected tool in JSON output:\n%s", out)
	}
}

func TestTerminalRenderer_InventorySummary(t *testing.T) {
	r := TerminalRenderer{}
	report := InventoryReport{
		Tools: []ToolInventoryItem{
			{Name: "hello", Version: "v1.0.0", PackagePath: "example.com/hello", Status: "Healthy"},
			{Name: "localdev", Version: "(devel)", PackagePath: "example.com/localdev", Status: "Local"},
		},
		Invalid: []InvalidReport{
			{Path: "/gobin/notgo", Message: "missing or unreadable build info"},
		},
		Summary: InventorySummary{Healthy: 2, Local: 1, Invalid: 1, Unhealthy: 1},
	}
	out, err := captureOutput(t, func() error {
		return r.Inventory(report)
	})
	if err == nil {
		t.Error("expected error for unhealthy inventory")
	}
	if !bytes.Contains([]byte(out), []byte("NAME")) {
		t.Errorf("expected table header in output:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("Healthy")) {
		t.Errorf("expected Healthy in output:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("Local         1")) {
		t.Errorf("expected Local count in output:\n%s", out)
	}
}

func TestTerminalRenderer_PlanConciseAndVerbose(t *testing.T) {
	report := PlanReport{
		WouldUpdate: []PlanItem{
			{Name: "hello", PackagePath: "example.com/hello", InstallTarget: "example.com/hello", Command: "go install example.com/hello@latest"},
		},
		Skipped: []PlanItem{{Name: "localdev"}},
	}

	concise, err := captureOutput(t, func() error {
		return TerminalRenderer{}.Plan(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(concise, "go install") {
		t.Errorf("concise plan must not show commands:\n%s", concise)
	}

	verbose, err := captureOutput(t, func() error {
		return TerminalRenderer{verbose: true}.Plan(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(verbose, "go install example.com/hello@latest") {
		t.Errorf("verbose plan must show install commands:\n%s", verbose)
	}
}

func TestQuietRenderer_UpdateSummaryOnly(t *testing.T) {
	r := QuietRenderer{}
	report := UpdateReport{
		Updated: []string{"hello"},
		Failed:  make([]string, 0),
		Skipped: []string{"localdev"},
	}
	out, err := captureOutput(t, func() error {
		return r.Update(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Updated", "Skipped", "Failed", "Duration"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("expected %s in quiet summary:\n%s", want, out)
		}
	}
	if bytes.Contains([]byte(out), []byte("hello")) {
		t.Errorf("quiet mode must not print per-tool names:\n%s", out)
	}
}

func TestQuietRenderer_HeaderNoop(t *testing.T) {
	out, err := captureOutput(t, func() error {
		return QuietRenderer{}.Header(HeaderInfo{Gobin: "/gobin"})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("quiet header must emit nothing, got:\n%s", out)
	}
}

func TestCIRenderer_ASCIIOnly(t *testing.T) {
	r := CIRenderer{}
	report := InventoryReport{
		Tools: []ToolInventoryItem{
			{Name: "hello", Version: "v1.0.0", PackagePath: "example.com/hello", Status: "Healthy"},
		},
		Summary: InventorySummary{Healthy: 1},
	}
	out, err := captureOutput(t, func() error {
		return r.Inventory(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "✓") || strings.Contains(out, "✗") || strings.Contains(out, "•") {
		t.Errorf("CI renderer must not emit Unicode symbols:\n%s", out)
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("expected ASCII status OK in CI output:\n%s", out)
	}
}

func TestCIRenderer_DeterministicPlan(t *testing.T) {
	r := CIRenderer{}
	report := PlanReport{
		WouldUpdate: []PlanItem{
			{Name: "world", PackagePath: "example.com/world", InstallTarget: "example.com/world", Command: "go install example.com/world@latest"},
		},
	}
	out, err := captureOutput(t, func() error {
		return r.Plan(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "update: world") {
		t.Errorf("expected line-oriented update entry:\n%s", out)
	}
	if strings.Contains(out, "would-update: 0") {
		t.Errorf("expected would-update count to reflect plan:\n%s", out)
	}
}

func TestTerminalRenderer_UpdateSkippedIndented(t *testing.T) {
	r := TerminalRenderer{}
	report := UpdateReport{
		Updated: []string{"hello"},
		Skipped: []string{"/gobin/notgo"},
		Failed:  make([]string, 0),
	}
	out, err := captureOutput(t, func() error {
		return r.Update(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "  • /gobin/notgo") {
		t.Errorf("skipped bullets must use the same 2-space indent as other sections (regression):\n%s", out)
	}
}

func TestTerminalRenderer_OutdatedSummaryFromReport(t *testing.T) {
	r := TerminalRenderer{}
	report := OutdatedReport{
		Results: []OutdatedItemReport{
			{Name: "hello", Current: "v1.0.0", Outdated: false},
			{Name: "world", Current: "v1.2.0", Outdated: true},
		},
		Summary: OutdatedSummary{Outdated: 1, UpToDate: 1},
	}
	out, err := captureOutput(t, func() error {
		return r.Outdated(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The renderer must never re-count; it consumes the report summary.
	if !regexp.MustCompile(`(?m)^Outdated\s+1$`).MatchString(out) {
		t.Errorf("summary must reflect report.Summary.Outdated, not local counting (regression):\n%s", out)
	}
}

func TestTerminalRenderer_EmptyInventoryStillSummarizes(t *testing.T) {
	r := TerminalRenderer{}
	out, err := captureOutput(t, func() error {
		return r.Inventory(InventoryReport{})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"No Go tools found.", "Summary", "Healthy"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty inventory must keep the canonical summary rhythm; missing %q:\n%s", want, out)
		}
	}
	if !regexp.MustCompile(`(?m)^Healthy\s+0$`).MatchString(out) {
		t.Errorf("summary must end with the aligned zero totals (regression):\n%s", out)
	}
}

func TestCIRenderer_UpdateSummaryKeysUnambiguous(t *testing.T) {
	r := CIRenderer{}
	report := UpdateReport{
		Updated: []string{"hello"},
		Skipped: []string{"/gobin/notgo"},
		Failed:  make([]string, 0),
	}
	out, err := captureOutput(t, func() error {
		return r.Update(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"updated: hello", "skipped: /gobin/notgo", "updated-count: 1", "skipped-count: 1", "failed-count: 0"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in CI update output (regression):\n%s", want, out)
		}
	}
	// Count lines must be distinguishable from per-tool record lines.
	if strings.Contains(out, "\nupdated: 1\n") {
		t.Errorf("summary count must not reuse the record key 'updated:' (regression):\n%s", out)
	}
}

func TestJSONRenderer_InfoHasNoEnvelope(t *testing.T) {
	r := JSONRenderer{}
	loadRes := tool.LoadResult{
		Tools: []tool.Tool{makeTool("hello", "example.com/hello", "v1.0.0")},
	}
	out, err := captureOutput(t, func() error {
		return r.Info(loadRes, "hello")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, `"operation"`) || strings.Contains(out, `"success"`) {
		t.Errorf("--info --json must stay a bare ToolReport without the operation envelope (contract):\n%s", out)
	}
}

// TestTerminalRenderer_ProgressCheckmarkAlignment is the regression test for
// the v1.9.2 progress-column fix: the completion mark must land in the same
// column regardless of version-string length, and oversize transitions
// (long or unknown version strings) must never be truncated.
func TestTerminalRenderer_ProgressCheckmarkAlignment(t *testing.T) {
	r := TerminalRenderer{}
	base := makeTool("world", "example.com/world", "v1.0.0")

	alignCol := -1
	for _, resolved := range []string{"v1.3.0", "v0.5"} {
		out, _ := captureOutput(t, func() error {
			r.OnProgress(tool.Progress{Current: 1, Total: 1, Tool: base, Version: resolved, Action: "Start"})
			r.OnProgress(tool.Progress{Current: 1, Total: 1, Tool: base, Version: resolved, Action: "Complete", Success: true})
			return nil
		})
		idx := strings.Index(out, symCheck)
		if idx < 0 {
			t.Fatalf("expected %q in progress output for resolved %q:\n%s", symCheck, resolved, out)
		}
		if alignCol == -1 {
			alignCol = idx
		} else if idx != alignCol {
			t.Errorf("checkmark column %d != %d for resolved %q:\n%s", idx, alignCol, resolved, out)
		}
	}

	// Oversize transitions (e.g. an "unknown" current version combined with a
	// long resolved version) exceed the alignment column but must not be
	// truncated or mangled.
	for _, resolved := range []string{"unknown", "v2024.1.1"} {
		out, _ := captureOutput(t, func() error {
			r.OnProgress(tool.Progress{Current: 1, Total: 1, Tool: base, Version: resolved, Action: "Start"})
			r.OnProgress(tool.Progress{Current: 1, Total: 1, Tool: base, Version: resolved, Action: "Complete", Success: true})
			return nil
		})
		if !strings.Contains(out, "v1.0.0 → "+resolved) {
			t.Errorf("oversize transition for resolved %q must not be truncated:\n%s", resolved, out)
		}
	}
}

// TestTerminalRenderer_InstallationChildOrderDeterministic is the regression
// test for the v1.9.2 installation-tree fix: tools sharing a module must
// render in a stable name order regardless of the input detail ordering.
func TestTerminalRenderer_InstallationChildOrderDeterministic(t *testing.T) {
	r := TerminalRenderer{}
	zeta := UpdatedToolDetail{Name: "zeta", ModulePath: "example.com/suite", Previous: "v1.0.0", Resolved: "v1.2.0", PackagePath: "example.com/suite/cmd/zeta"}
	alpha := UpdatedToolDetail{Name: "alpha", ModulePath: "example.com/suite", Previous: "v1.0.0", Resolved: "v1.2.0", PackagePath: "example.com/suite/cmd/alpha"}

	reportFwd := UpdateReport{UpdatedDetail: []UpdatedToolDetail{alpha, zeta}}
	reportRev := UpdateReport{UpdatedDetail: []UpdatedToolDetail{zeta, alpha}}

	outFwd, err := captureOutput(t, func() error { return r.Update(reportFwd) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outRev, err := captureOutput(t, func() error { return r.Update(reportRev) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if outFwd != outRev {
		t.Errorf("installations output depends on input order (must sort children by name):\nfwd:\n%s\nrev:\n%s", outFwd, outRev)
	}
	alphaIdx := strings.Index(outFwd, "    alpha ")
	zetaIdx := strings.Index(outFwd, "    zeta ")
	if alphaIdx < 0 || zetaIdx < 0 {
		t.Fatalf("expected both alpha and zeta children under the module:\n%s", outFwd)
	}
	if alphaIdx > zetaIdx {
		t.Errorf("children not sorted by name (alpha after zeta):\n%s", outFwd)
	}
}
