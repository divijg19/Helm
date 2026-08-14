package tool

import (
	"errors"
	"strings"
	"testing"
)

type errTest string

func (e errTest) Error() string { return string(e) }

func TestGetGobin(t *testing.T) {
	tests := []struct {
		name           string
		gobinOut       string
		gobinErr       error
		gopathOut      string
		gopathErr      error
		wantPath       string
		wantErrContain string
		wantCalls      []string
	}{
		{
			name:      "Case A — GOBIN set",
			gobinOut:  "/custom/bin\n",
			gopathOut: "/home/user/go\n",
			wantPath:  "/custom/bin",
			wantCalls: []string{"GOBIN"},
		},
		{
			name:      "Case B — GOPATH fallback (GOBIN empty)",
			gobinOut:  "\n",
			gopathOut: "/home/user/go\n",
			wantPath:  "/home/user/go/bin",
			wantCalls: []string{"GOBIN", "GOPATH"},
		},
		{
			name:      "Case C — GOBIN query failure fallback",
			gobinErr:  errTest("go env GOBIN failed"),
			gopathOut: "/home/user/go\n",
			wantPath:  "/home/user/go/bin",
			wantCalls: []string{"GOBIN", "GOPATH"},
		},
		{
			name:           "Case D — empty GOPATH",
			gobinOut:       "\n",
			gopathOut:      "\n",
			wantErrContain: "GOPATH is not set and GOBIN is empty",
			wantCalls:      []string{"GOBIN", "GOPATH"},
		},
		{
			name:           "Case E — GOPATH query failure",
			gobinOut:       "\n",
			gopathErr:      errTest("go env GOPATH failed"),
			wantErrContain: "failed to determine GOPATH",
			wantCalls:      []string{"GOBIN", "GOPATH"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			env := func(args ...string) (string, error) {
				if len(args) >= 2 {
					calls = append(calls, args[1])
				}
				if len(args) >= 2 && args[1] == "GOBIN" {
					return tc.gobinOut, tc.gobinErr
				}
				if len(args) >= 2 && args[1] == "GOPATH" {
					return tc.gopathOut, tc.gopathErr
				}
				return "", nil
			}

			got, err := getGobin(env)

			if tc.wantErrContain != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrContain)
				}
				if !errors.Is(err, ErrGobinResolution) {
					t.Errorf("expected error to wrap ErrGobinResolution, got %v", err)
				}
				if !strings.Contains(err.Error(), tc.wantErrContain) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErrContain)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.wantPath {
					t.Errorf("getGobin = %q, want %q", got, tc.wantPath)
				}
			}

			if len(calls) != len(tc.wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, tc.wantCalls)
			}
			for i, call := range calls {
				if call != tc.wantCalls[i] {
					t.Errorf("call at index %d = %q, want %q (all calls: %v)", i, call, tc.wantCalls[i], calls)
				}
			}
		})
	}
}
