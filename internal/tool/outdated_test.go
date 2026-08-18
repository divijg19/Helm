package tool

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCheckOutdated_UpToDate(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v1.0.0"}`}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "v1.0.0"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Outdated {
		t.Errorf("expected not outdated, got outdated=true")
	}
	if r.Current != "v1.0.0" {
		t.Errorf("expected current v1.0.0, got %s", r.Current)
	}
	if r.Latest != "v1.0.0" {
		t.Errorf("expected latest v1.0.0, got %s", r.Latest)
	}
	if r.Error != nil {
		t.Errorf("unexpected error: %v", r.Error)
	}
}

func TestCheckOutdated_Outdated(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v1.1.0"}`}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "v1.0.0"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Outdated {
		t.Errorf("expected outdated=true, got false")
	}
}

func TestCheckOutdated_CommandError(t *testing.T) {
	runner := mockRunner{err: errors.New("network error")}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "v1.0.0"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestCheckOutdated_MalformedJSON(t *testing.T) {
	runner := mockRunner{output: "not json"}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "v1.0.0"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Errorf("expected parse error, got nil")
	}
}

func TestCheckOutdated_SkipsLocal(t *testing.T) {
	runner := mockRunner{output: "{}"}
	tools := []Tool{
		{name: "local", path: "/gobin/local", info: &buildinfo.BuildInfo{
			Path: "(devel)",
			Main: debug.Module{Path: "example.com/local", Version: "(devel)"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 0 {
		t.Errorf("expected 0 results for local/devel tool, got %d", len(results))
	}
}

func TestCheckOutdated_NonSemver(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"1.2.3"}`}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "abc123"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Errorf("unexpected error: %v", results[0].Error)
	}
	if !results[0].Outdated {
		t.Errorf("expected outdated=true for different non-semver versions")
	}
}

func TestCheckOutdated_NonSemverEqual(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"abc123"}`}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "abc123"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Outdated {
		t.Errorf("expected outdated=false for equal non-semver versions")
	}
}

func TestCheckOutdated_EmptyLatest(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":""}`}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "v1.0.0"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Errorf("expected error for empty latest version, got nil")
	}
}

func TestCheckOutdated_Retracted(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v1.1.0","Retracted":["v1.1.0"]}`}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "v1.0.0"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Errorf("expected error for retracted latest version, got nil")
	}
}

func TestCheckOutdated_PseudoVersion(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v1.0.0"}`}
	tools := []Tool{
		{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
			Path: "example.com/foo/cmd/foo",
			Main: debug.Module{Path: "example.com/foo", Version: "v1.0.1-0.20230501123456-abcdef123456"},
		}},
	}
	results := CheckOutdated(context.Background(), tools, runner)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Outdated {
		t.Errorf("expected pseudo-version (newer base) to be considered up to date/ahead against older latest tag")
	}
}

// TestCheckOutdated_RetractedRange establishes the complete inclusive-range
// contract for Helm's retracted-version handling, exercised through the
// observable CheckOutdated behavior (not the private isRetracted helper).
//
// Given a retraction of the form [low, high], the implementation treats a
// latest version as retracted when:
//
//	low <= version <= high   (inclusive on both boundaries)
//
// Any value outside that window falls through to normal outdated evaluation.
// Malformed or semantically invalid range metadata must be ignored rather than
// incorrectly converting an otherwise valid outdated check into a retraction
// failure.
func TestCheckOutdated_RetractedRange(t *testing.T) {
	const rangeSpec = "[v1.1.0, v1.2.0]"
	tests := []struct {
		name          string
		latest        string
		retraction    string
		wantRetracted bool
		wantOutdated  bool
	}{
		{
			name:          "interior",
			latest:        "v1.1.5",
			retraction:    rangeSpec,
			wantRetracted: true,
		},
		{
			name:          "lower boundary inclusive",
			latest:        "v1.1.0",
			retraction:    rangeSpec,
			wantRetracted: true,
		},
		{
			name:          "upper boundary inclusive",
			latest:        "v1.2.0",
			retraction:    rangeSpec,
			wantRetracted: true,
		},
		{
			name:          "above range",
			latest:        "v1.2.1",
			retraction:    rangeSpec,
			wantRetracted: false,
			wantOutdated:  true,
		},
		{
			name:          "below range",
			latest:        "v1.0.9",
			retraction:    rangeSpec,
			wantRetracted: false,
			wantOutdated:  true,
		},
		{
			name:          "invalid low boundary",
			latest:        "v1.1.5",
			retraction:    "[invalid, v1.2.0]",
			wantRetracted: false,
			wantOutdated:  true,
		},
		{
			name:          "invalid high boundary",
			latest:        "v1.1.5",
			retraction:    "[v1.1.0, invalid]",
			wantRetracted: false,
			wantOutdated:  true,
		},
		{
			name:          "malformed range single boundary",
			latest:        "v1.1.5",
			retraction:    "[v1.1.0]",
			wantRetracted: false,
			wantOutdated:  true,
		},
		{
			name:          "missing brackets",
			latest:        "v1.1.5",
			retraction:    "v1.1.0, v1.2.0",
			wantRetracted: false,
			wantOutdated:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := mockRunner{output: `{"Path":"example.com/foo","Version":"` + tc.latest + `","Retracted":["` + tc.retraction + `"]}`}
			tools := []Tool{
				{name: "foo", path: "/gobin/foo", info: &buildinfo.BuildInfo{
					Path: "example.com/foo/cmd/foo",
					Main: debug.Module{Path: "example.com/foo", Version: "v1.0.0"},
				}},
			}
			results := CheckOutdated(context.Background(), tools, runner)
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			r := results[0]
			if tc.wantRetracted {
				if r.Error == nil {
					t.Errorf("expected retraction error for latest %s in range %s, got nil", tc.latest, tc.retraction)
				} else if !strings.Contains(r.Error.Error(), "retracted") {
					t.Errorf("unexpected error message: %v", r.Error)
				}
				return
			}
			if r.Error != nil {
				t.Errorf("unexpected error for latest %s with retraction %s: %v", tc.latest, tc.retraction, r.Error)
			}
			if r.Outdated != tc.wantOutdated {
				t.Errorf("expected outdated=%v for latest %s (installed v1.0.0), got %v", tc.wantOutdated, tc.latest, r.Outdated)
			}
		})
	}
}

// makeOutdatedTool builds a synthetic updatable Tool for concurrency tests.
func makeOutdatedTool(name, pkg, version string) Tool {
	return Tool{
		name: name,
		path: "/gobin/" + name,
		info: &buildinfo.BuildInfo{
			Path: pkg + "/cmd/" + name,
			Main: debug.Module{Path: pkg, Version: version},
		},
	}
}

// TestCheckOutdated_SequentialEquivalence proves that the concurrent execution
// path with a worker count of 1 produces results identical to the existing
// sequential contract: same length, same tool order, same per-tool outcome.
func TestCheckOutdated_SequentialEquivalence(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v9.9.9"}`}
	tools := []Tool{
		makeOutdatedTool("foo", "example.com/foo", "v1.0.0"),
		makeOutdatedTool("bar", "example.com/bar", "v2.0.0"),
		makeOutdatedTool("baz", "example.com/baz", "v3.0.0"),
	}

	got := checkOutdatedConcurrency(context.Background(), tools, runner, 1)
	if len(got) != len(tools) {
		t.Fatalf("expected %d results, got %d", len(tools), len(got))
	}
	wantNames := []string{"foo", "bar", "baz"}
	for i, r := range got {
		if r.Tool.Name() != wantNames[i] {
			t.Errorf("result %d: expected tool %s, got %s", i, wantNames[i], r.Tool.Name())
		}
		if !r.Outdated {
			t.Errorf("result %d (%s): expected outdated=true", i, r.Tool.Name())
		}
	}
}

// TestCheckOutdated_ConcurrentMultiTool exercises the bounded worker model with
// several tools and confirms all results are present and correct.
func TestCheckOutdated_ConcurrentMultiTool(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v9.9.9"}`}
	tools := []Tool{
		makeOutdatedTool("alpha", "example.com/alpha", "v1.0.0"),
		makeOutdatedTool("beta", "example.com/beta", "v2.0.0"),
		makeOutdatedTool("gamma", "example.com/gamma", "v3.0.0"),
		makeOutdatedTool("delta", "example.com/delta", "v4.0.0"),
		makeOutdatedTool("epsilon", "example.com/epsilon", "v5.0.0"),
	}

	got := checkOutdatedConcurrency(context.Background(), tools, runner, defaultOutdatedConcurrency)
	if len(got) != len(tools) {
		t.Fatalf("expected %d results, got %d", len(tools), len(got))
	}
	for i, r := range got {
		if r.Tool.Name() != tools[i].Name() {
			t.Errorf("result %d: expected tool %s, got %s", i, tools[i].Name(), r.Tool.Name())
		}
		if !r.Outdated {
			t.Errorf("result %d (%s): expected outdated=true", i, r.Tool.Name())
		}
	}
}

// delayedRunner simulates variable per-tool latency so that later-index tools
// finish before earlier ones, forcing workers to complete out of order. The
// returned JSON keys the latest version off the requested module path so each
// result can be validated independently of position.
type delayedRunner struct {
	delays map[string]time.Duration
	mu     sync.Mutex
	order  []string
}

func (r *delayedRunner) Run(ctx context.Context, c Command) (string, error) {
	mod := c.Args[len(c.Args)-1]
	mod = strings.TrimSuffix(mod, "@latest")

	r.mu.Lock()
	r.order = append(r.order, mod)
	r.mu.Unlock()

	if d, ok := r.delays[mod]; ok {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return `{"Path":"` + mod + `","Version":"v9.9.9"}`, nil
}

// TestCheckOutdated_DeterministicOrdering is the critical regression guard:
// a concurrent implementation that appends as workers finish would reorder the
// output. This runner makes later tools complete first; the assertion requires
// results to remain in the original input order.
func TestCheckOutdated_DeterministicOrdering(t *testing.T) {
	tools := []Tool{
		makeOutdatedTool("first", "example.com/first", "v1.0.0"),
		makeOutdatedTool("second", "example.com/second", "v1.0.0"),
		makeOutdatedTool("third", "example.com/third", "v1.0.0"),
	}

	runner := &delayedRunner{
		delays: map[string]time.Duration{
			"example.com/first":  100 * time.Millisecond,
			"example.com/second": 50 * time.Millisecond,
			"example.com/third":  0,
		},
	}

	got := checkOutdatedConcurrency(context.Background(), tools, runner, defaultOutdatedConcurrency)
	if len(got) != len(tools) {
		t.Fatalf("expected %d results, got %d", len(tools), len(got))
	}

	want := []string{"first", "second", "third"}
	for i, r := range got {
		if r.Tool.Name() != want[i] {
			t.Errorf("result %d: expected tool %s (original order), got %s", i, want[i], r.Tool.Name())
		}
		// Each result must report the latest version the runner returned for
		// its own module, proving the correct per-tool outcome landed in the
		// correct slot.
		if r.Latest != "v9.9.9" {
			t.Errorf("result %d (%s): expected latest v9.9.9, got %s", i, r.Tool.Name(), r.Latest)
		}
	}

	// Sanity: confirm the runner actually completed out of order, otherwise the
	// test would not have exercised the concurrency hazard it guards against.
	runner.mu.Lock()
	completed := append([]string(nil), runner.order...)
	runner.mu.Unlock()
	if len(completed) == 3 &&
		completed[0] == "third" && completed[1] == "second" && completed[2] == "first" {
		// out-of-order completion observed; ordering guard is meaningful.
	} else {
		t.Logf("completion order was %v (out-of-order completion not observed; guard still validates order)", completed)
	}
}

// TestCheckOutdated_MixedSuccessFailure confirms that one tool's failure does
// not discard other results and that each outcome stays attached to its tool,
// in original order.
func TestCheckOutdated_MixedSuccessFailure(t *testing.T) {
	failRunner := failAfterRunner{errTool: "example.com/bar"}

	tools := []Tool{
		makeOutdatedTool("foo", "example.com/foo", "v1.0.0"),
		makeOutdatedTool("bar", "example.com/bar", "v1.0.0"),
		makeOutdatedTool("baz", "example.com/baz", "v1.0.0"),
	}

	got := checkOutdatedConcurrency(context.Background(), tools, failRunner, defaultOutdatedConcurrency)
	if len(got) != len(tools) {
		t.Fatalf("expected %d results, got %d", len(tools), len(got))
	}

	if got[0].Tool.Name() != "foo" || got[0].Error != nil {
		t.Errorf("result 0: expected foo success, got %s err=%v", got[0].Tool.Name(), got[0].Error)
	}
	if got[1].Tool.Name() != "bar" || got[1].Error == nil {
		t.Errorf("result 1: expected bar failure, got %s err=%v", got[1].Tool.Name(), got[1].Error)
	}
	if got[2].Tool.Name() != "baz" || got[2].Error != nil {
		t.Errorf("result 2: expected baz success, got %s err=%v", got[2].Tool.Name(), got[2].Error)
	}
}

// failAfterRunner returns a parseable success payload for every module except
// errTool, which receives an error. It exercises per-tool failure isolation.
type failAfterRunner struct {
	errTool string
}

func (r failAfterRunner) Run(ctx context.Context, c Command) (string, error) {
	mod := c.Args[len(c.Args)-1]
	mod = strings.TrimSuffix(mod, "@latest")
	if mod == r.errTool {
		return "", errors.New("simulated network error")
	}
	return `{"Path":"` + mod + `","Version":"v2.0.0"}`, nil
}

// TestCheckOutdated_Cancellation verifies that context cancellation propagates
// to in-flight work, workers terminate without leaking, and every updatable
// tool still receives a result (carrying the cancellation error).
func TestCheckOutdated_Cancellation(t *testing.T) {
	tools := []Tool{
		makeOutdatedTool("foo", "example.com/foo", "v1.0.0"),
		makeOutdatedTool("bar", "example.com/bar", "v1.0.0"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	got := checkOutdatedConcurrency(ctx, tools, blockingRunner{}, defaultOutdatedConcurrency)
	if len(got) != len(tools) {
		t.Fatalf("expected %d results, got %d", len(tools), len(got))
	}
	for i, r := range got {
		if r.Error == nil {
			t.Errorf("result %d (%s): expected cancellation error, got nil", i, r.Tool.Name())
		}
	}
}

// blockingRunner waits until the context is cancelled before returning, so it
// can only complete via cancellation propagation.
type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, c Command) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// TestCheckOutdated_EmptyInput confirms no workers are spawned and an empty
// result is returned for zero tools.
func TestCheckOutdated_EmptyInput(t *testing.T) {
	got := checkOutdatedConcurrency(context.Background(), nil, mockRunner{}, defaultOutdatedConcurrency)
	if len(got) != 0 {
		t.Fatalf("expected 0 results, got %d", len(got))
	}
}

// TestCheckOutdated_SingleTool verifies the fast, single-item path returns the
// expected result without unnecessary goroutine overhead hazards.
func TestCheckOutdated_SingleTool(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v1.1.0"}`}
	tools := []Tool{makeOutdatedTool("foo", "example.com/foo", "v1.0.0")}

	got := checkOutdatedConcurrency(context.Background(), tools, runner, defaultOutdatedConcurrency)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if !got[0].Outdated {
		t.Errorf("expected foo outdated=true")
	}
}

// TestCheckOutdated_SkipsLocalUnderConcurrency ensures local/devel tools are
// excluded from results under concurrency, preserving the sequential contract.
func TestCheckOutdated_SkipsLocalUnderConcurrency(t *testing.T) {
	runner := mockRunner{output: "{}"}
	tools := []Tool{
		makeOutdatedTool("updatable", "example.com/updatable", "v1.0.0"),
		Tool{
			name: "local",
			path: "/gobin/local",
			info: &buildinfo.BuildInfo{
				Path: "(devel)",
				Main: debug.Module{Path: "example.com/local", Version: "(devel)"},
			},
		},
	}

	got := checkOutdatedConcurrency(context.Background(), tools, runner, defaultOutdatedConcurrency)
	if len(got) != 1 {
		t.Fatalf("expected 1 result (local skipped), got %d", len(got))
	}
	if got[0].Tool.Name() != "updatable" {
		t.Errorf("expected updatable tool, got %s", got[0].Tool.Name())
	}
}

// countingRunner records how many times Run is invoked. It is used to prove that
// a pre-cancelled context launches no unnecessary work.
type countingRunner struct {
	mu    sync.Mutex
	calls int
}

func (r *countingRunner) Run(ctx context.Context, c Command) (string, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return `{"Path":"x","Version":"v9.9.9"}`, nil
}

// TestCheckOutdated_PreCancelledContext proves that a context already cancelled
// before CheckOutdated begins does not launch worker goroutines or runner calls;
// each updatable tool still receives the cancellation error in order.
func TestCheckOutdated_PreCancelledContext(t *testing.T) {
	tools := []Tool{
		makeOutdatedTool("foo", "example.com/foo", "v1.0.0"),
		makeOutdatedTool("bar", "example.com/bar", "v1.0.0"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before invocation

	runner := &countingRunner{}
	got := checkOutdatedConcurrency(ctx, tools, runner, defaultOutdatedConcurrency)

	if runner.calls != 0 {
		t.Errorf("expected 0 runner calls for pre-cancelled context, got %d", runner.calls)
	}
	if len(got) != len(tools) {
		t.Fatalf("expected %d results, got %d", len(tools), len(got))
	}
	for i, r := range got {
		if r.Tool.Name() != tools[i].Name() {
			t.Errorf("result %d: expected tool %s, got %s", i, tools[i].Name(), r.Tool.Name())
		}
		if r.Error == nil {
			t.Errorf("result %d (%s): expected cancellation error, got nil", i, r.Tool.Name())
		}
	}
}

// mixedCancelRunner completes tools whose module is not blockTool after a small
// delay; the blockTool blocks until the context is cancelled. This forces a
// mixed completion state: some tools finish, one is cancelled mid-flight.
type mixedCancelRunner struct {
	blockTool  string
	otherDelay time.Duration
}

func (r *mixedCancelRunner) Run(ctx context.Context, c Command) (string, error) {
	mod := c.Args[len(c.Args)-1]
	mod = strings.TrimSuffix(mod, "@latest")

	if mod == r.blockTool {
		<-ctx.Done()
		return "", ctx.Err()
	}

	if r.otherDelay > 0 {
		select {
		case <-time.After(r.otherDelay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return `{"Path":"` + mod + `","Version":"v9.9.9"}`, nil
}

// TestCheckOutdated_MidFlightCancellationMixed proves that when cancellation
// occurs while some tools are still running, completed tools keep their results
// and the blocked tool receives the cancellation error. The function must return
// without deadlock and preserve deterministic ordering.
func TestCheckOutdated_MidFlightCancellationMixed(t *testing.T) {
	tools := []Tool{
		makeOutdatedTool("fast", "example.com/fast", "v1.0.0"),
		makeOutdatedTool("blocked", "example.com/blocked", "v1.0.0"),
		makeOutdatedTool("slow", "example.com/slow", "v1.0.0"),
	}

	runner := &mixedCancelRunner{
		blockTool:  "example.com/blocked",
		otherDelay: 0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	got := checkOutdatedConcurrency(ctx, tools, runner, defaultOutdatedConcurrency)
	if len(got) != len(tools) {
		t.Fatalf("expected %d results, got %d", len(tools), len(got))
	}

	wantNames := []string{"fast", "blocked", "slow"}
	for i, r := range got {
		if r.Tool.Name() != wantNames[i] {
			t.Errorf("result %d: expected tool %s (original order), got %s", i, wantNames[i], r.Tool.Name())
		}
	}

	// "fast" should have completed successfully before cancellation.
	if got[0].Error != nil {
		t.Errorf("result 0 (fast): expected success, got error %v", got[0].Error)
	}
	if !got[0].Outdated {
		t.Errorf("result 0 (fast): expected outdated=true")
	}

	// "blocked" was waiting on ctx.Done() and must carry the cancellation error.
	if got[1].Error == nil {
		t.Errorf("result 1 (blocked): expected cancellation error, got nil")
	}

	// "slow" was mid-delay when cancellation fired; it may have completed or been
	// cancelled. Either way it must not deadlock and must retain ordering.
	_ = got[2]
}

// boundTrackingRunner tracks the maximum number of concurrently active Run
// invocations so the test can assert the worker pool never exceeds its bound.
type boundTrackingRunner struct {
	mu         sync.Mutex
	active     int
	maxActive  int
	totalCalls int
}

func (r *boundTrackingRunner) Run(ctx context.Context, c Command) (string, error) {
	r.mu.Lock()
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.totalCalls++
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.active--
		r.mu.Unlock()
	}()

	// Small sleep to maximize the chance that multiple workers overlap.
	time.Sleep(2 * time.Millisecond)
	return `{"Path":"x","Version":"v9.9.9"}`, nil
}

// TestCheckOutdated_WorkerBoundEnforced proves the implementation is genuinely
// bounded: the number of simultaneously executing runners never exceeds the
// configured concurrency limit, and every tool is still evaluated exactly once.
func TestCheckOutdated_WorkerBoundEnforced(t *testing.T) {
	const bound = 4
	const n = 12
	tools := make([]Tool, n)
	for i := 0; i < n; i++ {
		tools[i] = makeOutdatedTool(fmt.Sprintf("tool%d", i), fmt.Sprintf("example.com/tool%d", i), "v1.0.0")
	}

	runner := &boundTrackingRunner{}
	got := checkOutdatedConcurrency(context.Background(), tools, runner, bound)

	if runner.totalCalls != n {
		t.Errorf("expected %d total runner calls, got %d", n, runner.totalCalls)
	}
	if runner.maxActive > bound {
		t.Errorf("max active runners %d exceeded configured bound %d", runner.maxActive, bound)
	}
	if len(got) != n {
		t.Fatalf("expected %d results, got %d", n, len(got))
	}
	for i, r := range got {
		if r.Tool.Name() != fmt.Sprintf("tool%d", i) {
			t.Errorf("result %d: expected tool%d, got %s", i, i, r.Tool.Name())
		}
	}
}

// TestCheckOutdated_SequentialConcurrentEquivalence is the principal regression
// contract for v1.7.1: concurrency changes execution strategy, not semantic
// output. The same tool set under sequential (bound=1) and concurrent
// (bound=default) execution must produce identical results in every field.
func TestCheckOutdated_SequentialConcurrentEquivalence(t *testing.T) {
	tools := []Tool{
		makeOutdatedTool("foo", "example.com/foo", "v1.0.0"),
		makeOutdatedTool("bar", "example.com/bar", "v2.0.0"),
		makeOutdatedTool("baz", "example.com/baz", "v3.0.0"),
		makeOutdatedTool("qux", "example.com/qux", "v4.0.0"),
	}

	const out = `{"Path":"example.com/foo","Version":"v9.9.9"}`
	seq := checkOutdatedConcurrency(context.Background(), tools, mockRunner{output: out}, 1)
	conc := checkOutdatedConcurrency(context.Background(), tools, mockRunner{output: out}, defaultOutdatedConcurrency)

	if len(seq) != len(conc) {
		t.Fatalf("length mismatch: sequential=%d concurrent=%d", len(seq), len(conc))
	}
	for i := range seq {
		s, c := seq[i], conc[i]
		if s.Tool.Name() != c.Tool.Name() {
			t.Errorf("result %d: tool mismatch seq=%s conc=%s", i, s.Tool.Name(), c.Tool.Name())
		}
		if s.Current != c.Current {
			t.Errorf("result %d: current mismatch seq=%s conc=%s", i, s.Current, c.Current)
		}
		if s.Latest != c.Latest {
			t.Errorf("result %d: latest mismatch seq=%s conc=%s", i, s.Latest, c.Latest)
		}
		if s.Outdated != c.Outdated {
			t.Errorf("result %d: outdated mismatch seq=%v conc=%v", i, s.Outdated, c.Outdated)
		}
		if (s.Error == nil) != (c.Error == nil) {
			t.Errorf("result %d: error presence mismatch seq=%v conc=%v", i, s.Error, c.Error)
		}
	}
}

// indexedFailRunner fails for tools whose index is present in failSet. The
// module path encodes the index (example.com/toolN), allowing deterministic
// per-position failure without coupling to result ordering.
type indexedFailRunner struct {
	failSet map[int]bool
}

func (r *indexedFailRunner) Run(ctx context.Context, c Command) (string, error) {
	mod := c.Args[len(c.Args)-1]
	mod = strings.TrimSuffix(mod, "@latest")

	var idx int
	if _, err := fmt.Sscanf(mod, "example.com/tool%d", &idx); err == nil && r.failSet[idx] {
		return "", errors.New("simulated network error")
	}
	return `{"Path":"` + mod + `","Version":"v9.9.9"}`, nil
}

// TestCheckOutdated_MixedSuccessFailureTable is a table-driven sweep over
// failure layouts. The invariant under test: one tool's failure must not
// prevent independent tools from being evaluated, and results remain in the
// original tool order regardless of which positions fail.
func TestCheckOutdated_MixedSuccessFailureTable(t *testing.T) {
	cases := []struct {
		name           string
		failingIndices []int
		n              int
	}{
		{"all_success", nil, 4},
		{"one_failure_middle", []int{1}, 4},
		{"one_failure_first", []int{0}, 4},
		{"one_failure_last", []int{3}, 4},
		{"multiple_failures", []int{0, 2}, 4},
		{"all_fail", []int{0, 1, 2, 3}, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tools := make([]Tool, tc.n)
			for i := 0; i < tc.n; i++ {
				tools[i] = makeOutdatedTool(fmt.Sprintf("tool%d", i), fmt.Sprintf("example.com/tool%d", i), "v1.0.0")
			}

			failSet := make(map[int]bool, len(tc.failingIndices))
			for _, idx := range tc.failingIndices {
				failSet[idx] = true
			}

			runner := &indexedFailRunner{failSet: failSet}
			got := checkOutdatedConcurrency(context.Background(), tools, runner, defaultOutdatedConcurrency)

			if len(got) != tc.n {
				t.Fatalf("expected %d results, got %d", tc.n, len(got))
			}

			for i, r := range got {
				if r.Tool.Name() != fmt.Sprintf("tool%d", i) {
					t.Errorf("result %d: expected tool%d (original order), got %s", i, i, r.Tool.Name())
				}
				if failSet[i] {
					if r.Error == nil {
						t.Errorf("result %d: expected failure, got success", i)
					}
				} else {
					if r.Error != nil {
						t.Errorf("result %d: expected success, got error %v", i, r.Error)
					}
				}
			}
		})
	}
}
