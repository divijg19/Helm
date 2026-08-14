package tool

import (
	"strings"
	"testing"
)

// errTest is a trivial error used to simulate a `go env` subprocess failure in
// the resolution seam. It is local to the test so the seam stays hermetic.
type errTest string

func (e errTest) Error() string { return string(e) }

func TestGetGobin(t *testing.T) {
	tests := []struct {
		name           string
		gobinOut       string
		gobinErr       error
		gopathOut      string
		gopathErr      error
		want           string
		wantErrContain string
		wantCalls      []string
	}{
		{
			name:      "GOBIN set short-circuits GOPATH",
			gobinOut:  "/custom/bin\n",
			gopathOut: "/home/user/go\n",
			want:      "/custom/bin",
			wantCalls: []string{"GOBIN"},
		},
		{
			name:      "GOBIN empty GOPATH set",
			gobinOut:  "\n",
			gopathOut: "/home/user/go\n",
			want:      "/home/user/go/bin",
			wantCalls: []string{"GOBIN", "GOPATH"},
		},
		{
			name:      "GOBIN query fails falls back to GOPATH",
			gobinErr:  errTest("go env GOBIN failed"),
			gopathOut: "/home/user/go\n",
			want:      "/home/user/go/bin",
			wantCalls: []string{"GOBIN", "GOPATH"},
		},
		{
			name:           "GOBIN empty GOPATH empty",
			gobinOut:       "\n",
			gopathOut:      "\n",
			wantErrContain: "GOPATH is not set and GOBIN is empty",
			wantCalls:      []string{"GOBIN", "GOPATH"},
		},
		{
			name:           "GOPATH query fails",
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
				switch args[1] {
				case "GOBIN":
					return tc.gobinOut, tc.gobinErr
				case "GOPATH":
					return tc.gopathOut, tc.gopathErr
				}
				return "", nil
			}

			got, err := getGobin(env)

			if tc.wantErrContain != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrContain)
				}
				if !strings.Contains(err.Error(), tc.wantErrContain) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErrContain)
				}
				if strings.Join(calls, ",") != strings.Join(tc.wantCalls, ",") {
					t.Errorf("go env call order = %v, want %v", calls, tc.wantCalls)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("getGobin = %q, want %q", got, tc.want)
			}
			if strings.Join(calls, ",") != strings.Join(tc.wantCalls, ",") {
				t.Errorf("go env call order = %v, want %v", calls, tc.wantCalls)
			}
		})
	}
}
