package tool

import (
	"context"
	"debug/buildinfo"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"
)

// benchGobin builds one real Go binary into a temp directory so Load exercises
// the actual discovery + buildinfo inspection path against a real filesystem.
func benchGobin(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	src := filepath.Join("..", "..", "testdata", "fixtures", "binaries", "hello")
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "hello"), ".")
	cmd.Dir = src
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("go build fixture failed: %v\n%s", err, out)
	}
	return dir
}

// benchBinaries copies the real fixture binary to the given names inside one
// directory, so a benchmark that stats or executes tool paths measures the
// present-and-executable case rather than the missing-file error path.
func benchBinaries(b *testing.B, names ...string) []string {
	b.Helper()
	dir := benchGobin(b)
	src := filepath.Join(dir, "hello")
	paths := make([]string, 0, len(names))
	for _, name := range names {
		dst := filepath.Join(dir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o755); err != nil {
			b.Fatal(err)
		}
		paths = append(paths, dst)
	}
	return paths
}

func BenchmarkLoad(b *testing.B) {
	dir := benchGobin(b)
	// Validate once outside the timed region so a broken fixture cannot add
	// assertion cost to every sample.
	if _, err := Load(dir); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerify measures the healthy path against real executables. Pointing
// the fixtures at /gobin/... would stat missing files every iteration and
// report only the unhealthy case, which is the opposite of what a GOBIN
// normally contains.
func BenchmarkVerify(b *testing.B) {
	paths := benchBinaries(b, "tool1", "tool2", "tool3")
	tools := make([]Tool, 0, len(paths))
	for i, p := range paths {
		name := fmt.Sprintf("tool%d", i+1)
		tools = append(tools, NewTool(name, p, &buildinfo.BuildInfo{
			Path: "example.com/" + name + "/cmd/" + name,
			Main: debug.Module{Path: "example.com/" + name, Version: "v1.0.0"},
		}))
	}
	results := Verify(tools)
	for _, vr := range results {
		if !vr.Healthy {
			b.Fatalf("benchmark fixture must be healthy, got %s unhealthy: %v", vr.Tool.Name(), vr.Error)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Verify(tools)
	}
}

func BenchmarkPlan(b *testing.B) {
	tools := []Tool{
		fixtureTool("tool1", "example.com/tool1", "v1.0.0"),
		fixtureTool("tool2", "example.com/tool2", "v2.0.0"),
		fixtureTool("tool3", "example.com/tool3", "(devel)"),
	}
	loadRes := LoadResult{Tools: tools}
	filter := []string{"tool1", "tool2"}
	if got := Plan(loadRes, filter); len(got.ToUpdate) != 2 {
		b.Fatalf("benchmark fixture must select 2 tools, got %d", len(got.ToUpdate))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Plan(loadRes, filter)
	}
}

func BenchmarkCheckOutdated(b *testing.B) {
	ctx := context.Background()
	tools := []Tool{
		fixtureTool("tool1", "example.com/tool1", "v1.0.0"),
		fixtureTool("tool2", "example.com/tool2", "v2.0.0"),
	}
	runner := mockRunner{output: `{"Path":"example.com/tool1","Version":"v1.1.0"}`}
	if got := CheckOutdated(ctx, tools, runner); len(got) != 2 {
		b.Fatalf("benchmark fixture must produce 2 results, got %d", len(got))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckOutdated(ctx, tools, runner)
	}
}

// BenchmarkCheckOutdatedConcurrent exercises the bounded worker path with a
// larger tool set using the deterministic mock runner. It guards against
// regression in the concurrent dispatch (goroutine/channel overhead) without
// depending on the real network or module proxy.
func BenchmarkCheckOutdatedConcurrent(b *testing.B) {
	ctx := context.Background()
	const n = 16
	tools := make([]Tool, n)
	for i := 0; i < n; i++ {
		tools[i] = fixtureTool(fmt.Sprintf("tool%d", i), fmt.Sprintf("example.com/tool%d", i), "v1.0.0")
	}
	runner := mockRunner{output: `{"Path":"example.com/tool1","Version":"v1.1.0"}`}
	if got := checkOutdatedConcurrency(ctx, tools, runner, defaultOutdatedConcurrency); len(got) != n {
		b.Fatalf("benchmark fixture must produce %d results, got %d", n, len(got))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checkOutdatedConcurrency(ctx, tools, runner, defaultOutdatedConcurrency)
	}
}
