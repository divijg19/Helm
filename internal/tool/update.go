package tool

import (
	"context"
	"strings"
	"time"
)

type Diagnostic struct {
	ToolName string
	Category string
	Message  string
}

type ToolUpdateResult struct {
	Tool Tool
	// Success reports whether the install completed. Error carries the reason
	// when it did not, and is what the non-terminal renderers report as the
	// failure cause.
	Success bool
	Notes   []string
	Error   error
}

type Progress struct {
	Current int
	Total   int
	Tool    Tool
	Version string // resolved version being installed (e.g. "v1.3.0")
	Action  string // "Start", "Output", "Complete"
	Line    string
	Success bool
	Notes   []string
	Error   error
}

// UpdateCandidates installs exactly the given candidates at their resolved
// versions, sequentially. Callers must only pass candidates produced by
// ResolveUpdateCandidates; the resolved version is installed verbatim and is
// never re-resolved to @latest. There is intentionally no fallback: an exact
// install failure is an ordinary update failure.
//
// The returned duration is the total wall time of the install phase only:
// outdated resolution happens beforehand in ResolveUpdateCandidates and is
// excluded. Renderers display it as the operation duration; it is not part of
// the JSON contract.
func UpdateCandidates(ctx context.Context, candidates []UpdateCandidate, runner Runner, onProgress func(Progress)) ([]ToolUpdateResult, time.Duration, []Diagnostic) {
	start := time.Now()
	if runner == nil {
		runner = DefaultRunner{}
	}

	results := make([]ToolUpdateResult, 0, len(candidates))
	var diagnostics []Diagnostic

	total := len(candidates)
	for i, c := range candidates {
		res, diags := installTool(ctx, c.Tool, c.Version, i+1, total, runner, onProgress)
		results = append(results, res)
		diagnostics = append(diagnostics, diags...)
	}

	return results, time.Since(start), diagnostics
}

// isFetchEvent reports whether a toolchain line describes module fetching.
// Fetch events stream to progress and persist in notes verbatim, but they
// never feed the deprecation/warning diagnostics classifier: a module path
// can contain those words without anything being wrong.
func isFetchEvent(line string) bool {
	return strings.HasPrefix(line, "go: downloading") || strings.HasPrefix(line, "go: extracting")
}

// installTool executes one installation of t at the already-resolved version
// and reports the outcome through the shared progress/diagnostics contract.
// The install reference is derived here so there is exactly one source of
// truth for it: the resolved version and the package it came from.
func installTool(ctx context.Context, t Tool, resolvedVersion string, current, total int, runner Runner, onProgress func(Progress)) (ToolUpdateResult, []Diagnostic) {
	var diagnostics []Diagnostic

	if onProgress != nil {
		onProgress(Progress{
			Current: current,
			Total:   total,
			Tool:    t,
			Version: resolvedVersion,
			Action:  "Start",
		})
	}

	output, err := runner.Run(ctx, Command{
		Name: "go",
		Args: []string{"install", InstallExactRef(t.InstallTarget(), resolvedVersion)},
		OnLine: func(line string) {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				return
			}
			if onProgress != nil {
				onProgress(Progress{
					Current: current,
					Total:   total,
					Tool:    t,
					Action:  "Output",
					Line:    trimmed,
				})
			}
		},
	})

	success := err == nil

	var notes []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		notes = append(notes, line)
		if isFetchEvent(line) {
			continue
		}

		lower := strings.ToLower(line)
		if strings.Contains(lower, "deprecated") || strings.Contains(lower, "deprecation") {
			diagnostics = append(diagnostics, Diagnostic{
				ToolName: t.Name(),
				Category: "Deprecation",
				Message:  line,
			})
		} else if strings.Contains(lower, "warning") {
			diagnostics = append(diagnostics, Diagnostic{
				ToolName: t.Name(),
				Category: "Warning",
				Message:  line,
			})
		}
	}

	prog := Progress{
		Current: current,
		Total:   total,
		Tool:    t,
		Action:  "Complete",
		Success: success,
		Notes:   notes,
		Error:   err,
	}
	// Every non-empty line already streamed live through OnLine when a
	// progress sink is present (only the terminal renderer implements it),
	// so a successful completion does not repeat them: the live subtree is
	// the record, not an after-note. Failures keep the full notes so the
	// error and its context stay co-located for triage. Revisit this if
	// output filtering ever returns.
	if onProgress != nil && success {
		prog.Notes = nil
	}
	if onProgress != nil {
		onProgress(prog)
	}

	return ToolUpdateResult{
		Tool:    t,
		Success: success,
		Notes:   notes,
		Error:   err,
	}, diagnostics
}
