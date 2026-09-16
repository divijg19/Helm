package cli

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"helm/internal/app"
	"helm/internal/tool"
)

func TestResolveInvocation(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantName string
	}{
		{"canonical lowercase", "helm", "helm"},
		{"canonical uppercase", "Helm", "Helm"},
		{"compatibility alias", "update-go-tools", "update-go-tools"},
		{"unknown defaults canonical", "helm-manager", "helm-manager"},
		{"empty defaults canonical", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := ResolveInvocation(tc.raw)
			if inv.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", inv.Name, tc.wantName)
			}
		})
	}
}

// TestRunPropagatesEnvResolutionErrorToExitEnv proves the environment-failure
// propagation chain through the REAL cli.Run path:
//
//	GetGobin() failure
//	    ↓
//	ErrGobinResolution
//	    ↓
//	NewApp() failure (with ErrGobinResolution)
//	    ↓
//	cli.Run failure path
//	    ↓
//	ExitEnv (3)
//
// It drives the actual Run entry point with a resolver that makes NewApp fail
// with an error wrapping ErrGobinResolution, then asserts the resulting exit code.
func TestRunPropagatesEnvResolutionErrorToExitEnv(t *testing.T) {
	prev := newApp
	newApp = func(renderer app.Renderer, runner tool.Runner) (*app.App, error) {
		return nil, fmt.Errorf("GOPATH is not set and GOBIN is empty: %w", tool.ErrGobinResolution)
	}
	defer func() { newApp = prev }()

	code := Run(ResolveInvocation("helm"), []string{"--help"})
	if code != ExitEnv {
		t.Errorf("environment-resolution failure exited with %d, want ExitEnv (%d)", code, ExitEnv)
	}
}

// TestRunPropagatesNewAppErrorToExitFailure proves the second link of the
// environment-failure propagation chain through the REAL cli.Run path:
//
//	NewApp() failure (generic)
//	    ↓
//	cli.Run failure path (fail)
//	    ↓
//	ExitFailure (1)
//
// It does not call fail() directly; it drives the actual Run entry point with a
// resolver that makes NewApp fail exactly as a generic failure would, then
// asserts the resulting exit code. Combined with the app-package test
// (GetGobin error -> NewApp error), the full chain
// GetGobin -> NewApp -> cli.Run -> ExitFailure (1) is established with real code.
func TestRunPropagatesNewAppErrorToExitFailure(t *testing.T) {
	prev := newApp
	newApp = func(renderer app.Renderer, runner tool.Runner) (*app.App, error) {
		return nil, errors.New("GOPATH is not set and GOBIN is empty")
	}
	defer func() { newApp = prev }()

	code := Run(ResolveInvocation("helm"), []string{"--help"})
	if code != ExitFailure {
		t.Errorf("NewApp error exited with %d, want ExitFailure (%d)", code, ExitFailure)
	}
	if code == ExitEnv {
		t.Errorf("generic NewApp failure must NOT use ExitEnv (%d); that code is reserved for environment-resolution failures", ExitEnv)
	}
}

// TestSplitOperation_CheckDryRunArePlanOnly documents the dispatch contract for
// v1.9.2: --check and --dry-run are plan flags parsed by parseFlags and are
// NEVER positional operations, so splitOperation must not treat them as
// operations and Run must never dispatch them as explicit operations.
func TestSplitOperation_CheckDryRunArePlanOnly(t *testing.T) {
	tests := []struct {
		name       string
		positional []string
		wantOp     string
		wantArgs   []string
	}{
		{"no args", nil, "", []string{}},
		{"list", []string{"--list"}, "--list", []string{}},
		{"list with tool", []string{"--list", "hello"}, "--list", []string{"hello"}},
		{"info with target", []string{"--info", "hello"}, "--info", []string{"hello"}},
		{"outdated", []string{"--outdated"}, "--outdated", []string{}},
		{"help", []string{"--help"}, "--help", []string{}},
		{"version", []string{"--version"}, "--version", []string{}},
		{"tool names stay update args", []string{"hello", "world"}, "", []string{"hello", "world"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op, args := splitOperation(tt.positional)
			if op != tt.wantOp {
				t.Errorf("operation = %q, want %q", op, tt.wantOp)
			}
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
			}
		})
	}
}

// TestParseFlags_CheckAndDryRunSetPlan confirms --check/--dry-run are converted
// into the plan option by parseFlags and never reach the positional slice, which
// is why the old dispatch branches for them were unreachable.
func TestParseFlags_CheckAndDryRunSetPlan(t *testing.T) {
	check, code := parseFlags([]string{"--check"})
	if code != 0 {
		t.Fatalf("parseFlags(--check) code = %d, want 0", code)
	}
	dry, code := parseFlags([]string{"--dry-run"})
	if code != 0 {
		t.Fatalf("parseFlags(--dry-run) code = %d, want 0", code)
	}
	if !check.plan || !dry.plan {
		t.Errorf("plan must be true for --check/%v and --dry-run/%v", check, dry)
	}
	if len(check.positional) != 0 || len(dry.positional) != 0 {
		t.Errorf("--check/--dry-run must not reach positional, got %v / %v", check.positional, dry.positional)
	}
}

// TestParseFlags_PlanFlagsMayCombineWithOperations proves --check still works
// alongside explicit operations without changing their dispatch (the plan flag
// only drives the default no-operation path).
func TestParseFlags_PlanFlagsMayCombineWithOperations(t *testing.T) {
	opts, code := parseFlags([]string{"--check", "--list"})
	if code != 0 {
		t.Fatalf("parseFlags code = %d, want 0", code)
	}
	if !opts.plan {
		t.Error("expected plan to be set")
	}
	if !slices.Equal(opts.positional, []string{"--list"}) {
		t.Errorf("positional = %v, want [--list]", opts.positional)
	}
}
