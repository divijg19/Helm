package tool

import (
	"testing"
)

func TestGetGobin(t *testing.T) {
	tests := []struct {
		name      string
		gobinOut  string
		gopathOut string
		want      string
		wantCalls []string
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
			gobinOut:  "\n",
			gopathOut: "/home/user/go\n",
			want:      "/home/user/go/bin",
			wantCalls: []string{"GOBIN", "GOPATH"},
		},
		{
			name:      "GOBIN empty GOPATH empty",
			gobinOut:  "\n",
			gopathOut: "\n",
			wantCalls: []string{"GOBIN", "GOPATH"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := func(args ...string) (string, error) {
				if len(args) >= 2 {
					if args[1] == "GOBIN" {
						return tc.gobinOut, nil
					}
					if args[1] == "GOPATH" {
						return tc.gopathOut, nil
					}
				}
				return "", nil
			}

			got, err := getGobin(env)
			_ = err

			if got != tc.want {
				t.Errorf("getGobin = %q, want %q", got, tc.want)
			}
		})
	}
}
