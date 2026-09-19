package app

import (
	"encoding/json"
	"fmt"

	"helm/internal/tool"
)

// JSONRenderer emits machine-readable JSON. It is an output renderer, not an
// operation. Arrays are always initialized (never null), ordering is
// deterministic, and human formatting never affects the JSON shape.
type JSONRenderer struct{}

func (JSONRenderer) Header(HeaderInfo) error { return nil }

func (JSONRenderer) Inventory(report InventoryReport) error {
	toolsRep := make([]ToolReport, 0, len(report.Tools))
	for _, t := range report.Tools {
		toolsRep = append(toolsRep, ToolReport{
			Name:        t.Name,
			Version:     t.Version,
			PackagePath: t.PackagePath,
			ModulePath:  t.ModulePath,
		})
	}
	if err := emitJSON(ListReport{
		OperationEnvelope: report.OperationEnvelope,
		Tools:             toolsRep,
	}); err != nil {
		return err
	}
	if report.Summary.Unhealthy > 0 || report.Summary.Invalid > 0 {
		return fmt.Errorf("%d issues found during inventory check", report.Summary.Unhealthy+report.Summary.Invalid)
	}
	return nil
}

func (JSONRenderer) Plan(report PlanReport) error {
	return emitJSON(report)
}

func (JSONRenderer) Outdated(report OutdatedReport) error {
	outReports := make([]OutdatedItemReport, 0, len(report.Results))
	for _, o := range report.Results {
		outReports = append(outReports, OutdatedItemReport{
			Name:     o.Name,
			Current:  o.Current,
			Latest:   o.Latest,
			Outdated: o.Outdated,
			Error:    o.Error,
		})
	}
	if err := emitJSON(OutdatedReport{
		OperationEnvelope: report.OperationEnvelope,
		Results:           outReports,
	}); err != nil {
		return err
	}
	// Process success agrees with resolution failures exactly like the human
	// renderers, while the JSON document on stdout stays complete.
	if report.Summary.Failed > 0 {
		return fmt.Errorf("%d outdated checks failed", report.Summary.Failed)
	}
	return nil
}

func (JSONRenderer) Update(report UpdateReport) error {
	if err := emitJSON(report); err != nil {
		return err
	}
	// Process success must agree with the operation failure state regardless
	// of renderer: a failed update exits non-zero exactly like the human
	// renderers, while the JSON document on stdout stays complete.
	if len(report.Failed) > 0 {
		return fmt.Errorf("%d updates failed", len(report.Failed))
	}
	return nil
}

func (JSONRenderer) Info(loadRes tool.LoadResult, target string) error {
	for _, t := range loadRes.Tools {
		if t.Name() == target {
			return emitJSON(ToolReport{
				Name:        t.Name(),
				Version:     t.Version(),
				PackagePath: t.PackagePath(),
				ModulePath:  t.ModulePath(),
			})
		}
	}
	return fmt.Errorf("tool '%s' not found or has no module metadata", target)
}

func emitJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode JSON output: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
