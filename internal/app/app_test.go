package app

import (
	"errors"
	"fmt"
	"testing"

	"helm/internal/tool"
)

// TestNewAppPropagatesGobinResolutionError proves the first link of the
// environment-failure propagation chain with the real NewApp code:
//
//	GetGobin() failure
//	    ↓
//	NewApp() failure
//
// The failing resolver simulates the exact error tool.GetGobin returns when
// GOBIN and GOPATH are unset, so this exercises the genuine production path
// rather than mocking NewApp itself.
func TestNewAppPropagatesGobinResolutionError(t *testing.T) {
	prev := gobinResolver
	gobinResolver = func() (string, error) {
		return "", fmt.Errorf("GOPATH is not set and GOBIN is empty: %w", tool.ErrGobinResolution)
	}
	defer func() { gobinResolver = prev }()

	result, err := NewApp(NewRenderer(ModeTerminal, false), tool.DefaultRunner{})
	if err == nil {
		t.Fatalf("expected NewApp to fail when GOBIN resolution fails, got nil (app=%v)", result)
	}
	if !errors.Is(err, tool.ErrGobinResolution) {
		t.Fatalf("expected ErrGobinResolution, got %v", err)
	}
}
