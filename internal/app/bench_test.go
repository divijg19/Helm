package app

import (
	"os"
	"testing"
)

// silenceStdout redirects os.Stdout so renderer benchmarks measure pure
// rendering cost without polluting benchmark output. The returned function
// must be deferred by the caller: os.Stdout is process-global, so leaving it
// pointed at /dev/null would silently swallow the rest of the test binary's
// output.
func silenceStdout() func() {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		panic(err)
	}
	old := os.Stdout
	os.Stdout = devNull
	return func() {
		os.Stdout = old
		devNull.Close()
	}
}

// benchUpdateReport is the shared render input: one update, one failure, and
// one skip, so both renderer benchmarks measure the same work and differ only
// in the renderer under test.
func benchUpdateReport() UpdateReport {
	return UpdateReport{
		Updated: []string{"tool1"},
		Failed:  []string{"tool2"},
		Skipped: []string{"tool3"},
	}
}

func BenchmarkJSONUpdateRendering(b *testing.B) {
	r := JSONRenderer{}
	report := benchUpdateReport()
	restore := silenceStdout()
	defer restore()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Update(report)
	}
}

func BenchmarkTerminalUpdateRendering(b *testing.B) {
	r := TerminalRenderer{}
	report := benchUpdateReport()
	restore := silenceStdout()
	defer restore()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Update(report)
	}
}
