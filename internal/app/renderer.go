package app

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/divijg19/Helm/internal/tool"
)

// RenderMode selects the concrete renderer via NewRenderer.
type RenderMode int

const (
	// ModeTerminal is the default human-oriented renderer (Unicode, progress).
	ModeTerminal RenderMode = iota
	// ModeQuiet suppresses the discovery header and live progress. Only the
	// update operation is summary-only: inventory, plan, outdated, and info
	// delegate to TerminalRenderer, so their tables and summaries still appear.
	ModeQuiet
	// ModeCI produces deterministic, ASCII-only, line-oriented terminal output.
	ModeCI
	// ModeJSON emits machine-readable JSON only.
	ModeJSON
)

// Renderer is the single output abstraction for every operation.
// Business logic never knows how output is formatted.
type Renderer interface {
	// Header renders the discovery header. Renderers that fold header
	// information into a machine format (JSON) or hide it (quiet) no-op.
	Header(hdr HeaderInfo) error
	Inventory(report InventoryReport) error
	Plan(report PlanReport) error
	Outdated(report OutdatedReport) error
	Update(report UpdateReport) error
	Info(loadRes tool.LoadResult, target string) error
}

// ProgressSink is implemented by renderers that stream per-tool progress
// while an update runs. The App probes the renderer for this interface and
// only wires live progress when it is present; TerminalRenderer is currently
// the sole implementation.
type ProgressSink interface {
	OnProgress(p tool.Progress)
}

// NewRenderer returns the renderer matching the requested mode. verbose
// selects the detailed planning view (packages + commands) for human
// renderers; it is ignored by JSON and quiet modes.
func NewRenderer(mode RenderMode, verbose bool) Renderer {
	switch mode {
	case ModeQuiet:
		return QuietRenderer{}
	case ModeCI:
		return CIRenderer{verbose: verbose}
	case ModeJSON:
		return JSONRenderer{}
	default:
		return TerminalRenderer{verbose: verbose}
	}
}

// LookupTool returns the loaded tool with the given name, or an error when
// no loaded tool matches. All Info renderers share this lookup so the miss
// behavior and message stay identical across output modes; the CLI also uses
// it to reject unknown --info targets before rendering anything.
func LookupTool(loadRes tool.LoadResult, target string) (tool.Tool, error) {
	for _, t := range loadRes.Tools {
		if t.Name() == target {
			return t, nil
		}
	}
	return tool.Tool{}, fmt.Errorf("tool '%s' not found or has no module metadata", target)
}

// HeaderInfo carries the discovery context shown by human renderers.
type HeaderInfo struct {
	Gobin     string
	GoVersion string // may be empty when go env GOVERSION cannot be read
	LoadRes   tool.LoadResult
}

// Operation identifiers carried by every operation report. These values are
// stable across the 1.x and 2.x series; the JSON renderer emits them, human
// renderers ignore them. --dry-run reports the same operation as --check.
const (
	OperationList     = "list"
	OperationCheck    = "check"
	OperationUpdate   = "update"
	OperationOutdated = "outdated"
)

// OperationEnvelope is the machine-readable prefix every operation report
// carries. Human renderers never read it; the JSON renderer serializes it.
type OperationEnvelope struct {
	Operation string `json:"operation"`
	Success   bool   `json:"success"`
}

type InventoryReport struct {
	OperationEnvelope
	Tools   []ToolInventoryItem `json:"tools"`
	Invalid []InvalidReport     `json:"invalid,omitempty"`
	Summary InventorySummary    `json:"summary"`
}

// Inventory status vocabulary. These are the only values inventoryReport can
// assign to ToolInventoryItem.Status; invalid binaries are reported through
// the separate Invalid list and never carry a status.
const (
	statusHealthy   = "Healthy"
	statusLocal     = "Local"
	statusUnhealthy = "Unhealthy"
)

type ToolInventoryItem struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PackagePath string `json:"package_path"`
	ModulePath  string `json:"module_path,omitempty"`
	Status      string `json:"status"` // "Healthy", "Local", "Unhealthy"
	Error       string `json:"error,omitempty"`
}

type InventorySummary struct {
	Healthy   int `json:"healthy"`
	Local     int `json:"local"`
	Invalid   int `json:"invalid"`
	Unhealthy int `json:"unhealthy"`
}

type ToolReport struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PackagePath string `json:"package_path"`
	ModulePath  string `json:"module_path"`
}

type InvalidReport struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type OutdatedReport struct {
	OperationEnvelope
	Results []OutdatedItemReport `json:"results"`
	Summary OutdatedSummary      `json:"-"`
}

type OutdatedItemReport struct {
	Name     string `json:"name"`
	Current  string `json:"current"`
	Latest   string `json:"latest,omitempty"`
	Outdated bool   `json:"outdated"`
	Error    string `json:"error,omitempty"`
}

// OutdatedSummary is the human and CI view model for an outdated report. It is
// deliberately excluded from JSON (OutdatedReport.Summary is `json:"-"`), so
// the tags below never reach the wire and machine consumers must derive these
// counts from the result array instead.
type OutdatedSummary struct {
	// Total is the number of tools checked, so Checked == Outdated + UpToDate
	// + Failed can be asserted rather than recomputed per renderer.
	Total    int `json:"total"`
	Outdated int `json:"outdated"`
	UpToDate int `json:"up_to_date"`
	Failed   int `json:"failed"`
}

type UpdatedToolDetail struct {
	Name        string
	PackagePath string
	ModulePath  string
	Previous    string
	Resolved    string
}

// ToolFailure records one failed install and the reason it failed. Failed
// carries the tool names for every renderer; this carries the causes, so a
// script reading --json or --ci learns why an install failed rather than only
// that it did.
type ToolFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type UpdateReport struct {
	OperationEnvelope
	Updated       []string            `json:"updated"`
	UpToDate      []string            `json:"up_to_date"`
	UpdatedDetail []UpdatedToolDetail `json:"-"`
	Notes         []string            `json:"notes,omitempty"`
	Skipped       []string            `json:"skipped"`
	Failed        []string            `json:"failed"`
	FailedDetail  []ToolFailure       `json:"failed_detail,omitempty"`
	Duration      time.Duration       `json:"-"`
	Diagnostics   []tool.Diagnostic   `json:"-"`
}

// countPhrase renders "<n> <noun>" with the noun agreeing in number, so
// user-facing failure messages read correctly for a single problem instead of
// saying "1 issues found".
func countPhrase(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}

// Err is the single success/failure decision for an inventory report. Every
// renderer returns this error, so the exit code, the stderr message, and the
// JSON success flag can never disagree about whether an operation failed.
func (r InventoryReport) Err() error {
	n := r.Summary.Unhealthy + r.Summary.Invalid
	if n == 0 {
		return nil
	}
	return errors.New(countPhrase(n, "issue", "issues") + " found during inventory check")
}

// Err is the single success/failure decision for an outdated report.
func (r OutdatedReport) Err() error {
	if r.Summary.Failed == 0 {
		return nil
	}
	return errors.New(countPhrase(r.Summary.Failed, "outdated check", "outdated checks") + " failed")
}

// Err is the single success/failure decision for an update report.
func (r UpdateReport) Err() error {
	if len(r.Failed) == 0 {
		return nil
	}
	return errors.New(countPhrase(len(r.Failed), "update", "updates") + " failed")
}

// PlanReport is the single unified planning operation report produced by
// --check and --dry-run (aliases). Verbosity only affects human rendering; the
// JSON shape is identical regardless of --verbose.
type PlanReport struct {
	OperationEnvelope
	WouldUpdate []PlanItem `json:"would_update"`
	Skipped     []PlanItem `json:"skipped"`
}

type PlanItem struct {
	Name          string `json:"name"`
	PackagePath   string `json:"package_path,omitempty"`
	InstallTarget string `json:"install_target,omitempty"`
	Command       string `json:"command,omitempty"`
}
