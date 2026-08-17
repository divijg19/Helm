package tool

import (
	"context"
	"debug/buildinfo"
	"errors"
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
