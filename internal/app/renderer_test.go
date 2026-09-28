package app

import (
	"bytes"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/divijg19/Helm/internal/tool"
)

func captureOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// os.Stdout is process-global, so the swap must be undone even if fn
	// panics: leaving it bound to a closed pipe would poison every later
	// test in the package.
	os.Stdout = w
	defer func() {
		os.Stdout = old
		w.Close()
		r.Close()
	}()

	runErr := fn()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String(), runErr
}

// captureStderr captures os.Stderr, which the failure and diagnostic paths
// write to. captureOutput only sees stdout, so those paths would otherwise go
// unasserted.
func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = old
		w.Close()
		r.Close()
	}()

	runErr := fn()

	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String(), runErr
}

// TestQuietRenderer_UpdateReportsFailures pins the quiet mode failure
// contract, which is the whole point of the scripting mode: failed tools and
// diagnostics go to stderr, a summary line is still printed, and a non-nil
// error is returned so the process exits non-zero. This path wrote to stderr
// and was previously asserted by no test.
func TestQuietRenderer_UpdateReportsFailures(t *testing.T) {
	report := UpdateReport{
		Updated:     []string{"hello"},
		Failed:      []string{"world", "broken"},
		Skipped:     []string{"localdev"},
		Diagnostics: []tool.Diagnostic{{ToolName: "world", Category: "Deprecation", Message: "example.com/world is deprecated"}},
	}

	var out string
	errText, err := captureStderr(t, func() error {
		var innerErr error
		out, innerErr = captureOutput(t, func() error {
			return QuietRenderer{}.Update(report)
		})
		return innerErr
	})
	if err == nil {
		t.Fatal("expected a non-nil error when tools failed, got nil")
	}
	if want := "2 updates failed"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	for _, want := range []string{"failed: world", "failed: broken"} {
		if !strings.Contains(errText, want) {
			t.Errorf("expected %q on stderr, got:\n%s", want, errText)
		}
	}
	if !strings.Contains(errText, "diagnostic: world: example.com/world is deprecated") {
		t.Errorf("expected the diagnostic on stderr, got:\n%s", errText)
	}
	// Quiet mode keeps the summary on stdout: scripts still need the counts.
	for _, want := range []string{"Updated", "Failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the quiet summary, got:\n%s", want, out)
		}
	}
	// Quiet mode must stay free of chatter and headers.
	if strings.Contains(out, "Discovery") || strings.Contains(out, "Installations") {
		t.Errorf("quiet output must not carry headers, got:\n%s", out)
	}
}

func makeTool(name, pkg, version string) tool.Tool {
	return tool.NewTool(name, "/gobin/"+name, &buildinfo.BuildInfo{
		Path: pkg + "/cmd/" + name,
		Main: debug.Module{Path: pkg, Version: version},
	})
}

func TestColumnWidths(t *testing.T) {
	if got := columnWidths([]int{4, 7}, []string{"hello", "v1.0.0"}, []string{"worldwide", "v1.2.0"}); !reflect.DeepEqual(got, []int{9, 7}) {
		t.Errorf("columnWidths = %v, want [9 7] (max wins, floor holds)", got)
	}
	if got := columnWidths([]int{16}, []string{"hi"}); !reflect.DeepEqual(got, []int{16}) {
		t.Errorf("columnWidths = %v, want [16] (floor holds for short cells)", got)
	}
	if got := columnWidths([]int{4, 7}); !reflect.DeepEqual(got, []int{4, 7}) {
		t.Errorf("columnWidths with no rows = %v, want minimums", got)
	}
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
		Summary: InventorySummary{Healthy: 1, Invalid: 1, Unhealthy: 0},
		OperationEnvelope: OperationEnvelope{
			Operation: OperationList,
			Success:   false,
		},
	}
	out, err := captureOutput(t, func() error {
		return r.Inventory(report)
	})
	if err == nil {
		t.Error("expected error when inventory has issues")
	}
	for _, want := range []string{`"operation": "list"`, `"success": false`, `"invalid": [`, `/gobin/notgo`, `"healthy": 1`, `"invalid": 1`, `"unhealthy": 0`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %s in inventory JSON:\n%s", want, out)
		}
	}
}

// TestInventoryReport_EmptyToolsSerializeAsArray proves the never-null array
// contract holds for the inventory report built from an empty GOBIN: the
// tools array must serialize as [] and never as null.
func TestInventoryReport_EmptyToolsSerializeAsArray(t *testing.T) {
	report := (&App{}).inventoryReport(tool.LoadResult{})

	if report.Tools == nil {
		t.Error("report.Tools = nil, want a non-nil empty slice")
	}
	out, err := captureOutput(t, func() error {
		return JSONRenderer{}.Inventory(report)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, `"tools": null`) {
		t.Errorf("tools array serialized as null, violating the never-null contract:\n%s", out)
	}
	if !strings.Contains(out, `"tools": []`) {
		t.Errorf("expected \"tools\": [] for an empty GOBIN, got:\n%s", out)
	}
}

func TestJSONRenderer_OutdatedNilResultsEmitEmptyArray(t *testing.T) {
	r := JSONRenderer{}
	out, err := captureOutput(t, func() error {
		return r.Outdated(OutdatedReport{
			OperationEnvelope: OperationEnvelope{Operation: OperationOutdated, Success: true},
		})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"results": []`) {
		t.Errorf("nil results must serialize as an empty array, got:\n%s", out)
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

// TestCIRenderer_StatusTokens pins the full status-token mapping. The goldens
// only ever contain Healthy and Local, because the CLI fixture's single
// unhealthy entry is an invalid binary rather than an unhealthy tool, so the
// Unhealthy and error paths went unexercised.
func TestCIRenderer_StatusTokens(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{statusHealthy, "OK"},
		{statusLocal, "LOCAL"},
		{statusUnhealthy, "ERROR"},
		// An unrecognized status passes through rather than being silently
		// relabeled, so a producer-side typo stays visible in CI output.
		{"Helthy", "Helthy"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := statusToken(tt.status); got != tt.want {
				t.Errorf("statusToken(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

// TestCIRenderer_InventoryUnhealthy pins that an unhealthy tool renders with
// the ERROR token and its per-tool error on stderr, and that the run reports
// the issue total.
func TestCIRenderer_InventoryUnhealthy(t *testing.T) {
	report := InventoryReport{
		Tools: []ToolInventoryItem{
			{Name: "hello", Version: "v1.0.0", PackagePath: "example.com/hello", Status: statusHealthy},
			{Name: "broken", Version: "v1.0.0", PackagePath: "example.com/broken", Status: statusUnhealthy, Error: "not executable"},
		},
		Summary: InventorySummary{Healthy: 1, Unhealthy: 1},
	}
	var out string
	errText, err := captureStderr(t, func() error {
		var innerErr error
		out, innerErr = captureOutput(t, func() error {
			return CIRenderer{}.Inventory(report)
		})
		return innerErr
	})
	if err == nil || err.Error() != "1 issue found during inventory check" {
		t.Errorf("error = %v, want 1 issue found during inventory check", err)
	}
	if !strings.Contains(out, "ERROR") {
		t.Errorf("expected the ERROR token for an unhealthy tool:\n%s", out)
	}
	if !strings.Contains(errText, "error: broken: not executable") {
		t.Errorf("expected the per-tool error on stderr, got:\n%s", errText)
	}
	if !strings.Contains(out, "unhealthy: 1") {
		t.Errorf("expected the unhealthy count line:\n%s", out)
	}
	if !strings.Contains(out, "broken") {
		t.Errorf("expected the unhealthy tool to be listed:\n%s", out)
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
	if !strings.Contains(out, "skipped: 0") {
		t.Errorf("expected skipped count alongside would-update count:\n%s", out)
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
		Summary: OutdatedSummary{Total: 2, Outdated: 1, UpToDate: 1},
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
	// Checked comes from Summary.Total, so a report whose Total disagrees
	// with its result count must still render Total, not a re-count.
	if !regexp.MustCompile(`(?m)^Checked\s+2$`).MatchString(out) {
		t.Errorf("summary must render Summary.Total, not a re-count of Results (regression):\n%s", out)
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

// TestFormatDuration pins the duration format across the one-minute
// boundary. Only the sub-minute form ever appeared in a golden, so a
// regression in the minutes computation or the "XmY.Zs" layout was invisible.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"zero", 0, "0.0s"},
		{"sub-minute rounds to tenths", 1500 * time.Millisecond, "1.5s"},
		{"just under a minute stays in seconds", 59*time.Second + 900*time.Millisecond, "59.9s"},
		{"exactly a minute switches form", time.Minute, "1m0.0s"},
		{"past a minute keeps the remainder", 63*time.Second + 200*time.Millisecond, "1m3.2s"},
		{"many minutes", 62 * time.Minute, "62m0.0s"},
		{"hours are reported as minutes", 2 * time.Hour, "120m0.0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDuration(tt.in); got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestUpdateReport_FailedDetailCarriesInstallCauses proves a failed install
// reaches every non-terminal renderer with its reason, not just its name.
// Before FailedDetail existed, --json/--ci/--quiet reported `failed: world`
// with no cause, so a script could not tell a network failure from a build
// failure. The terminal must keep its live presentation instead.
func TestUpdateReport_FailedDetailCarriesInstallCauses(t *testing.T) {
	report := UpdateReport{
		OperationEnvelope: OperationEnvelope{Operation: OperationUpdate},
		Failed:            []string{"world", "broken"},
		FailedDetail: []ToolFailure{
			{Name: "world", Error: "dial tcp: lookup proxy.example: no such host"},
		},
	}

	t.Run("ci reports the cause", func(t *testing.T) {
		out, err := captureOutput(t, func() error { return CIRenderer{}.Update(report) })
		if err == nil || err.Error() != "2 updates failed" {
			t.Errorf("err = %v, want 2 updates failed", err)
		}
		if !strings.Contains(out, "failed: world\n") || !strings.Contains(out, "failed: broken\n") {
			t.Errorf("expected one failed line per tool, got:\n%s", out)
		}
		if !strings.Contains(out, "failed-reason: world: dial tcp: lookup proxy.example: no such host") {
			t.Errorf("expected the install cause, got:\n%s", out)
		}
		// broken has no recorded cause and must not be given a fabricated one.
		if strings.Contains(out, "failed-reason: broken") {
			t.Errorf("must not invent a reason for a tool without one, got:\n%s", out)
		}
	})

	t.Run("quiet reports the cause on stderr once", func(t *testing.T) {
		errText, err := captureStderr(t, func() error {
			_, inner := captureOutput(t, func() error { return QuietRenderer{}.Update(report) })
			return inner
		})
		if err == nil {
			t.Fatal("expected a non-nil error")
		}
		if strings.Count(errText, "failed: world") != 1 {
			t.Errorf("world must be reported exactly once, got:\n%s", errText)
		}
		if !strings.Contains(errText, "failed: world: dial tcp") {
			t.Errorf("expected the cause alongside the name, got:\n%s", errText)
		}
		if !strings.Contains(errText, "failed: broken\n") {
			t.Errorf("a tool without a cause must still be listed, got:\n%s", errText)
		}
	})

	t.Run("terminal does not repeat the cause", func(t *testing.T) {
		out, err := captureOutput(t, func() error { return TerminalRenderer{}.Update(report) })
		if err == nil {
			t.Fatal("expected a non-nil error")
		}
		if !strings.Contains(out, "- world") {
			t.Errorf("expected the failed tool listed, got:\n%s", out)
		}
		if strings.Contains(out, "dial tcp") {
			t.Errorf("the terminal already showed the cause live; it must not repeat it, got:\n%s", out)
		}
	})
}

// TestUpdateReport_FailedDetailOmittedWhenNothingFailed pins that a successful
// run gains no new JSON key, keeping the wire shape stable.
func TestUpdateReport_FailedDetailOmittedWhenNothingFailed(t *testing.T) {
	out, err := captureOutput(t, func() error {
		return JSONRenderer{}.Update(UpdateReport{
			OperationEnvelope: OperationEnvelope{Operation: OperationUpdate, Success: true},
			Updated:           []string{"world"},
		})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "failed_detail") {
		t.Errorf("a successful run must not emit failed_detail, got:\n%s", out)
	}
}

// TestTerminalRenderer_ProgressFailureRendering pins what a user sees when an
// install fails. This is the only place the error and the toolchain's own
// output are shown together, and no test or golden exercised it: the CLI
// fixture's single failure mode is offline resolution, which happens before any
// install starts, so no progress event is ever emitted for it.
func TestTerminalRenderer_ProgressFailureRendering(t *testing.T) {
	failing := makeTool("world", "example.com/world", "v1.2.0")

	var out string
	errText, err := captureStderr(t, func() error {
		var inner error
		out, inner = captureOutput(t, func() error {
			TerminalRenderer{}.OnProgress(tool.Progress{
				Current: 1, Total: 1, Tool: failing, Version: "v1.3.0", Action: "Start",
			})
			TerminalRenderer{}.OnProgress(tool.Progress{
				Current: 1, Total: 1, Tool: failing, Action: "Output",
				Line: "go: build example.com/world: build constraints exclude all Go files",
			})
			TerminalRenderer{}.OnProgress(tool.Progress{
				Current: 1, Total: 1, Tool: failing, Action: "Complete",
				Success: false,
				Error:   errors.New("exit status 1"),
				Notes:   []string{"go: build example.com/world: build constraints exclude all Go files"},
			})
			return nil
		})
		return inner
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errText != "" {
		t.Errorf("failure rendering must stay on stdout, got stderr:\n%s", errText)
	}

	for _, want := range []string{
		"[01/01] world",
		"v1.2.0 → v1.3.0",
		"build constraints exclude all Go files",
		symFail,
		"Error",
		"exit status 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in failure output:\n%s", want, out)
		}
	}
	// A failure must never be reported with the success mark.
	if strings.Contains(out, symCheck) {
		t.Errorf("failure output must not contain the success mark:\n%s", out)
	}
	// The captured output appears twice on purpose: once streamed live, once
	// beside the error. That co-location is the point of the failure branch.
	if n := strings.Count(out, "build constraints exclude all Go files"); n != 2 {
		t.Errorf("captured line appeared %d times, want 2 (streamed live and beside the error):\n%s", n, out)
	}
}

// TestTerminalRenderer_ProgressSuccessHasNoNotesSection pins that a successful
// completion is just the checkmark. installTool drops notes on success
// precisely so the live stream is not repeated, so this asserts the absence
// of a trailing notes/package block rather than the presence of one.
func TestTerminalRenderer_ProgressSuccessHasNoNotesSection(t *testing.T) {
	installed := makeTool("world", "example.com/world", "v1.3.0")
	r := TerminalRenderer{}
	out, err := captureOutput(t, func() error {
		r.OnProgress(tool.Progress{Current: 1, Total: 1, Tool: installed, Version: "v1.3.0", Action: "Start"})
		r.OnProgress(tool.Progress{Current: 1, Total: 1, Tool: installed, Action: "Output", Line: "go: downloading example.com/world v1.3.0"})
		r.OnProgress(tool.Progress{Current: 1, Total: 1, Tool: installed, Action: "Complete", Success: true})
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, symCheck) {
		t.Errorf("expected the success mark:\n%s", out)
	}
	if strings.Contains(out, "Package") {
		t.Errorf("success must not repeat the install target:\n%s", out)
	}
	if n := strings.Count(out, "go: downloading example.com/world v1.3.0"); n != 1 {
		t.Errorf("live line appeared %d times, want exactly 1 (never repeated):\n%s", n, out)
	}
}

// TestNewRenderer_ModeMapping pins the mode-to-renderer selection and the
// verbose pass-through. The mapping was previously asserted nowhere, so a
// swapped case would silently change which output format a user's flag
// produces, and the verbose field only reached terminal and CI.
func TestNewRenderer_ModeMapping(t *testing.T) {
	tests := []struct {
		mode        RenderMode
		wantType    string
		wantVerbose bool
	}{
		{ModeTerminal, "TerminalRenderer", true},
		{ModeQuiet, "QuietRenderer", false},
		{ModeCI, "CIRenderer", true},
		{ModeJSON, "JSONRenderer", false},
	}
	for _, tt := range tests {
		t.Run(tt.wantType, func(t *testing.T) {
			r := NewRenderer(tt.mode, true)
			// %T qualifies with the package name; trim it so the expected
			// names stay readable.
			if got := strings.TrimPrefix(fmt.Sprintf("%T", r), "app."); got != tt.wantType {
				t.Errorf("NewRenderer(%v) = %s, want %s", tt.mode, got, tt.wantType)
			}
			// Only the human renderers carry verbose; the others must not be
			// given it, since they ignore it by design.
			var gotVerbose bool
			switch v := r.(type) {
			case TerminalRenderer:
				gotVerbose = v.verbose
			case CIRenderer:
				gotVerbose = v.verbose
			default:
				gotVerbose = false
			}
			if gotVerbose != tt.wantVerbose {
				t.Errorf("verbose = %v, want %v for %s", gotVerbose, tt.wantVerbose, tt.wantType)
			}
		})
	}
}

// TestCIRenderer_Outdated covers all three mutually exclusive result branches
// of the CI outdated report, which no test exercised.
func TestCIRenderer_Outdated(t *testing.T) {
	report := OutdatedReport{
		OperationEnvelope: OperationEnvelope{Operation: OperationOutdated},
		Results: []OutdatedItemReport{
			{Name: "world", Current: "v1.2.0", Latest: "v1.3.0", Outdated: true},
			{Name: "hello", Current: "v1.0.0", Outdated: false},
			{Name: "broken", Current: "v1.0.0", Error: "dial tcp: i/o timeout"},
		},
		Summary: OutdatedSummary{Total: 3, Outdated: 1, UpToDate: 1, Failed: 1},
	}
	out, err := captureOutput(t, func() error { return CIRenderer{}.Outdated(report) })
	if err == nil || err.Error() != "1 outdated check failed" {
		t.Errorf("err = %v, want 1 outdated check failed", err)
	}
	for _, want := range []string{
		"world", "outdated: v1.2.0 -> v1.3.0",
		"hello", "up-to-date: v1.0.0",
		"broken", "error: dial tcp: i/o timeout",
		"checked: 3", "outdated: 1", "up-to-date: 1", "failed: 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in CI outdated output:\n%s", want, out)
		}
	}

	clean := OutdatedReport{
		OperationEnvelope: OperationEnvelope{Operation: OperationOutdated, Success: true},
		Results:           []OutdatedItemReport{{Name: "hello", Current: "v1.0.0"}},
		Summary:           OutdatedSummary{Total: 1, UpToDate: 1},
	}
	cleanOut, err := captureOutput(t, func() error { return CIRenderer{}.Outdated(clean) })
	if err != nil {
		t.Errorf("a clean report must not fail, got %v", err)
	}
	if !strings.Contains(cleanOut, "checked: 1") {
		t.Errorf("expected the checked count from Summary.Total:\n%s", cleanOut)
	}
}

// TestCIRenderer_Info covers the CI tool-metadata report, which had no test.
func TestCIRenderer_Info(t *testing.T) {
	loadRes := tool.LoadResult{Tools: []tool.Tool{makeTool("hello", "example.com/hello", "v1.0.0")}}
	out, err := captureOutput(t, func() error { return CIRenderer{}.Info(loadRes, "hello") })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"name: hello", "package: example.com/hello/cmd/hello",
		"module: example.com/hello", "version: v1.0.0", "can-update: true",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in CI info output:\n%s", want, out)
		}
	}

	// A missing target must fail identically to the other renderers.
	if _, err := captureOutput(t, func() error { return CIRenderer{}.Info(loadRes, "nosuch") }); err == nil {
		t.Error("expected an error for an unknown --info target in CI mode")
	}
}

// TestCIRenderer_PlanVerbose pins the verbose planning view in CI, which
// duplicated the terminal's behavior and was never taken by any test.
func TestCIRenderer_PlanVerbose(t *testing.T) {
	report := PlanReport{
		OperationEnvelope: OperationEnvelope{Operation: OperationCheck, Success: true},
		WouldUpdate: []PlanItem{
			{Name: "world", PackagePath: "example.com/world", InstallTarget: "example.com/world", Command: "go install example.com/world@latest"},
		},
	}
	concise, err := captureOutput(t, func() error { return CIRenderer{}.Plan(report) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(concise, "go install") {
		t.Errorf("concise CI plan must not show install commands:\n%s", concise)
	}
	verbose, err := captureOutput(t, func() error { return CIRenderer{verbose: true}.Plan(report) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(verbose, "go install example.com/world@latest") {
		t.Errorf("verbose CI plan must show the install command:\n%s", verbose)
	}
}

// TestCountPhrase pins singular/plural agreement for the user-facing failure
// messages. These strings are what a user reads on stderr, and a single problem
// must not be reported as "1 issues found".
func TestCountPhrase(t *testing.T) {
	tests := []struct {
		n        int
		singular string
		plural   string
		want     string
	}{
		{0, "issue", "issues", "0 issues"},
		{1, "issue", "issues", "1 issue"},
		{2, "issue", "issues", "2 issues"},
		{7, "issue", "issues", "7 issues"},
		{1, "outdated check", "outdated checks", "1 outdated check"},
		{3, "update", "updates", "3 updates"},
	}
	for _, tt := range tests {
		if got := countPhrase(tt.n, tt.singular, tt.plural); got != tt.want {
			t.Errorf("countPhrase(%d, %q, %q) = %q, want %q", tt.n, tt.singular, tt.plural, got, tt.want)
		}
	}
}

// TestReportErr_MessageWording pins the exact wording of all three operation
// failure messages, including the plural forms. Only the singular form changed
// when pluralization was introduced, so the plural strings are asserted too:
// they are user-visible and were previously unguarded.
func TestReportErr_MessageWording(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"inventory single issue", InventoryReport{Summary: InventorySummary{Unhealthy: 1}}.Err(), "1 issue found during inventory check"},
		{"inventory two issues", InventoryReport{Summary: InventorySummary{Unhealthy: 1, Invalid: 1}}.Err(), "2 issues found during inventory check"},
		{"inventory clean", InventoryReport{}.Err(), ""},
		{"outdated single", OutdatedReport{Summary: OutdatedSummary{Failed: 1}}.Err(), "1 outdated check failed"},
		{"outdated several", OutdatedReport{Summary: OutdatedSummary{Failed: 3}}.Err(), "3 outdated checks failed"},
		{"outdated clean", OutdatedReport{}.Err(), ""},
		{"update single", UpdateReport{Failed: []string{"world"}}.Err(), "1 update failed"},
		{"update several", UpdateReport{Failed: []string{"a", "b"}}.Err(), "2 updates failed"},
		{"update clean", UpdateReport{}.Err(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want == "" {
				if tt.err != nil {
					t.Fatalf("expected no error, got %q", tt.err)
				}
				return
			}
			if tt.err == nil {
				t.Fatalf("expected %q, got nil", tt.want)
			}
			if tt.err.Error() != tt.want {
				t.Errorf("error = %q, want %q", tt.err, tt.want)
			}
		})
	}
}
