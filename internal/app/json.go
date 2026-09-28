package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/divijg19/Helm/internal/tool"
)

// JSONRenderer emits machine-readable JSON. It is an output renderer, not an
// operation. Arrays are always initialized (never null), ordering is
// deterministic, and human formatting never affects the JSON shape.
type JSONRenderer struct{}

func (JSONRenderer) Header(HeaderInfo) error { return nil }

func (JSONRenderer) Inventory(report InventoryReport) error {
	// Emit the full inventory model, not the bare tool list: machine
	// consumers need the same status, invalid-binary, and summary detail
	// the human renderers show to reconcile a failed inventory.
	//
	// The never-null array contract is enforced at this wire boundary rather
	// than trusted from the report builder, for the same reason as Outdated.
	if report.Tools == nil {
		report.Tools = []ToolInventoryItem{}
	}
	if err := emitJSON(report); err != nil {
		return err
	}
	return report.Err()
}

func (JSONRenderer) Plan(report PlanReport) error {
	return emitJSON(report)
}

func (JSONRenderer) Outdated(report OutdatedReport) error {
	// The never-null array contract is enforced here, at the wire boundary,
	// rather than trusted from the report builder: this renderer is reachable
	// with a directly constructed report, and a machine consumer must never
	// have to handle a null where an array is documented.
	results := report.Results
	if results == nil {
		results = []OutdatedItemReport{}
	}
	if err := emitJSON(OutdatedReport{
		OperationEnvelope: report.OperationEnvelope,
		Results:           results,
	}); err != nil {
		return err
	}
	// Process success agrees with resolution failures exactly like the human
	// renderers, while the JSON document on stdout stays complete.
	return report.Err()
}

func (JSONRenderer) Update(report UpdateReport) error {
	if err := emitJSON(report); err != nil {
		return err
	}
	// Process success must agree with the operation failure state regardless
	// of renderer: a failed update exits non-zero exactly like the human
	// renderers, while the JSON document on stdout stays complete.
	return report.Err()
}

func (JSONRenderer) Info(loadRes tool.LoadResult, target string) error {
	t, err := LookupTool(loadRes, target)
	if err != nil {
		return err
	}
	return emitJSON(ToolReport{
		Name:        t.Name(),
		Version:     t.Version(),
		PackagePath: t.PackagePath(),
		ModulePath:  t.ModulePath(),
	})
}

// emitJSON writes one JSON document followed by a newline. A streaming
// encoder is used rather than MarshalIndent so the document is never copied
// into an intermediate string; the output is byte-identical, since both
// escape HTML by default and Encode appends the trailing newline.
func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("failed to encode JSON output: %w", err)
	}
	return nil
}
