package tool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestHelperProcess is not a real test: it re-executes the test binary as a
// child process that produces controlled output on demand. The mode selects
// the behavior; each mode exits zero except "fail".
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}
	const chunk = 4096
	switch os.Getenv("GO_TEST_HELPER_MODE") {
	case "dualstream":
		// Write ~256KiB to stdout and stderr interleaved, well above typical
		// 64KiB pipe buffers, so sequential draining would deadlock.
		lineOut := strings.Repeat("o", chunk-1) + "\n"
		lineErr := strings.Repeat("e", chunk-1) + "\n"
		for i := 0; i < 64; i++ {
			os.Stdout.WriteString(lineOut)
			os.Stderr.WriteString(lineErr)
		}
	case "longline":
		// One 256KiB line, above the bufio.Scanner default token limit.
		os.Stdout.WriteString(strings.Repeat("l", 256*1024) + "\n")
	case "fail":
		os.Stdout.WriteString("partial out\n")
		os.Stderr.WriteString("partial err\n")
		os.Exit(3)
	}
	os.Exit(0)
}

func helperCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER_PROCESS=1", "GO_TEST_HELPER_MODE="+mode)
	return cmd
}

// runHelperCommand executes the test binary as a child through DefaultRunner.
func runHelperCommand(t *testing.T, ctx context.Context, mode string, onLine func(string)) (string, error) {
	t.Helper()
	child := helperCommand(t, mode)
	// Rebuild the exec.Cmd as a tool.Command: DefaultRunner only needs a
	// binary path plus arguments, so re-express the child identically.
	return DefaultRunner{}.Run(ctx, Command{
		Name:   child.Path,
		Args:   child.Args[1:],
		Dir:    "",
		Env:    []string{"GO_TEST_HELPER_PROCESS=1", "GO_TEST_HELPER_MODE=" + mode},
		OnLine: onLine,
	})
}

// TestRunner_ConcurrentStreams proves both pipes drain without deadlock when
// each stream exceeds the kernel pipe buffer. Sequential draining would hang
// here because the child blocks filling stderr while stdout is awaited.
func TestRunner_ConcurrentStreams(t *testing.T) {
	var lines []string
	out, err := runHelperCommand(t, context.Background(), "dualstream", func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("dual-stream run failed: %v", err)
	}
	var sawOut, sawErr int
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, strings.Repeat("o", 16)):
			sawOut++
		case strings.HasPrefix(line, strings.Repeat("e", 16)):
			sawErr++
		default:
			t.Fatalf("unexpected line prefix: %q", line[:16])
		}
	}
	if sawOut != 64 || sawErr != 64 {
		t.Errorf("got %d stdout + %d stderr lines, want 64 + 64 (total lines %d)", sawOut, sawErr, len(lines))
	}
	if !strings.Contains(out, strings.Repeat("o", 16)) || !strings.Contains(out, strings.Repeat("e", 16)) {
		t.Error("combined output must contain both streams")
	}
}

// TestRunner_LongLinePreserved proves lines above the bufio.Scanner default
// token limit are delivered intact instead of silently dropped.
func TestRunner_LongLinePreserved(t *testing.T) {
	var lines []string
	out, err := runHelperCommand(t, context.Background(), "longline", func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("long-line run failed: %v", err)
	}
	if len(lines) != 1 || len(lines[0]) != 256*1024 {
		t.Fatalf("got %d lines (first %d bytes), want 1 line of %d bytes", len(lines), len(firstOrEmpty(lines)), 256*1024)
	}
	if !strings.Contains(out, strings.Repeat("l", 1024)) {
		t.Error("combined output must contain the long line")
	}
}

func firstOrEmpty(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

// TestRunner_ProcessFailureKeepsOutput proves process exit errors stay
// distinguishable and partial diagnostic output is preserved.
func TestRunner_ProcessFailureKeepsOutput(t *testing.T) {
	out, err := runHelperCommand(t, context.Background(), "fail", nil)
	if err == nil {
		t.Fatal("expected non-nil error for exit code 3")
	}
	if !strings.Contains(out, "partial out") || !strings.Contains(out, "partial err") {
		t.Errorf("partial output must be preserved on failure, got:\n%s", out)
	}
}

// errReader fails after n successful empty reads to simulate a stream error.
type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

// TestDrainStream_SurfacesReadError proves stream read failures reach the
// caller instead of ending the scan silently.
func TestDrainStream_SurfacesReadError(t *testing.T) {
	ch := make(chan streamResult, 4)
	drainStream(errReader{err: errors.New("simulated stream failure")}, "stderr", ch)
	close(ch)
	var errs []error
	for res := range ch {
		if res.err != nil {
			errs = append(errs, res.err)
		}
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "stderr") {
		t.Errorf("expected one stderr-attributed stream error, got %v", errs)
	}
}
