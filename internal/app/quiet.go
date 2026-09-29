package app

import (
	"fmt"
	"os"

	"github.com/divijg19/Helm/v2/internal/tool"
)

// QuietRenderer is the shell-scripting mode. It suppresses the banner,
// discovery summary, and progress renderer. Only the update operation is
// summary-only; inventory, plan, and outdated delegate to the terminal
// renderer (without its header) because their tables are the requested data.
type QuietRenderer struct{}

func (QuietRenderer) Header(HeaderInfo) error { return nil }

func (QuietRenderer) Inventory(report InventoryReport) error {
	// Inventory output is the requested data, not chatter; still print it but
	// without the discovery header.
	return TerminalRenderer{}.Inventory(report)
}

func (QuietRenderer) Plan(report PlanReport) error {
	return TerminalRenderer{}.Plan(report)
}

func (QuietRenderer) Outdated(report OutdatedReport) error {
	return TerminalRenderer{}.Outdated(report)
}

func (QuietRenderer) Update(report UpdateReport) error {
	for _, row := range updateSummaryRows(report) {
		printSummaryLine(row[0], row[1])
	}

	if len(report.Diagnostics) > 0 {
		fmt.Println()
		for _, d := range report.Diagnostics {
			fmt.Fprintf(os.Stderr, "diagnostic: %s: %s\n", d.ToolName, d.Message)
		}
	}

	if err := report.Err(); err != nil {
		fmt.Println()
		for _, f := range report.FailedDetail {
			fmt.Fprintf(os.Stderr, "failed: %s: %s\n", f.Name, f.Error)
		}
		for _, f := range report.Failed {
			if !hasFailureDetail(f, report.FailedDetail) {
				fmt.Fprintf(os.Stderr, "failed: %s\n", f)
			}
		}
		return err
	}
	return nil
}

// hasFailureDetail reports whether name already appears in detail, so a tool
// with a recorded cause is not listed twice.
func hasFailureDetail(name string, detail []ToolFailure) bool {
	for _, d := range detail {
		if d.Name == name {
			return true
		}
	}
	return false
}

func (QuietRenderer) Info(loadRes tool.LoadResult, target string) error {
	return TerminalRenderer{}.Info(loadRes, target)
}
