package tool

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// moduleRunner answers `go list -m -json <module>@latest` from a per-module
// version table and records every invocation, so tests can prove exactly how
// many resolutions and installations occurred and with which references.
// Install calls succeed unless installErr is set.
type moduleRunner struct {
	versions   map[string]string
	listErr    map[string]error
	installErr error

	mu          sync.Mutex
	listMods    []string
	installRefs []string
}

func (r *moduleRunner) Run(ctx context.Context, c Command) (string, error) {
	if len(c.Args) == 0 {
		return "", errTestNoArgs
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch c.Args[0] {
	case "list":
		mod := strings.TrimSuffix(c.Args[len(c.Args)-1], "@latest")
		r.listMods = append(r.listMods, mod)
		if err, ok := r.listErr[mod]; ok {
			return "", err
		}
		return `{"Path":"` + mod + `","Version":"` + r.versions[mod] + `"}`, nil
	case "install":
		r.installRefs = append(r.installRefs, c.Args[len(c.Args)-1])
		return "", r.installErr
	default:
		return "", errTestNoArgs
	}
}

func (r *moduleRunner) listCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.listMods)
}

func (r *moduleRunner) installCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.installRefs)
}

var errTestNoArgs = errTestSentinel()

func errTestSentinel() error {
	return errorString("test: no command args")
}

type errorString string

func (e errorString) Error() string { return string(e) }

func candidateTools() []Tool {
	return []Tool{
		fixtureTool("hello", "example.com/hello", "v1.0.0"),
		fixtureTool("world", "example.com/world", "v1.2.0"),
	}
}

func candidateVersions() map[string]string {
	return map[string]string{
		"example.com/hello": "v1.0.0",
		"example.com/world": "v1.3.0",
	}
}

func candidateNames(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name())
	}
	return names
}

func equalNames(got []Tool, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i, t := range got {
		if t.Name() != want[i] {
			return false
		}
	}
	return true
}

// TestResolveCandidates_OnlyOutdatedBecomeCandidates is the central
// regression test: a current tool must never become an installation
// candidate, and the outdated tool must carry its exact resolved version.
func TestResolveCandidates_OnlyOutdatedBecomeCandidates(t *testing.T) {
	runner := &moduleRunner{versions: candidateVersions()}
	set := ResolveUpdateCandidates(context.Background(), candidateTools(), nil, runner)

	if len(set.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(set.Candidates))
	}
	c := set.Candidates[0]
	if c.Tool.Name() != "world" {
		t.Errorf("expected candidate world, got %s", c.Tool.Name())
	}
	if c.Version != "v1.3.0" {
		t.Errorf("expected resolved version v1.3.0, got %s", c.Version)
	}
	if !equalNames(set.UpToDate, "hello") {
		t.Errorf("expected up-to-date [hello], got %v", candidateNames(set.UpToDate))
	}
	if len(set.Failed) != 0 {
		t.Errorf("expected no failures, got %d", len(set.Failed))
	}

	// Installing the candidate set must touch only world, at the exact
	// resolved reference — hello receives zero install attempts.
	results, _, _ := UpdateCandidates(context.Background(), set.Candidates, runner, nil)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected 1 successful install, got %+v", results)
	}
	if got := runner.installCalls(); got != 1 {
		t.Fatalf("expected exactly 1 install call, got %d", got)
	}
	runner.mu.Lock()
	ref := runner.installRefs[0]
	runner.mu.Unlock()
	if want := "example.com/world/cmd/world@v1.3.0"; ref != want {
		t.Errorf("install ref = %q, want %q", ref, want)
	}
	if strings.Contains(ref, "@latest") {
		t.Errorf("install ref %q must not contain @latest", ref)
	}
}

// TestResolveCandidates_CurrentToolReceivesZeroInstalls proves Invariant A at
// the unit level: with every tool already current, resolution runs but no
// installation is ever attempted.
func TestResolveCandidates_CurrentToolReceivesZeroInstalls(t *testing.T) {
	runner := &moduleRunner{versions: map[string]string{
		"example.com/hello": "v1.0.0",
	}}
	tools := []Tool{fixtureTool("hello", "example.com/hello", "v1.0.0")}

	set := ResolveUpdateCandidates(context.Background(), tools, nil, runner)
	if len(set.Candidates) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(set.Candidates))
	}
	if !equalNames(set.UpToDate, "hello") {
		t.Errorf("expected up-to-date [hello], got %v", candidateNames(set.UpToDate))
	}

	results, _, _ := UpdateCandidates(context.Background(), set.Candidates, runner, nil)
	if len(results) != 0 {
		t.Fatalf("expected 0 install results, got %d", len(results))
	}
	if got := runner.installCalls(); got != 0 {
		t.Errorf("expected 0 install calls, got %d", got)
	}
}

// TestResolveCandidates_ResolutionFailureVetoes proves Invariant C: a tool
// whose outdated state cannot be determined is excluded while unrelated
// valid candidates still proceed.
func TestResolveCandidates_ResolutionFailureVetoes(t *testing.T) {
	runner := &moduleRunner{
		versions: map[string]string{
			"example.com/foo": "v9.9.9",
			"example.com/baz": "v9.9.9",
		},
		listErr: map[string]error{
			"example.com/bar": errorString("simulated network error"),
		},
	}
	tools := []Tool{
		fixtureTool("foo", "example.com/foo", "v1.0.0"),
		fixtureTool("bar", "example.com/bar", "v1.0.0"),
		fixtureTool("baz", "example.com/baz", "v1.0.0"),
	}

	set := ResolveUpdateCandidates(context.Background(), tools, nil, runner)
	if len(set.Candidates) != 2 {
		t.Fatalf("expected 2 candidates (foo, baz), got %d", len(set.Candidates))
	}
	if len(set.Failed) != 1 || set.Failed[0].Tool.Name() != "bar" {
		t.Fatalf("expected failed [bar], got %v", set.Failed)
	}

	results, _, _ := UpdateCandidates(context.Background(), set.Candidates, runner, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 install results, got %d", len(results))
	}
	runner.mu.Lock()
	refs := append([]string(nil), runner.installRefs...)
	runner.mu.Unlock()
	for _, ref := range refs {
		if strings.Contains(ref, "example.com/bar") {
			t.Errorf("failed tool bar must never be installed, got ref %q", ref)
		}
	}
}

// TestResolveCandidates_EmptyLatestVetoes covers the degenerate resolver
// output: a tool whose latest version cannot be resolved authorizes nothing
// and is reported as failed. The failure here originates in
// checkToolOutdated, so it carries that error rather than the defensive
// ErrUnresolvedVersion; that branch is pinned directly by
// TestPartitionResult_UnresolvedVersionVetoes.
func TestResolveCandidates_EmptyLatestVetoes(t *testing.T) {
	runner := &moduleRunner{versions: map[string]string{"example.com/foo": ""}}
	tools := []Tool{fixtureTool("foo", "example.com/foo", "v1.0.0")}

	set := ResolveUpdateCandidates(context.Background(), tools, nil, runner)
	if len(set.Candidates) != 0 {
		t.Fatalf("expected 0 candidates for empty latest, got %d", len(set.Candidates))
	}
	if len(set.Failed) != 1 {
		t.Fatalf("expected 1 failure for empty latest, got %d", len(set.Failed))
	}
}

// TestPartitionResult_UnresolvedVersionVetoes pins the defensive
// ErrUnresolvedVersion branch directly. It is unreachable through
// CheckOutdated today (an unresolvable latest already fails there), so it can
// only be exercised by handing partitionResult a result that claims Outdated
// with an empty Latest. If CheckOutdated's contract ever changes, this test
// still proves an outdated-but-unversioned tool can never become a candidate.
func TestPartitionResult_UnresolvedVersionVetoes(t *testing.T) {
	tool := fixtureTool("foo", "example.com/foo", "v1.0.0")
	var set CandidateSet

	partitionResult(&set, OutdatedResult{Tool: tool, Current: "v1.0.0", Outdated: true, Latest: ""})

	if len(set.Candidates) != 0 {
		t.Fatalf("outdated without a version must not authorize an install, got %d candidates", len(set.Candidates))
	}
	if len(set.Failed) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(set.Failed))
	}
	if !errors.Is(set.Failed[0].Error, ErrUnresolvedVersion) {
		t.Errorf("failure error = %v, want ErrUnresolvedVersion", set.Failed[0].Error)
	}
	if set.Failed[0].Tool.Name() != "foo" || set.Failed[0].Current != "v1.0.0" {
		t.Errorf("failure must retain tool provenance, got %+v", set.Failed[0])
	}
	if len(set.UpToDate) != 0 {
		t.Errorf("an unresolved outdated tool must not be reported up-to-date, got %d", len(set.UpToDate))
	}
}

// TestPartitionResult_Buckets is a table over the partition contract: every
// result lands in exactly one bucket, and only a resolved outdated result is
// ever installable.
func TestPartitionResult_Buckets(t *testing.T) {
	tool := fixtureTool("foo", "example.com/foo", "v1.0.0")
	tests := []struct {
		name       string
		result     OutdatedResult
		wantBucket string
	}{
		{"resolved outdated is installable", OutdatedResult{Tool: tool, Outdated: true, Latest: "v1.2.0"}, "candidates"},
		{"current is up to date", OutdatedResult{Tool: tool, Current: "v1.0.0", Latest: "v1.0.0"}, "uptodate"},
		{"resolution error fails", OutdatedResult{Tool: tool, Error: errors.New("boom")}, "failed"},
		{"outdated without version fails", OutdatedResult{Tool: tool, Outdated: true, Latest: ""}, "failed"},
		{"error outranks outdated", OutdatedResult{Tool: tool, Outdated: true, Latest: "v1.2.0", Error: errors.New("boom")}, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var set CandidateSet
			partitionResult(&set, tt.result)
			got := map[string]int{
				"candidates": len(set.Candidates),
				"uptodate":   len(set.UpToDate),
				"failed":     len(set.Failed),
			}
			if got[tt.wantBucket] != 1 {
				t.Errorf("result landed as %v, want exactly 1 in %q", got, tt.wantBucket)
			}
			for bucket, n := range got {
				if bucket != tt.wantBucket && n != 0 {
					t.Errorf("result also landed in %q (%d entries); buckets must be exclusive", bucket, n)
				}
			}
		})
	}
}

// TestResolveCandidates_SelectionSafety proves unselected tools are neither
// resolved nor installed, even when outdated.
func TestResolveCandidates_SelectionSafety(t *testing.T) {
	runner := &moduleRunner{versions: candidateVersions()}

	set := ResolveUpdateCandidates(context.Background(), candidateTools(), []string{"world"}, runner)
	if len(set.Candidates) != 1 || set.Candidates[0].Tool.Name() != "world" {
		t.Fatalf("expected candidate [world], got %+v", set.Candidates)
	}
	if len(set.UpToDate) != 0 || len(set.Failed) != 0 {
		t.Fatalf("expected no other buckets, got up-to-date=%v failed=%v", set.UpToDate, set.Failed)
	}

	runner.mu.Lock()
	lists := append([]string(nil), runner.listMods...)
	runner.mu.Unlock()
	if len(lists) != 1 || lists[0] != "example.com/world" {
		t.Errorf("expected resolution of world only, got %v", lists)
	}
}

// TestResolveCandidates_SingleResolutionPerTool proves Invariant E: the
// update path performs exactly one outdated resolution per selected updatable
// tool and never re-resolves during installation.
func TestResolveCandidates_SingleResolutionPerTool(t *testing.T) {
	runner := &moduleRunner{versions: candidateVersions()}

	set := ResolveUpdateCandidates(context.Background(), candidateTools(), nil, runner)
	if got := runner.listCalls(); got != 2 {
		t.Fatalf("expected exactly 2 list calls (one per tool), got %d", got)
	}

	_, _, _ = UpdateCandidates(context.Background(), set.Candidates, runner, nil)
	if got := runner.listCalls(); got != 2 {
		t.Errorf("installation must not re-resolve: list calls went 2 -> %d", got)
	}
	if got := runner.installCalls(); got != 1 {
		t.Errorf("expected exactly 1 install call, got %d", got)
	}
}

// TestResolveCandidates_PreCancelledContextVetoes proves Invariant H:
// cancellation yields exclusion, never authorization, and no installs.
func TestResolveCandidates_PreCancelledContextVetoes(t *testing.T) {
	runner := &moduleRunner{versions: candidateVersions()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	set := ResolveUpdateCandidates(ctx, candidateTools(), nil, runner)
	if len(set.Candidates) != 0 {
		t.Fatalf("expected 0 candidates under cancellation, got %d", len(set.Candidates))
	}
	if len(set.Failed) != 2 {
		t.Fatalf("expected 2 failures under cancellation, got %d", len(set.Failed))
	}
	if got := runner.installCalls(); got != 0 {
		t.Errorf("expected 0 install calls under cancellation, got %d", got)
	}
}

// TestUpdateCandidates_ExactVersionNoFallback proves Invariants D and G:
// the install command carries the resolved version (never @latest), and an
// install failure is reported as Failed with no second attempt.
func TestUpdateCandidates_ExactVersionNoFallback(t *testing.T) {
	runner := &moduleRunner{installErr: errorString("simulated install failure")}
	candidates := []UpdateCandidate{
		{Tool: fixtureTool("foo", "example.com/foo", "v1.0.0"), Version: "v1.4.2"},
	}

	results, _, _ := UpdateCandidates(context.Background(), candidates, runner, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Errorf("expected install failure, got success")
	}
	if results[0].Error == nil {
		t.Error("a failed install must carry the error, since the non-terminal renderers report it as the failure reason")
	}
	if got := runner.installCalls(); got != 1 {
		t.Fatalf("expected exactly 1 install attempt (no fallback), got %d", got)
	}
	runner.mu.Lock()
	ref := runner.installRefs[0]
	runner.mu.Unlock()
	if want := "example.com/foo/cmd/foo@v1.4.2"; ref != want {
		t.Errorf("install ref = %q, want %q", ref, want)
	}
}

// TestResolveCandidates_SkippedLocals pins Skipped semantics shared with the
// plan operation: selected tools that cannot update land in Skipped while
// updatable tools partition into candidates, up-to-date, or failures
// unaffected. Unselected tools appear in no bucket.
func TestResolveCandidates_SkippedLocals(t *testing.T) {
	runner := &moduleRunner{versions: candidateVersions()}
	tools := append(candidateTools(), fixtureTool("localdev", "example.com/localdev", "(devel)"))

	set := ResolveUpdateCandidates(context.Background(), tools, nil, runner)
	if !equalNames(set.Skipped, "localdev") {
		t.Errorf("expected skipped [localdev], got %v", candidateNames(set.Skipped))
	}
	var candidateNamesGot []string
	for _, c := range set.Candidates {
		candidateNamesGot = append(candidateNamesGot, c.Tool.Name())
	}
	if len(set.Candidates) != 1 || len(set.UpToDate) != 1 || len(set.Failed) != 0 {
		t.Errorf("other buckets disturbed: candidates=%v up-to-date=%v failed=%v",
			candidateNamesGot, candidateNames(set.UpToDate), set.Failed)
	}

	filtered := ResolveUpdateCandidates(context.Background(), tools, []string{"world"}, runner)
	if len(filtered.Skipped) != 0 {
		t.Errorf("unselected local must appear in no bucket, got skipped=%v", candidateNames(filtered.Skipped))
	}
}

// TestInstallExactRef was removed: InstallExactRef is asserted by
// TestInstallDisplayVsExecutionReferences in plan_test.go, which pins the
// exact reference and the distinction from the floating @latest display rule
// in one place.

// TestResolveCandidates_SameModuleSharesResolvedVersion pins the invariant
// behind installation-tree module headers: tools from one module resolve
// through the same module path, so every candidate from that module carries
// the identical resolved version and the group header is input-order
// independent.
func TestResolveCandidates_SameModuleSharesResolvedVersion(t *testing.T) {
	runner := &moduleRunner{versions: map[string]string{"example.com/suite": "v1.5.0"}}
	tools := []Tool{
		fixtureTool("toolA", "example.com/suite", "v1.0.0"),
		fixtureTool("toolB", "example.com/suite", "v1.0.0"),
	}

	set := ResolveUpdateCandidates(context.Background(), tools, nil, runner)
	if len(set.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(set.Candidates))
	}
	for _, c := range set.Candidates {
		if c.Version != "v1.5.0" {
			t.Errorf("candidate %s resolved to %q, want v1.5.0 (single module, single latest)", c.Tool.Name(), c.Version)
		}
	}
}
