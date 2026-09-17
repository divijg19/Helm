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
	Tool    Tool
	Status  Status
	Success bool
	Notes   []string
	Error   error
}

type Progress struct {
	Current int
	Total   int
	Tool    Tool
	Version string // resolved version being installed (e.g. "v1.3.0"), empty for legacy path
	Action  string // "Start", "Output", "Complete", "Skipped"
	Line    string
	Status  Status
	Success bool
	Notes   []string
	Error   error
}

func Update(ctx context.Context, tools []Tool, filter []string, dryRun bool, runner Runner, onProgress func(Progress)) ([]ToolUpdateResult, time.Duration, []Diagnostic) {
	start := time.Now()
	if runner == nil {
		runner = DefaultRunner{}
	}

	set := nameSet(filter)

	var total int
	for _, t := range tools {
		if !selected(t.Name(), set) {
			continue
		}
		total++
	}

	var results []ToolUpdateResult
	var diagnostics []Diagnostic
	var current int

	for _, t := range tools {
		if !selected(t.Name(), set) {
			continue
		}
		current++

		if !t.CanUpdate() {
			prog := Progress{
				Current: current,
				Total:   total,
				Tool:    t,
				Action:  "Skipped",
				Status:  StatusSkippedLocal,
			}
			if onProgress != nil {
				onProgress(prog)
			}
			results = append(results, ToolUpdateResult{
				Tool:   t,
				Status: StatusSkippedLocal,
			})
			continue
		}

		if dryRun {
			prog := Progress{
				Current: current,
				Total:   total,
				Tool:    t,
				Action:  "Complete",
				Status:  StatusUpdated,
				Success: true,
			}
			if onProgress != nil {
				onProgress(prog)
			}
			results = append(results, ToolUpdateResult{
				Tool:    t,
				Status:  StatusUpdated,
				Success: true,
			})
			continue
		}

		res, diags := installTool(ctx, t, "", InstallRef(t.InstallTarget()), current, total, runner, onProgress)
		results = append(results, res)
		diagnostics = append(diagnostics, diags...)
	}

	return results, time.Since(start), diagnostics
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

	var results []ToolUpdateResult
	var diagnostics []Diagnostic

	total := len(candidates)
	for i, c := range candidates {
		res, diags := installTool(ctx, c.Tool, c.Version, InstallExactRef(c.Tool.InstallTarget(), c.Version), i+1, total, runner, onProgress)
		results = append(results, res)
		diagnostics = append(diagnostics, diags...)
	}

	return results, time.Since(start), diagnostics
}

// installTool executes one installation at the given reference and reports
// the outcome through the shared progress/diagnostics contract. Both the
// legacy floating-@latest path and the exact-version candidate path use it,
// so progress, notes, and failure semantics are identical.
func installTool(ctx context.Context, t Tool, resolvedVersion, ref string, current, total int, runner Runner, onProgress func(Progress)) (ToolUpdateResult, []Diagnostic) {
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
		Args: []string{"install", ref},
		OnLine: func(line string) {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				return
			}
			if strings.HasPrefix(trimmed, "go: downloading") || strings.HasPrefix(trimmed, "go: extracting") {
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
	status := StatusUpdated
	if !success {
		status = StatusFailed
	}

	var notes []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "go: downloading") || strings.HasPrefix(line, "go: extracting") {
			continue
		}
		notes = append(notes, line)

		lower := strings.ToLower(line)
		if strings.Contains(lower, "deprecated") || strings.Contains(lower, "deprecation") {
			diagnostics = append(diagnostics, Diagnostic{
				ToolName: t.Name(),
				Category: "Deprecation",
				Message:  line,
			})
		} else if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") {
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
		Status:  status,
		Success: success,
		Notes:   notes,
		Error:   err,
	}
	if onProgress != nil {
		onProgress(prog)
	}

	return ToolUpdateResult{
		Tool:    t,
		Status:  status,
		Success: success,
		Notes:   notes,
		Error:   err,
	}, diagnostics
}
