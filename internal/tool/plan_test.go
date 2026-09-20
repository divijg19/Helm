package tool

import (
	"debug/buildinfo"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
)

func planTool(name string, devel bool) Tool {
	version := "v1.0.0"
	if devel {
		version = "(devel)"
	}
	return NewTool(name, "/gobin/"+name, &buildinfo.BuildInfo{
		Path: "example.com/" + name + "/cmd/" + name,
		Main: debug.Module{Path: "example.com/" + name, Version: version},
	})
}

func TestPlan_AllTools(t *testing.T) {
	tools := []Tool{
		planTool("hello", false),
		planTool("world", false),
		planTool("localdev", true),
	}
	result := Plan(LoadResult{Tools: tools}, nil)
	if len(result.ToUpdate) != 2 {
		t.Errorf("expected 2 updatable tools, got %d", len(result.ToUpdate))
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Name() != "localdev" {
		t.Errorf("expected localdev skipped, got %v", result.Skipped)
	}
}

func TestPlan_Filter(t *testing.T) {
	tools := []Tool{
		planTool("hello", false),
		planTool("world", false),
		planTool("localdev", true),
	}
	result := Plan(LoadResult{Tools: tools}, []string{"world"})
	if len(result.ToUpdate) != 1 || result.ToUpdate[0].Name() != "world" {
		t.Errorf("expected only world to update, got %v", result.ToUpdate)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("filtered-out tools must not appear in skipped, got %v", result.Skipped)
	}
}

func TestPlan_FilterMatchesLocalOnly(t *testing.T) {
	tools := []Tool{
		planTool("localdev", true),
	}
	result := Plan(LoadResult{Tools: tools}, []string{"localdev"})
	if len(result.ToUpdate) != 0 {
		t.Errorf("local tool must never be an update candidate, got %v", result.ToUpdate)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Name() != "localdev" {
		t.Errorf("expected localdev in skipped, got %v", result.Skipped)
	}
}

func TestPlan_InvalidBinariesAlwaysSkipped(t *testing.T) {
	loadRes := LoadResult{
		Tools: []Tool{planTool("hello", false)},
		Invalid: []InvalidBinary{
			{Path: "/gobin/notgo", Error: ErrMissingBuildInfo},
		},
	}
	result := Plan(loadRes, []string{"hello"})
	if len(result.Invalid) != 1 || result.Invalid[0].Path != "/gobin/notgo" {
		t.Errorf("invalid binaries must always be reported, got %v", result.Invalid)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("no local tools expected, got %v", result.Skipped)
	}
}

func TestInstallRef(t *testing.T) {
	if got := InstallRef("example.com/hello"); got != "example.com/hello@latest" {
		t.Errorf("InstallRef = %q, want example.com/hello@latest", got)
	}
}

func TestInstallCommand(t *testing.T) {
	got := InstallCommand("example.com/hello")
	want := "go install example.com/hello@latest"
	if got != want {
		t.Errorf("InstallCommand = %q, want %q", got, want)
	}
}

// The plan display rule (floating @latest) intentionally differs from the
// executed reference (pinned @resolved version): --check shows what an
// update would run, while installation pins the exact version outdated
// resolution authorized. This test pins both sides of that distinction so
// neither the display nor the execution reference drifts silently.
func TestInstallDisplayVsExecutionReferences(t *testing.T) {
	target := "example.com/hello"
	if got := InstallCommand(target); got != "go install "+target+"@latest" {
		t.Errorf("plan display command = %q, want floating @latest reference", got)
	}
	if got := InstallExactRef(target, "v1.4.2"); got != target+"@v1.4.2" {
		t.Errorf("executed reference = %q, want pinned @version reference", got)
	}
	if strings.Contains(InstallExactRef(target, "v1.4.2"), "@latest") {
		t.Errorf("executed reference must never float to @latest")
	}
}

// TestUnknownFilterNames pins filter validation: every supplied name must
// match a known tool. Unknown names are reported in first-seen order without
// duplicates; an empty filter and fully known filters report nothing.
func TestUnknownFilterNames(t *testing.T) {
	tools := []Tool{
		planTool("hello", false),
		planTool("world", false),
		planTool("localdev", true),
	}
	tests := []struct {
		name   string
		filter []string
		want   []string
	}{
		{"empty filter", nil, nil},
		{"single known", []string{"hello"}, nil},
		{"multiple known", []string{"hello", "world"}, nil},
		{"known local tool", []string{"localdev"}, nil},
		{"single unknown", []string{"nope"}, []string{"nope"}},
		{"foreign completion words", []string{"completion", "fish"}, []string{"completion", "fish"}},
		{"mixed known and unknown", []string{"hello", "nope", "world"}, []string{"nope"}},
		{"duplicates reported once", []string{"nope", "hello", "nope"}, []string{"nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UnknownFilterNames(tools, tt.filter); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UnknownFilterNames(%v) = %v, want %v", tt.filter, got, tt.want)
			}
		})
	}
}
