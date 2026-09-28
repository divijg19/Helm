package tool

import (
	"errors"
	"fmt"
	"testing"

	"github.com/divijg19/Helm/internal/testutil"
)

func TestLoad_RealGOBIN(t *testing.T) {
	f := testutil.NewFixture(t)

	loadRes, err := Load(f.GobinDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loadRes.Tools) != 3 {
		names := make([]string, len(loadRes.Tools))
		for i, tool := range loadRes.Tools {
			names[i] = tool.Name()
		}
		t.Errorf("expected 3 valid tools (hello, world, localdev), got %d: %v", len(loadRes.Tools), names)
	}

	if len(loadRes.Invalid) != 1 {
		invalids := make([]string, len(loadRes.Invalid))
		for i, inv := range loadRes.Invalid {
			invalids[i] = inv.Path
		}
		t.Errorf("expected 1 invalid (notgo), got %d: %v", len(loadRes.Invalid), invalids)
	}
}

func TestLoad_SortedAlphabetically(t *testing.T) {
	f := testutil.NewFixture(t)

	loadRes, err := Load(f.GobinDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	for i := 1; i < len(loadRes.Tools); i++ {
		if loadRes.Tools[i-1].Name() > loadRes.Tools[i].Name() {
			t.Errorf("tools not sorted: %s > %s", loadRes.Tools[i-1].Name(), loadRes.Tools[i].Name())
		}
	}
}

// TestInvalidBinaryMessage covers every arm of Message, including the
// default. The two sentinels were reachable through the CLI fixture, but the
// fallback had no test at all, so a regression that dropped the errors.Is
// chain would keep every golden green while mislabelling real failures.
func TestInvalidBinaryMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"missing build info", ErrMissingBuildInfo, "missing or unreadable build info"},
		{"missing package path", ErrMissingPackagePath, "missing main package path"},
		{"wrapped missing build info", fmt.Errorf("%w: open x: no such file", ErrMissingBuildInfo), "missing or unreadable build info"},
		{"unrecognized error falls back", errors.New("something else"), "unable to inspect binary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (InvalidBinary{Path: "/gobin/x", Error: tt.err}).Message(); got != tt.want {
				t.Errorf("Message() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestInvalidBinaryErrorKeepsCause proves the inspection cause stays
// unwrappable, so callers can inspect it with errors.Is/errors.As. The
// classification sentinel must remain the one Message() matches.
func TestInvalidBinaryErrorKeepsCause(t *testing.T) {
	cause := errors.New("permission denied")
	inv := InvalidBinary{Path: "/gobin/x", Error: fmt.Errorf("%w: %w", ErrMissingBuildInfo, cause)}

	if !errors.Is(inv.Error, ErrMissingBuildInfo) {
		t.Error("classification sentinel must stay unwrappable")
	}
	if !errors.Is(inv.Error, cause) {
		t.Error("underlying cause must stay unwrappable")
	}
	if got := inv.Message(); got != "missing or unreadable build info" {
		t.Errorf("Message() = %q, want the stable sentinel text", got)
	}
}
