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
// those tools must remain visible but must never be installed. Skipped
// carries selected tools that cannot be updated (local/development
// builds); they are reported as skipped, mirroring the plan operation.
type CandidateSet struct {
	Candidates []UpdateCandidate
	UpToDate   []Tool
	Failed     []OutdatedResult
	Skipped    []Tool
}

// isUpdateCandidate is the explicit domain predicate authorizing mutation:
// selected and updatable (established by the caller), successfully resolved,
// proven outdated, and carrying a usable exact version.
func isUpdateCandidate(r OutdatedResult) bool {
	return r.Error == nil && r.Outdated && r.Latest != ""
}

// partitionResult files one outdated result into exactly one bucket of set.
// Only isUpdateCandidate results may become candidates; a result that claims
// to be outdated but carries no usable version is recorded as an
// ErrUnresolvedVersion failure so it stays visible without ever authorizing an
// install at an unknown version.
func partitionResult(set *CandidateSet, r OutdatedResult) {
	switch {
	case r.Error != nil:
		set.Failed = append(set.Failed, r)
	case isUpdateCandidate(r):
		set.Candidates = append(set.Candidates, UpdateCandidate{Tool: r.Tool, Version: r.Latest})
	case r.Outdated:
		set.Failed = append(set.Failed, OutdatedResult{
			Tool:    r.Tool,
			Current: r.Current,
			Latest:  r.Latest,
			Error:   ErrUnresolvedVersion,
		})
	default:
		set.UpToDate = append(set.UpToDate, r.Tool)
	}
}

// ResolveUpdateCandidates runs the existing CheckOutdated implementation over
// the selected updatable tools and partitions the fresh results. Selection
// uses the existing exact-name semantics; unselected tools are never
// resolved, and resolution errors veto installation while leaving unrelated
// candidates unaffected.
func ResolveUpdateCandidates(ctx context.Context, tools []Tool, filter []string, runner Runner) CandidateSet {
	set := nameSet(filter)

	var out CandidateSet
	// Same selection and eligibility rules as Plan, so the plan a user sees
	// and the update that then runs always cover the same tools.
	eligible, skipped := partitionSelection(tools, set)
	out.Skipped = skipped

	for _, r := range CheckOutdated(ctx, eligible, runner) {
		partitionResult(&out, r)
	}
	return out
}

// InstallExactRef returns the package@version reference installing exactly the
// given resolved version. target is a main package path (Tool.InstallTarget),
// not a module path: `go install` takes a package, and ModulePath is used
// separately for `go list -m` queries. The update phase must use this with the
// version produced by outdated resolution; InstallRef (floating @latest)
// remains the rule displayed by --check/--dry-run for eligible tools.
func InstallExactRef(target, version string) string {
	return target + "@" + version
}
