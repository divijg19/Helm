package tool

import (
	"context"
	"debug/buildinfo"
	"errors"
	"runtime/debug"
	"testing"
)

type mockRunner struct {
	output string
	err    error
}

func (m mockRunner) Run(ctx context.Context, c Command) (string, error) {
	return m.output, m.err
}

func TestUpdateRunner(t *testing.T) {
	ctx := context.Background()
	bi := &buildinfo.BuildInfo{
		Path: "example.com/tool/cmd/tool",
		Main: debug.Module{
			Path:    "example.com/tool",
			Version: "v1.0.0",
		},
	}
	tool := Tool{
		name: "dummy",
		path: "/fake/path",
		info: bi,
	}

	// Test successful run with note
	runner := mockRunner{output: "deprecated warning\n"}
	results, _, diagnostics := Update(ctx, []Tool{tool}, nil, false, runner, nil)
	if len(results) != 1 {
		Fatalf(t, "Expected 1 result, got %d", len(results))
	}
	res := results[0]

	if !res.Success {
		t.Errorf("Expected success, got failure")
	}
	if len(res.Notes) != 1 || res.Notes[0] != "deprecated warning" {
		t.Errorf("Unexpected notes: %v", res.Notes)
	}
	if len(diagnostics) != 1 || diagnostics[0].Category != "Deprecation" {
		t.Errorf("Expected deprecation diagnostic, got %v", diagnostics)
	}

	// Test failed run
	failRunner := mockRunner{err: errors.New("network error")}
	resultsFail, _, _ := Update(ctx, []Tool{tool}, nil, false, failRunner, nil)
	if len(resultsFail) != 1 {
		Fatalf(t, "Expected 1 result, got %d", len(resultsFail))
	}
	resFail := resultsFail[0]

	if resFail.Success {
		t.Errorf("Expected failure, got success")
	}
}

// TestUpdateWarnSubstringNotDiagnostic proves diagnostic precision: only
// lines containing "warning" (or deprecation markers) become diagnostics.
// A bare "warn" substring — as in "unwarned" or "forewarn" — must not
// produce a Warning diagnostic.
func TestUpdateWarnSubstringNotDiagnostic(t *testing.T) {
	ctx := context.Background()
	bi := &buildinfo.BuildInfo{
		Path: "example.com/tool/cmd/tool",
		Main: debug.Module{
			Path:    "example.com/tool",
			Version: "v1.0.0",
		},
	}
	tool := Tool{
		name: "dummy",
		path: "/fake/path",
		info: bi,
	}

	runner := mockRunner{output: "unwarned forewarn notice\n"}
	_, _, diagnostics := Update(ctx, []Tool{tool}, nil, false, runner, nil)
	if len(diagnostics) != 0 {
		t.Errorf("Expected no diagnostics for warn-substring line, got %v", diagnostics)
	}

	warnRunner := mockRunner{output: "go: warning: something odd\n"}
	_, _, warnDiagnostics := Update(ctx, []Tool{tool}, nil, false, warnRunner, nil)
	if len(warnDiagnostics) != 1 || warnDiagnostics[0].Category != "Warning" {
		t.Errorf("Expected one Warning diagnostic, got %v", warnDiagnostics)
	}
}

func Fatalf(t *testing.T, format string, args ...any) {
	t.Fatalf(format, args...)
}
