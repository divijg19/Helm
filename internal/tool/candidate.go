package tool

import (
	"context"
	"errors"
)

type candidate struct {
	name string
	path string
}

// ErrUnresolvedVersion marks the defensive case where outdated resolution
// reports a tool as outdated but produced no usable version. It can only
// arise if CheckOutdated's contract changes; treating it as a resolution
// failure keeps the update candidate predicate total without authorizing
// an install the resolver never evaluated.
var ErrUnresolvedVersion = errors.New("outdated check produced no resolvable version")

// UpdateCandidate is a tool authorized for installation at one exact,
// already-resolved upstream version. Candidates are the only values the
// update phase may install.
type UpdateCandidate struct {
	Tool    Tool
	Version string
}

// CandidateSet partitions selected, updatable tools by fresh outdated
// resolution. Failed carries resolution errors, which veto installation;
// those tools must remain visible but must never be installed.
type CandidateSet struct {
	Candidates []UpdateCandidate
	UpToDate   []Tool
	Failed     []OutdatedResult
}

// isUpdateCandidate is the explicit domain predicate authorizing mutation:
// selected and updatable (established by the caller), successfully resolved,
// proven outdated, and carrying a usable exact version.
func isUpdateCandidate(r OutdatedResult) bool {
	return r.Error == nil && r.Outdated && r.Latest != ""
}

// ResolveUpdateCandidates runs the existing CheckOutdated implementation over
// the selected updatable tools and partitions the fresh results. Selection
// uses the existing exact-name semantics; unselected tools are never
// resolved, and resolution errors veto installation while leaving unrelated
// candidates unaffected.
func ResolveUpdateCandidates(ctx context.Context, tools []Tool, filter []string, runner Runner) CandidateSet {
	set := nameSet(filter)

	eligible := make([]Tool, 0, len(tools))
	for _, t := range tools {
		if !selected(t.Name(), set) {
			continue
		}
		if !t.CanUpdate() {
			continue
		}
		eligible = append(eligible, t)
	}

	var out CandidateSet
	for _, r := range CheckOutdated(ctx, eligible, runner) {
		switch {
		case r.Error != nil:
			out.Failed = append(out.Failed, r)
		case isUpdateCandidate(r):
			out.Candidates = append(out.Candidates, UpdateCandidate{Tool: r.Tool, Version: r.Latest})
		case r.Outdated:
			out.Failed = append(out.Failed, OutdatedResult{
				Tool:    r.Tool,
				Current: r.Current,
				Latest:  r.Latest,
				Error:   ErrUnresolvedVersion,
			})
		default:
			out.UpToDate = append(out.UpToDate, r.Tool)
		}
	}
	return out
}

// InstallExactRef returns the module@version reference installing exactly the
// given resolved version. The update phase must use this with the version
// produced by outdated resolution; InstallRef (floating @latest) remains the
// rule displayed by --check/--dry-run for eligible tools.
func InstallExactRef(target, version string) string {
	return target + "@" + version
}
