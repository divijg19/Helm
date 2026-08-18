package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/mod/semver"
)

type OutdatedResult struct {
	Tool     Tool
	Current  string
	Latest   string
	Outdated bool
	Error    error
}

type goListModule struct {
	Path      string   `json:"Path"`
	Version   string   `json:"Version"`
	Retracted []string `json:"Retracted"`
}

var pseudoVersionRe = regexp.MustCompile(`-\d{14}-[0-9a-f]{12}$`)

func isPseudoVersion(v string) bool {
	return pseudoVersionRe.MatchString(v)
}

func pseudoVersionBase(v string) string {
	idx := strings.Index(v, "-20")
	if idx != -1 {
		base := v[:idx]
		base = strings.TrimSuffix(base, "-0")
		return base
	}
	return v
}

func isRetracted(version string, retracted []string) bool {
	for _, r := range retracted {
		if r == version {
			return true
		}
		if strings.HasPrefix(r, "[") && strings.HasSuffix(r, "]") {
			parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(r, "["), "]"), ",")
			if len(parts) == 2 {
				low := strings.TrimSpace(parts[0])
				high := strings.TrimSpace(parts[1])
				if semver.IsValid(low) && semver.IsValid(high) && semver.IsValid(version) {
					if semver.Compare(version, low) >= 0 && semver.Compare(version, high) <= 0 {
						return true
					}
				}
			}
		}
	}
	return false
}

// defaultOutdatedConcurrency is the bounded worker count used by CheckOutdated.
//
// Controlled measurements (see .opencode/) showed that unbounded concurrency
// increases shared module-cache pressure with diminishing returns, while a
// small bound around 4 materially reduced wall time for realistic tool
// counts. This is an internal execution default, not a user-facing contract.
const defaultOutdatedConcurrency = 4

// CheckOutdated reports which installed, updatable tools have newer upstream
// releases. Independent checks execute with bounded concurrency; the returned
// slice preserves the original discovered tool ordering of the input.
//
// Tools that cannot be updated (local/devel) produce no result, exactly as in
// the sequential contract. Per-tool failures remain attached to their tool.
func CheckOutdated(ctx context.Context, tools []Tool, runner Runner) []OutdatedResult {
	return checkOutdatedConcurrency(ctx, tools, runner, defaultOutdatedConcurrency)
}

// checkOutdatedConcurrency is the internal execution primitive behind
// CheckOutdated. It is separated so tests can exercise a specific worker count
// (including 1, the sequential baseline) without changing the public API.
func checkOutdatedConcurrency(ctx context.Context, tools []Tool, runner Runner, concurrency int) []OutdatedResult {
	if runner == nil {
		runner = DefaultRunner{}
	}

	// Record the positions of updatable tools, preserving input order. Results
	// are written back into these slots so the returned order is identical to
	// the sequential contract regardless of worker completion order.
	work := make([]int, 0, len(tools))
	for i, t := range tools {
		if t.CanUpdate() {
			work = append(work, i)
		}
	}

	results := make([]OutdatedResult, len(work))
	if len(work) == 0 {
		return results
	}

	// If the context is already cancelled, do not launch workers. Each updatable
	// tool receives the cancellation error, preserving the per-tool contract and
	// avoiding unnecessary goroutine/process spawning.
	if ctx.Err() != nil {
		for i, toolIdx := range work {
			t := tools[toolIdx]
			results[i] = OutdatedResult{
				Tool:    t,
				Current: t.Version(),
				Error:   ctx.Err(),
			}
		}
		return results
	}

	n := len(work)
	if concurrency > n {
		concurrency = n
	}
	if concurrency < 1 {
		concurrency = 1
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for workIdx := range jobs {
				toolIdx := work[workIdx]
				results[workIdx] = checkToolOutdated(ctx, tools[toolIdx], runner)
			}
		}()
	}

	for workIdx := range work {
		jobs <- workIdx
	}
	close(jobs)
	wg.Wait()

	return results
}

// checkToolOutdated performs the outdated check for a single tool. It contains
// the exact sequential logic formerly inline in CheckOutdated so that concurrent
// and sequential execution share identical behavior.
func checkToolOutdated(ctx context.Context, t Tool, runner Runner) OutdatedResult {
	modPath := t.ModulePath()
	if modPath == "" {
		modPath = t.PackagePath()
	}

	output, err := runner.Run(ctx, Command{
		Name: "go",
		Args: []string{"list", "-m", "-json", modPath + "@latest"},
	})
	if err != nil {
		return OutdatedResult{
			Tool:    t,
			Current: t.Version(),
			Error:   err,
		}
	}

	var mod goListModule
	decoder := json.NewDecoder(strings.NewReader(output))
	if err := decoder.Decode(&mod); err != nil {
		return OutdatedResult{
			Tool:    t,
			Current: t.Version(),
			Error:   fmt.Errorf("failed to parse go list output: %w", err),
		}
	}

	latest := mod.Version
	current := t.Version()

	if latest == "" {
		return OutdatedResult{
			Tool:    t,
			Current: current,
			Error:   fmt.Errorf("unable to resolve latest version"),
		}
	}

	if isRetracted(latest, mod.Retracted) {
		return OutdatedResult{
			Tool:    t,
			Current: current,
			Latest:  latest,
			Error:   fmt.Errorf("latest version %s is retracted", latest),
		}
	}

	// Normalize versions for semver comparison (ensure leading 'v')
	normCurrent := current
	if isPseudoVersion(normCurrent) {
		normCurrent = pseudoVersionBase(normCurrent)
	}
	if !strings.HasPrefix(normCurrent, "v") && semver.IsValid("v"+normCurrent) {
		normCurrent = "v" + normCurrent
	}
	normLatest := latest
	if !strings.HasPrefix(normLatest, "v") && semver.IsValid("v"+normLatest) {
		normLatest = "v" + normLatest
	}

	outdated := false
	if semver.IsValid(normCurrent) && semver.IsValid(normLatest) {
		outdated = semver.Compare(normLatest, normCurrent) > 0
	} else {
		outdated = current != latest && latest != ""
	}

	return OutdatedResult{
		Tool:     t,
		Current:  current,
		Latest:   latest,
		Outdated: outdated,
	}
}
