package tool

// PlanResult is the immutable outcome of planning an update. It is produced by
// the domain layer so planning has exactly one owner; renderers and the app
// layer never re-derive which tools would update.
type PlanResult struct {
	ToUpdate []Tool
	Skipped  []Tool
	Invalid  []InvalidBinary
}

// Plan computes which tools would be updated for the given filter. An empty
// filter selects every tool; otherwise only tools whose name matches are
// considered. Local/devel tools and invalid binaries are reported separately
// so renderers never count them as update candidates. Invalid binaries are
// always reported (they cannot be matched by name).
func Plan(loadRes LoadResult, filter []string) PlanResult {
	result := PlanResult{}
	result.ToUpdate, result.Skipped = partitionSelection(loadRes.Tools, nameSet(filter))
	result.Invalid = append(result.Invalid, loadRes.Invalid...)
	return result
}

// partitionSelection splits the selected tools into the ones that can be
// updated and the ones that were selected but are ineligible (local/devel
// builds), preserving input order. An empty name set selects every tool.
//
// Plan and ResolveUpdateCandidates must agree on which tools are eligible:
// they share this function so `helm --check` and an actual update can never
// disagree about what would change.
func partitionSelection(tools []Tool, set map[string]bool) (eligible, skipped []Tool) {
	for _, t := range tools {
		if !selected(t.Name(), set) {
			continue
		}
		if t.CanUpdate() {
			eligible = append(eligible, t)
		} else {
			skipped = append(skipped, t)
		}
	}
	return eligible, skipped
}

// InstallRef returns the floating `@latest` reference shown by --check and
// --dry-run for eligible tools. It is display-only: executed installs pin
// the resolved version via InstallExactRef, never this reference.
func InstallRef(target string) string {
	return target + "@latest"
}

// InstallCommand returns the human-readable `go install` command for a tool.
// The plan renderer uses it; it always agrees with what InstallRef would run.
func InstallCommand(target string) string {
	return "go install " + InstallRef(target)
}

// nameSet builds a lookup set of selected tool names. An empty set means every
// tool is selected.
func nameSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// selected reports whether a tool name passes the given filter set.
func selected(name string, set map[string]bool) bool {
	return len(set) == 0 || set[name]
}

// UnknownFilterNames returns the filter names that match no known tool, in
// first-seen order without duplicates. An empty filter selects everything and
// yields no unknown names. Callers use this to reject misspelled or foreign
// invocations (for example, another program's shell completion calling Helm
// with unexpected words) instead of running a silently empty operation.
func UnknownFilterNames(tools []Tool, filter []string) []string {
	known := make(map[string]bool, len(tools))
	for _, t := range tools {
		known[t.Name()] = true
	}
	var unknown []string
	seen := make(map[string]bool, len(filter))
	for _, n := range filter {
		if known[n] || seen[n] {
			continue
		}
		seen[n] = true
		unknown = append(unknown, n)
	}
	return unknown
}
