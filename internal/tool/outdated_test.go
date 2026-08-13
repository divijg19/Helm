package tool

import (
	"context"
	"debug/buildinfo"
	"errors"
	"runtime/debug"
	"strings"
	"testing"
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

func TestCheckOutdated_RetractedRange_Within(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v1.1.5","Retracted":["[v1.1.0, v1.2.0]"]}`}
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
		t.Errorf("expected error for retracted latest version in range, got nil")
	} else if !strings.Contains(results[0].Error.Error(), "retracted") {
		t.Errorf("unexpected error message: %v", results[0].Error)
	}
}

func TestCheckOutdated_RetractedRange_Outside(t *testing.T) {
	runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v1.2.1","Retracted":["[v1.1.0, v1.2.0]"]}`}
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
	if results[0].Error != nil {
		t.Errorf("unexpected error for version outside retracted range: %v", results[0].Error)
	}
	if !results[0].Outdated {
		t.Errorf("expected version v1.2.1 to be outdated compared to v1.0.0, got outdated=false")
	}
}

func TestCheckOutdated_RetractedRange_Invalid(t *testing.T) {
	tests := []struct {
		name       string
		retraction string
	}{
		{"invalid low", "[invalid, v1.2.0]"},
		{"invalid high", "[v1.1.0, invalid]"},
		{"malformed range", "[v1.1.0]"},
		{"no brackets", "v1.1.0, v1.2.0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := mockRunner{output: `{"Path":"example.com/foo","Version":"v1.1.5","Retracted":["` + tc.retraction + `"]}`}
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
			// None of the invalid retraction syntaxes should successfully classify v1.1.5 as retracted,
			// so the version check itself should succeed and be outdated (no error).
			if results[0].Error != nil {
				t.Errorf("unexpected error for invalid retraction %q: %v", tc.retraction, results[0].Error)
			}
		})
	}
}
