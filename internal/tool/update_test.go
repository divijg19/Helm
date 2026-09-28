package tool

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type mockRunner struct {
	output string
	err    error
}

func (m mockRunner) Run(ctx context.Context, c Command) (string, error) {
	return m.output, m.err
}

// resolvedCandidate wraps a fixture tool in the resolved candidate the production
// update path consumes. Every test in this file drives UpdateCandidates, the
// only install entry point production uses, so the streaming, notes, and
// diagnostics contract is covered on the live path rather than on a
// test-only wrapper.
func resolvedCandidate(tl Tool, version string) []UpdateCandidate {
	return []UpdateCandidate{{Tool: tl, Version: version}}
}

func TestUpdateCandidates_Runner(t *testing.T) {
	ctx := context.Background()
	tool := moduleTool("dummy", "v1.0.0")

	// Successful run: output is persisted as notes and classified.
	runner := mockRunner{output: "deprecated warning\n"}
	results, _, diagnostics := UpdateCandidates(ctx, resolvedCandidate(tool, "v1.2.0"), runner, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]

	if !res.Success {
		t.Error("expected success, got failure")
	}
	if len(res.Notes) != 1 || res.Notes[0] != "deprecated warning" {
		t.Errorf("unexpected notes: %v", res.Notes)
	}
	if len(diagnostics) != 1 || diagnostics[0].Category != "Deprecation" {
		t.Errorf("expected deprecation diagnostic, got %v", diagnostics)
	}

	// Failed run.
	failRunner := mockRunner{err: errors.New("network error")}
	resultsFail, _, _ := UpdateCandidates(ctx, resolvedCandidate(tool, "v1.2.0"), failRunner, nil)
	if len(resultsFail) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resultsFail))
	}
	if resultsFail[0].Success {
		t.Error("expected failure, got success")
	}
}

// TestUpdateCandidates_InstallsResolvedVersionExactly proves the production
// install path pins the version that was just resolved: the runner is handed
// <package>@<resolved>, never a floating @latest reference.
func TestUpdateCandidates_InstallsResolvedVersionExactly(t *testing.T) {
	var gotArgs []string
	recorder := runnerFunc(func(ctx context.Context, c Command) (string, error) {
		gotArgs = c.Args
		return "", nil
	})

	resolved := "v1.2.3"
	UpdateCandidates(context.Background(), resolvedCandidate(moduleTool("dummy", "v1.0.0"), resolved), recorder, nil)
	want := []string{"install", "example.com/dummy/cmd/dummy@" + resolved}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("install args = %v, want %v", gotArgs, want)
	}
}

// TestUpdateCandidates_WarnSubstringNotDiagnostic proves diagnostic precision:
// only lines containing "warning" (or deprecation markers) become diagnostics.
// A bare "warn" substring — as in "unwarned" or "forewarn" — must not
// produce a Warning diagnostic.
func TestUpdateCandidates_WarnSubstringNotDiagnostic(t *testing.T) {
	ctx := context.Background()
	tool := moduleTool("dummy", "v1.0.0")

	runner := mockRunner{output: "unwarned forewarn notice\n"}
	_, _, diagnostics := UpdateCandidates(ctx, resolvedCandidate(tool, "v1.2.0"), runner, nil)
	if len(diagnostics) != 0 {
		t.Errorf("expected no diagnostics for warn-substring line, got %v", diagnostics)
	}

	warnRunner := mockRunner{output: "go: warning: something odd\n"}
	_, _, warnDiagnostics := UpdateCandidates(ctx, resolvedCandidate(tool, "v1.2.0"), warnRunner, nil)
	if len(warnDiagnostics) != 1 || warnDiagnostics[0].Category != "Warning" {
		t.Errorf("expected one Warning diagnostic, got %v", warnDiagnostics)
	}
}

// replayRunner replays fixed lines through the OnLine callback exactly as a
// streaming subprocess would, then returns them joined as the full output.
type replayRunner struct {
	lines []string
	err   error
}

func (r replayRunner) Run(ctx context.Context, c Command) (string, error) {
	var sb strings.Builder
	for _, l := range r.lines {
		if c.OnLine != nil {
			c.OnLine(l)
		}
		sb.WriteString(l + "\n")
	}
	return sb.String(), r.err
}

// runnerFunc adapts a function to the Runner interface.
type runnerFunc func(ctx context.Context, c Command) (string, error)

func (f runnerFunc) Run(ctx context.Context, c Command) (string, error) { return f(ctx, c) }

// TestUpdateCandidates_FetchEventsStreamAndPersist pins the no-magic contract:
// toolchain fetch lines stream to progress verbatim and persist in notes,
// but never feed the diagnostics classifier (module paths may contain words
// like "warning" without anything being wrong).
func TestUpdateCandidates_FetchEventsStreamAndPersist(t *testing.T) {
	ctx := context.Background()
	tool := moduleTool("dummy", "v1.0.0")

	fetch := []string{
		"go: downloading example.com/dummy v1.2.0",
		"go: extracting example.com/dummy v1.2.0",
		"go: downloading example.com/warning v1.0.0",
	}
	var actions []Progress
	runner := replayRunner{lines: fetch}
	results, _, diagnostics := UpdateCandidates(ctx, resolvedCandidate(tool, "v1.2.0"), runner, func(p Progress) {
		actions = append(actions, p)
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	var streamed []string
	for _, p := range actions {
		if p.Action == "Output" {
			streamed = append(streamed, p.Line)
		}
	}
	if !reflect.DeepEqual(streamed, fetch) {
		t.Errorf("streamed fetch lines = %v, want %v verbatim and in order", streamed, fetch)
	}
	if !reflect.DeepEqual(results[0].Notes, fetch) {
		t.Errorf("persisted notes = %v, want %v", results[0].Notes, fetch)
	}
	if len(diagnostics) != 0 {
		t.Errorf("fetch lines must not feed diagnostics, got %v", diagnostics)
	}
}

// TestUpdateCandidates_CompleteNotesLiveOnlyOnSuccess pins the no-repeat
// contract: lines already streamed live are not repeated in a successful
// completion block, while failures keep the full notes co-located with
// the error for triage. Report-level notes stay complete in both cases.
func TestUpdateCandidates_CompleteNotesLiveOnlyOnSuccess(t *testing.T) {
	ctx := context.Background()
	tool := moduleTool("dummy", "v1.0.0")
	lines := []string{"go: downloading example.com/dummy v1.2.0", "built ok"}

	var progs []Progress
	collect := func(p Progress) { progs = append(progs, p) }
	completeNotes := func() []string {
		for _, p := range progs {
			if p.Action == "Complete" {
				return p.Notes
			}
		}
		return nil
	}

	results, _, _ := UpdateCandidates(ctx, resolvedCandidate(tool, "v1.2.0"), replayRunner{lines: lines}, collect)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected 1 successful result, got %+v", results)
	}
	if got := completeNotes(); len(got) != 0 {
		t.Errorf("successful Complete.Notes = %v, want empty (already streamed live)", got)
	}
	if !reflect.DeepEqual(results[0].Notes, lines) {
		t.Errorf("result notes = %v, want full %v", results[0].Notes, lines)
	}

	var failProgs []Progress
	failResults, _, _ := UpdateCandidates(ctx, resolvedCandidate(tool, "v1.2.0"), replayRunner{lines: lines, err: errors.New("boom")}, func(p Progress) {
		failProgs = append(failProgs, p)
	})
	if len(failResults) != 1 || failResults[0].Success {
		t.Fatalf("expected 1 failed result, got %+v", failResults)
	}
	var failNotes []string
	for _, p := range failProgs {
		if p.Action == "Complete" {
			failNotes = p.Notes
		}
	}
	if !reflect.DeepEqual(failNotes, lines) {
		t.Errorf("failed Complete.Notes = %v, want full %v", failNotes, lines)
	}
}

// TestUpdateCandidates_ProgressSequenceAndIndexing pins the progress
// contract the terminal renderer renders: a Start event, one Output event per
// streamed line, and a terminal Complete event, numbered 1-of-N across the
// candidate set so the "[01/01]" column is correct.
func TestUpdateCandidates_ProgressSequenceAndIndexing(t *testing.T) {
	ctx := context.Background()
	first := moduleTool("first", "v1.0.0")
	second := moduleTool("second", "v1.0.0")
	candidates := []UpdateCandidate{
		{Tool: first, Version: "v1.1.0"},
		{Tool: second, Version: "v2.0.0"},
	}

	var actions []Progress
	UpdateCandidates(ctx, candidates, replayRunner{lines: []string{"go: downloading x v1"}}, func(p Progress) {
		actions = append(actions, p)
	})

	want := []struct {
		action  string
		current int
		total   int
		version string
	}{
		{"Start", 1, 2, "v1.1.0"},
		{"Output", 1, 2, ""},
		{"Complete", 1, 2, ""},
		{"Start", 2, 2, "v2.0.0"},
		{"Output", 2, 2, ""},
		{"Complete", 2, 2, ""},
	}
	if len(actions) != len(want) {
		t.Fatalf("got %d progress events, want %d: %+v", len(actions), len(want), actions)
	}
	for i, w := range want {
		got := actions[i]
		if got.Action != w.action || got.Current != w.current || got.Total != w.total {
			t.Errorf("event %d = {%s %d/%d}, want {%s %d/%d}", i, got.Action, got.Current, got.Total, w.action, w.current, w.total)
		}
		if w.version != "" && got.Version != w.version {
			t.Errorf("event %d version = %q, want %q", i, got.Version, w.version)
		}
	}
}
