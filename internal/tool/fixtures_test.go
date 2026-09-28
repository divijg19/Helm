package tool

import (
	"debug/buildinfo"
	"runtime/debug"
)

// fixtureTool builds a synthetic updatable Tool shaped the way discovery and
// inspection produce one: a single valid binary at a synthetic GOBIN path
// whose main package sits at <pkg>/cmd/<name> inside module <pkg>.
//
// It is the one fixture builder for the whole package; tests that need a
// different shape (a local "(devel)" build, or a tool that must fail
// inspection) build that explicitly rather than cloning this shape.
func fixtureTool(name, pkg, version string) Tool {
	return NewTool(name, "/gobin/"+name, &buildinfo.BuildInfo{
		Path: pkg + "/cmd/" + name,
		Main: debug.Module{Path: pkg, Version: version},
	})
}

// moduleTool builds a fixture whose module and main-package path are both
// derived from the tool name, which is the shape the CLI test fixture and most
// plan tests use.
func moduleTool(name, version string) Tool {
	return fixtureTool(name, "example.com/"+name, version)
}

// localTool builds a fixture that cannot be updated: a "(devel)" main module
// version marks a local development build.
func localTool(name string) Tool {
	return moduleTool(name, "(devel)")
}
