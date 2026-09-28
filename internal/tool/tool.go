package tool

import (
	"debug/buildinfo"
)

type Tool struct {
	name string
	path string
	info *buildinfo.BuildInfo
}

// NewTool constructs a Tool from its discovered name, filesystem path, and
// embedded build metadata. It is the single construction path used by both
// inspection and callers that assemble Tools from known metadata.
func NewTool(name, path string, info *buildinfo.BuildInfo) Tool {
	return Tool{name: name, path: path, info: info}
}

func (t Tool) Name() string {
	return t.name
}

func (t Tool) Path() string {
	return t.path
}

func (t Tool) PackagePath() string {
	if t.info == nil {
		return ""
	}
	return t.info.Path
}

func (t Tool) ModulePath() string {
	if t.info == nil {
		return ""
	}
	if t.info.Main.Path != "" {
		return t.info.Main.Path
	}
	return t.info.Path
}

func (t Tool) Version() string {
	if t.info == nil || t.info.Main.Version == "" {
		return "unknown"
	}
	return t.info.Main.Version
}

func (t Tool) GoVersion() string {
	if t.info == nil || t.info.GoVersion == "" {
		return "unknown"
	}
	return t.info.GoVersion
}

// InstallTarget returns the main package path `go install` is given. The name
// distinguishes the install-time selector from ModulePath, which selects the
// module for `go list -m`; for a valid tool the two are different strings.
func (t Tool) InstallTarget() string {
	return t.PackagePath()
}

func (t Tool) CanUpdate() bool {
	pkg := t.PackagePath()
	if pkg == "" || pkg == "(devel)" {
		return false
	}
	// Version returns "unknown" rather than "" when no version is readable, so
	// the empty-string case is not reachable from a loaded tool.
	ver := t.Version()
	if ver == "unknown" || ver == "(devel)" {
		return false
	}
	return true
}

func (t Tool) IsValid() bool {
	return t.info != nil && t.PackagePath() != ""
}
