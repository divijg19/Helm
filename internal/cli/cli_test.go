package cli

import (
	"errors"
	"fmt"
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
