package app

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/divijg19/Helm/internal/tool"
)

const (
	symCheck    = "✓"
	symBullet   = "•"
	symFail     = "✗"
	symOutdated = "↑"
)

type TerminalRenderer struct {
	verbose bool
}

func (r TerminalRenderer) Header(hdr HeaderInfo) error {
	if hdr.GoVersion != "" {
		fmt.Printf("Go: %s\n\n", hdr.GoVersion)
	}
	fmt.Println("Discovery")
	fmt.Println()
	fmt.Printf("  %-11s : %s\n", "Gobin", hdr.Gobin)
	fmt.Printf("  %-11s : %d\n", "Executables", hdr.LoadRes.Summary.Executables)
	fmt.Printf("  %-11s : %d\n", "Updatable", hdr.LoadRes.Summary.Updatable)
	fmt.Printf("  %-11s : %d\n", "Local", hdr.LoadRes.Summary.Local)
	fmt.Printf("  %-11s : %d\n", "Invalid", hdr.LoadRes.Summary.Invalid)
	fmt.Println()
	if hdr.LoadRes.Summary.Updatable > 0 {
		fmt.Println("Updatable tools:")
		// Collect the updatable tools once, then size the name column to the
		// widest of them. Two passes with the same filter (one to build
		// single-cell rows only to measure them) was pure overhead.
		updatable := make([]tool.Tool, 0, len(hdr.LoadRes.Tools))
		width := 16 // matches established alignment
		for _, t := range hdr.LoadRes.Tools {
			if !t.CanUpdate() {
				continue
			}
			updatable = append(updatable, t)
			if n := len(t.Name()); n > width {
				width = n
			}
		}
		for _, t := range updatable {
			fmt.Printf("  %s %-*s %s\n", symBullet, width, t.Name(), t.Version())
		}
		fmt.Println()
	}
	if hdr.LoadRes.Summary.Local > 0 {
		fmt.Println("Skipping local development binaries:")
		for _, t := range hdr.LoadRes.Tools {
			if !t.CanUpdate() {
				fmt.Printf("  %s %s\n", symBullet, t.Name())
			}
		}
		fmt.Println()
	}
	return nil
}

func (r TerminalRenderer) Inventory(report InventoryReport) error {
	if len(report.Tools) == 0 && len(report.Invalid) == 0 {
		fmt.Println("No Go tools found.")
		fmt.Println()
		printInventorySummary(report.Summary)
		// Still the report's own decision: an empty tool list with issues in
		// the summary must not exit 0 here while the other three renderers
		// exit 1. It is nil today only because no current producer can build
		// that shape.
		return report.Err()
	}

	var rows [][]string
	for _, t := range report.Tools {
		rows = append(rows, []string{t.Name, t.Version, t.Status, t.PackagePath})
	}
	// Floors match the header widths ("NAME", "VERSION", "STATUS", "PACKAGE").
	widths := columnWidths([]int{4, 7, 7, 7}, rows...)

	format := fmt.Sprintf("%%-%ds   %%-%ds   %%-%ds   %%-%ds   %%s\n", widths[0], widths[1], widths[2], widths[3])
	fmt.Printf(format, "NAME", "VERSION", "STATUS", "PACKAGE", "MODULE")

	for _, t := range report.Tools {
		// No empty-name guard is needed: every tool reaching a report was
		// discovered from a directory entry, so its name is never empty, and
		// a nameless row would misalign the table anyway.
		modPath := t.ModulePath
		if modPath == "" {
			modPath = "-"
		}
		fmt.Printf(format, t.Name, t.Version, t.Status, t.PackagePath, modPath)
		if t.Error != "" {
			fmt.Fprintf(os.Stderr, "  ↳ %s\n", t.Error)
		}
	}

	fmt.Println()
	fmt.Println("Invalid / Uninspectable binaries")
	fmt.Println()
	if len(report.Invalid) > 0 {
		for _, inv := range report.Invalid {
			fmt.Printf("  %s %s (%s)\n", symBullet, inv.Path, inv.Message)
		}
	} else {
		fmt.Println("  none")
	}

	fmt.Println()
	printInventorySummary(report.Summary)

	return report.Err()
}

func (r TerminalRenderer) Plan(report PlanReport) error {
	if len(report.WouldUpdate) > 0 {
		fmt.Println("Would update")
		fmt.Println()
		for _, item := range report.WouldUpdate {
			if r.verbose {
				fmt.Printf("  %s\n", item.Name)
				fmt.Printf("    Package : %s\n", item.PackagePath)
				fmt.Printf("    Command : %s\n\n", item.Command)
			} else {
				fmt.Printf("  %s\n", item.Name)
			}
		}
		fmt.Println()
	}

	if len(report.Skipped) > 0 {
		fmt.Println("Skipped")
		fmt.Println()
		for _, item := range report.Skipped {
			fmt.Printf("  %s %s\n", symBullet, item.Name)
		}
		fmt.Println()
	}

	printSummaryBlock([][2]string{
		{"Would update", itoa(len(report.WouldUpdate))},
		{"Skipped", itoa(len(report.Skipped))},
	})
	return nil
}

func (r TerminalRenderer) Outdated(report OutdatedReport) error {
	var rows [][]string
	for _, o := range report.Results {
		rows = append(rows, []string{o.Name, o.Current})
	}
	// Floors match the header widths ("NAME", "CURRENT").
	widths := columnWidths([]int{4, 7}, rows...)

	format := fmt.Sprintf("%%-%ds   %%-%ds   %%s\n", widths[0], widths[1])
	fmt.Printf(format, "NAME", "CURRENT", "STATUS")

	for _, o := range report.Results {
		status := symCheck
		if o.Error != "" {
			status = "error (" + o.Error + ")"
		} else if o.Outdated {
			status = symOutdated + " " + o.Latest
		}
		fmt.Printf(format, o.Name, o.Current, status)
	}

	fmt.Println()
	printSummaryBlock([][2]string{
		{"Checked", itoa(report.Summary.Total)},
		{"Outdated", itoa(report.Summary.Outdated)},
		{"Up-to-date", itoa(report.Summary.UpToDate)},
		{"Failed", itoa(report.Summary.Failed)},
	})
	return report.Err()
}

func (r TerminalRenderer) OnProgress(p tool.Progress) {
	switch p.Action {
	case "Start":
		if p.Version != "" {
			// Pad the version transition so the completion mark below stays in a
			// fixed column regardless of version-string length. Oversized
			// transitions are never truncated; they simply extend past the
			// alignment column.
			fmt.Printf("[%02d/%02d] %-18s %-15s", p.Current, p.Total, p.Tool.Name(), p.Tool.Version()+" → "+p.Version)
		} else {
			fmt.Printf("[%02d/%02d] %-18s", p.Current, p.Total, p.Tool.Name())
		}
	case "Output":
		fmt.Printf("  %s\n", p.Line)
	case "Complete":
		// A successful completion always arrives with no notes: installTool
		// drops them whenever a progress sink is present, because the live
		// subtree above is already the record. So success is simply the
		// checkmark, and only the failure branch carries an error and notes.
		if p.Success {
			fmt.Println("           " + symCheck)
		} else {
			fmt.Println("           " + symFail)
			fmt.Println("  Error")
			if p.Error != nil {
				fmt.Printf("    %v\n", p.Error)
			}
			for _, note := range p.Notes {
				fmt.Printf("    %s\n", note)
			}
		}
	}
}

func (r TerminalRenderer) Update(report UpdateReport) error {
	if len(report.Skipped) > 0 {
		fmt.Println()
		fmt.Println("Skipped")
		fmt.Println()
		for _, name := range report.Skipped {
			fmt.Printf("  %s %s\n", symBullet, name)
		}
		fmt.Println()
	}

	// Installation detail: show resolved version per tool, grouped by module.
	if len(report.UpdatedDetail) > 0 {
		fmt.Println("Installations")
		fmt.Println()
		// Group by module path for readability.
		moduleMap := make(map[string][]UpdatedToolDetail)
		for _, d := range report.UpdatedDetail {
			key := d.ModulePath
			if key == "" {
				key = "-"
			}
			moduleMap[key] = append(moduleMap[key], d)
		}
		// Sort modules deterministically.
		var modules []string
		for m := range moduleMap {
			modules = append(modules, m)
		}
		slices.Sort(modules)
		for _, m := range modules {
			details := moduleMap[m]
			// Order children within a module deterministically by display name;
			// grouping by module is otherwise input-order dependent.
			slices.SortFunc(details, func(a, b UpdatedToolDetail) int {
				return strings.Compare(a.Name, b.Name)
			})
			// Use the first detail's resolved version as the module header.
			// moduleMap already keyed every entry by its displayed module
			// header, substituting "-" for an empty ModulePath, so the key is
			// the header and this cannot disagree with the group title.
			first := details[0]
			fmt.Printf("  %s@%s\n", m, first.Resolved)
			for _, d := range details {
				prev := d.Previous
				if prev == "" {
					prev = "(current)"
				}
				fmt.Printf("    %s (%s → %s) %s\n", d.Name, prev, d.Resolved, d.PackagePath)
			}
		}
		fmt.Println()
	}

	if len(report.Diagnostics) > 0 {
		fmt.Println()
		fmt.Println("Diagnostics")
		fmt.Println()
		for _, d := range report.Diagnostics {
			fmt.Printf("  %s %s\n", symBullet, d.ToolName)
			fmt.Printf("  Category : %s\n", d.Category)
			fmt.Printf("  Message  : %s\n\n", d.Message)
		}
	}

	printSummaryBlock(updateSummaryRows(report))

	if err := report.Err(); err != nil {
		fmt.Println()
		fmt.Println("Failed tools:")
		// The cause is deliberately not repeated here: the terminal already
		// streamed it live, in context, under each installing tool.
		for _, f := range report.Failed {
			fmt.Printf("- %s\n", f)
		}
		return err
	}

	return nil
}

// summaryLabelWidth aligns every summary block across all commands so each
// renderer ends with the same visual rhythm: "Summary" then aligned values.
const summaryLabelWidth = 14

// columnWidths returns the display width of each column: the maximum byte
// length across all rows, floored element-wise by minimums. Callers pass one
// minimum per column and rows of exactly that many cells; the byte-length
// rule matches the historical %-Ns formatting exactly.
func columnWidths(minimums []int, rows ...[]string) []int {
	widths := append([]int(nil), minimums...)
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	return widths
}

// printInventorySummary renders the canonical inventory totals shared by
// the empty and non-empty inventory paths.
func printInventorySummary(summary InventorySummary) {
	printSummaryBlock([][2]string{
		{"Healthy", itoa(summary.Healthy)},
		{"Local", itoa(summary.Local)},
		{"Invalid", itoa(summary.Invalid)},
		{"Unhealthy", itoa(summary.Unhealthy)},
	})
}

// updateSummaryRows is the canonical update summary shared by the terminal
// and quiet renderers, which differ only in whether they print the "Summary"
// heading. Keeping one row list stops the two from drifting apart silently.
// The CI renderer has its own key: value form and is intentionally separate.
func updateSummaryRows(report UpdateReport) [][2]string {
	return [][2]string{
		{"Updated", itoa(len(report.Updated))},
		{"Up-to-date", itoa(len(report.UpToDate))},
		{"Skipped", itoa(len(report.Skipped))},
		{"Failed", itoa(len(report.Failed))},
		{"Duration", formatDuration(report.Duration)},
	}
}

func printSummaryLine(label, value string) {
	fmt.Printf("%-*s%s\n", summaryLabelWidth, label, value)
}

func printSummaryBlock(rows [][2]string) {
	fmt.Println("Summary")
	fmt.Println()
	for _, row := range rows {
		printSummaryLine(row[0], row[1])
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := d.Seconds() - float64(mins*60)
	return fmt.Sprintf("%dm%.1fs", mins, secs)
}

func (r TerminalRenderer) Info(loadRes tool.LoadResult, target string) error {
	t, err := LookupTool(loadRes, target)
	if err != nil {
		return err
	}
	fmt.Printf("Binary\n\n  %s\n\n", t.Name())
	fmt.Printf("Main Package Path\n\n  %s\n\n", t.PackagePath())
	fmt.Printf("Module Path\n\n  %s\n\n", t.ModulePath())
	fmt.Printf("Version\n\n  %s\n\n", t.Version())
	fmt.Printf("Go (Built with)\n\n  %s\n\n", t.GoVersion())
	fmt.Printf("Location\n\n  %s\n\n", t.Path())
	fmt.Printf("Can Update\n\n  %t\n", t.CanUpdate())
	return nil
}
