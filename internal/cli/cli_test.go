package cli

import "testing"

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
