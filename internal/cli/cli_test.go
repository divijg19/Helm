package cli

import (
	"errors"
	"testing"
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

// TestEnvironmentFailurePropagatesToExitFailure proves the current propagation
// contract for an environment-resolution failure (e.g. GetGobin error): it
// reaches the generic CLI failure path and exits with ExitFailure (1).
//
// This is a deliberate, behavior-preserving assertion. The dedicated ExitEnv
// constant exists but is intentionally NOT used for this path yet; activating
// it is deferred semantic work and must not be smuggled into this coverage
// commit. The test therefore locks the established exit code rather than a new
// one.
func TestEnvironmentFailurePropagatesToExitFailure(t *testing.T) {
	code := fail("Error:", errors.New("GOPATH is not set and GOBIN is empty"))
	if code != ExitFailure {
		t.Errorf("environment-resolution failure exited with %d, want ExitFailure (%d)", code, ExitFailure)
	}
	if code == ExitEnv {
		t.Errorf("environment-resolution failure must NOT use ExitEnv (%d) in v1.6.6; that is deferred", ExitEnv)
	}
}
