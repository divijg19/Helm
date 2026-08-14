package cli

import (
	"errors"
	"testing"

	"helm/internal/app"
	"helm/internal/tool"
)

func TestResolveInvocation(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantName  string
		wantCanon bool
	}{
		{"canonical lowercase", "helm", "helm", true},
		{"canonical uppercase", "Helm", "Helm", true},
		{"compatibility alias", "update-go-tools", "update-go-tools", false},
		{"unknown defaults canonical", "helm-manager", "helm-manager", true},
		{"empty defaults canonical", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := ResolveInvocation(tc.raw)
			if inv.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", inv.Name, tc.wantName)
			}
			if inv.Canonical != tc.wantCanon {
				t.Errorf("Canonical = %v, want %v", inv.Canonical, tc.wantCanon)
			}
		})
	}
}

// TestRunPropagatesNewAppErrorToExitFailure proves the second link of the
// environment-failure propagation chain through the REAL cli.Run path:
//
//	NewApp() failure
//	    ↓
//	cli.Run failure path (fail)
//	    ↓
//	ExitFailure (1)
//
// It does not call fail() directly; it drives the actual Run entry point with a
// resolver that makes NewApp fail exactly as a GOBIN/GOPATH resolution failure
// would, then asserts the resulting exit code. Combined with the app-package
// test (GetGobin error -> NewApp error), the full chain
// GetGobin -> NewApp -> cli.Run -> ExitFailure is established with real code.
//
// ExitEnv remains intentionally unused: activating it is deferred semantic
// work and must not be smuggled into this coverage commit.
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
		t.Errorf("environment-resolution failure must NOT use ExitEnv (%d) in v1.6.7; that is deferred", ExitEnv)
	}
}
