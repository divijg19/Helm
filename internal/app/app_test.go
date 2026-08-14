package app

import (
	"errors"
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
		return "", errors.New("GOPATH is not set and GOBIN is empty")
	}
	defer func() { gobinResolver = prev }()

	result, err := NewApp(NewRenderer(ModeTerminal, false), tool.DefaultRunner{})
	if err == nil {
		t.Fatalf("expected NewApp to fail when GOBIN resolution fails, got nil (app=%v)", result)
	}
}
