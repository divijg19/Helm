package app

import (
	"context"
	"time"

	"helm/internal/tool"
)

type App struct {
	Gobin        string
	Renderer     Renderer
	Runner       tool.Runner
	loadRes      tool.LoadResult
	loadResValid bool
}

// gobinResolver resolves the GOBIN directory. It is a package-private seam so
// tests can deterministically exercise GOBIN/GOPATH resolution failure
// propagation through the real NewApp path without invoking the toolchain.
// Production behavior is unchanged: it defaults to tool.GetGobin.
var gobinResolver = tool.GetGobin

func NewApp(renderer Renderer, runner tool.Runner) (*App, error) {
	gobin, err := gobinResolver()
	if err != nil {
		return nil, err
	}
	return &App{
		Gobin:    gobin,
		Renderer: renderer,
		Runner:   runner,
	}, nil
}

func (a *App) load() (tool.LoadResult, error) {
	if a.loadResValid {
		return a.loadRes, nil
	}
	res, err := tool.Load(a.Gobin)
	if err != nil {
		return tool.LoadResult{}, err
	}
	a.loadRes = res
	a.loadResValid = true
	return res, nil
}

func (a *App) RunInventory() error {
	loadRes, err := a.load()
	if err != nil {
		return err
	}
	report := a.inventoryReport(loadRes)
	return a.Renderer.Inventory(report)
}

func (a *App) inventoryReport(loadRes tool.LoadResult) InventoryReport {
	verifyResults := tool.Verify(loadRes.Tools)
	verifyMap := make(map[string]tool.VerificationResult)
	for _, vr := range verifyResults {
		verifyMap[vr.Tool.Name()] = vr
	}

	var items []ToolInventoryItem
	healthy := 0
	localCount := 0
	unhealthy := 0

	for _, t := range loadRes.Tools {
		vr, ok := verifyMap[t.Name()]
		status := "Healthy"
		errStr := ""
		if ok && !vr.Healthy {
			status = "Unhealthy"
			errStr = vr.Error
			unhealthy++
		} else if !t.CanUpdate() {
			status = "Local"
			localCount++
			healthy++
		} else {
			healthy++
		}

		items = append(items, ToolInventoryItem{
			Name:        t.Name(),
			Version:     t.Version(),
			PackagePath: t.PackagePath(),
			ModulePath:  t.ModulePath(),
			Status:      status,
			Error:       errStr,
		})
	}

	for range loadRes.Invalid {
		unhealthy++
	}

	return InventoryReport{
		OperationEnvelope: OperationEnvelope{
			Operation: OperationList,
			Success:   unhealthy == 0 && len(loadRes.Invalid) == 0,
		},
		Tools:   items,
		Invalid: a.invalidReports(loadRes.Invalid),
		Summary: InventorySummary{
			Healthy:   healthy,
			Local:     localCount,
			Invalid:   len(loadRes.Invalid),
			Unhealthy: unhealthy,
		},
	}
}

func (a *App) invalidReports(invalids []tool.InvalidBinary) []InvalidReport {
	var reps []InvalidReport
	for _, inv := range invalids {
		reps = append(reps, InvalidReport{
			Path:    inv.Path,
			Message: inv.Message(),
		})
	}
	return reps
}

func (a *App) RunOutdated(ctx context.Context) error {
	loadRes, err := a.load()
	if err != nil {
		return err
	}
	outdatedRes := tool.CheckOutdated(ctx, loadRes.Tools, a.Runner)
	report := a.outdatedReport(outdatedRes)
	return a.Renderer.Outdated(report)
}

func (a *App) outdatedReport(outdatedRes []tool.OutdatedResult) OutdatedReport {
	outReports := make([]OutdatedItemReport, 0, len(outdatedRes))
	outdatedCount := 0
	upToDateCount := 0
	failedCount := 0

	for _, o := range outdatedRes {
		errStr := ""
		if o.Error != nil {
			errStr = o.Error.Error()
		}
		// Resolution failures are their own category: a tool whose update
		// state could not be determined is neither outdated nor up-to-date.
		switch {
		case o.Error != nil:
			failedCount++
		case o.Outdated:
			outdatedCount++
		default:
			upToDateCount++
		}
		outReports = append(outReports, OutdatedItemReport{
			Name:     o.Tool.Name(),
			Current:  o.Current,
			Latest:   o.Latest,
			Outdated: o.Outdated,
			Error:    errStr,
		})
	}

	return OutdatedReport{
		OperationEnvelope: OperationEnvelope{
			Operation: OperationOutdated,
			Success:   failedCount == 0,
		},
		Results: outReports,
		Summary: OutdatedSummary{
			Outdated: outdatedCount,
			UpToDate: upToDateCount,
			Failed:   failedCount,
		},
	}
}

func (a *App) RunInfo(target string) error {
	loadRes, err := a.load()
	if err != nil {
		return err
	}
	return a.Renderer.Info(loadRes, target)
}

func (a *App) LoadTools() (tool.LoadResult, error) {
	return a.load()
}

func (a *App) RunPlan(ctx context.Context, args []string) error {
	loadRes, err := a.load()
	if err != nil {
		return err
	}
	plan := tool.Plan(loadRes, args)
	report := a.planReport(plan)
	return a.Renderer.Plan(report)
}

func (a *App) planReport(plan tool.PlanResult) PlanReport {
	toUpdate := make([]PlanItem, 0, len(plan.ToUpdate))
	for _, t := range plan.ToUpdate {
		toUpdate = append(toUpdate, PlanItem{
			Name:          t.Name(),
			PackagePath:   t.PackagePath(),
			InstallTarget: t.InstallTarget(),
			Command:       tool.InstallCommand(t.InstallTarget()),
		})
	}

	skipped := make([]PlanItem, 0, len(plan.Skipped)+len(plan.Invalid))
	for _, t := range plan.Skipped {
		skipped = append(skipped, PlanItem{Name: t.Name()})
	}
	for _, inv := range plan.Invalid {
		skipped = append(skipped, PlanItem{Name: inv.Path})
	}

	return PlanReport{
		OperationEnvelope: OperationEnvelope{
			Operation: OperationCheck,
			Success:   true,
		},
		WouldUpdate: toUpdate,
		Skipped:     skipped,
	}
}

func (a *App) RunUpdate(ctx context.Context, args []string) error {
	loadRes, err := a.load()
	if err != nil {
		return err
	}

	var onProgress func(tool.Progress)
	if termRend, ok := a.Renderer.(interface{ OnProgress(tool.Progress) }); ok {
		onProgress = termRend.OnProgress
	}

	// Outdated-first update: resolve the selected updatable tools, then install
	// only the candidates a successful outdated check authorized. Resolution
	// failures veto installation; they are reported as failures (preserving
	// the existing non-zero exit behavior) rather than silent skips.
	set := tool.ResolveUpdateCandidates(ctx, loadRes.Tools, args, a.Runner)

	results, duration, diagnostics := tool.UpdateCandidates(ctx, set.Candidates, a.Runner, onProgress)
	report := a.updateReport(results, loadRes, set, duration, diagnostics)
	return a.Renderer.Update(report)
}

func (a *App) updateReport(results []tool.ToolUpdateResult, loadRes tool.LoadResult, set tool.CandidateSet, duration time.Duration, diagnostics []tool.Diagnostic) UpdateReport {
	updated := make([]string, 0)
	notes := make([]string, 0)
	failed := make([]string, 0)
	updatedDetail := make([]UpdatedToolDetail, 0)

	for _, res := range results {
		if res.Success {
			updated = append(updated, res.Tool.Name())
			if len(res.Notes) > 0 {
				notes = append(notes, res.Tool.Name())
			}
		} else {
			failed = append(failed, res.Tool.Name())
		}
	}

	upToDate := make([]string, 0, len(set.UpToDate))
	for _, t := range set.UpToDate {
		upToDate = append(upToDate, t.Name())
	}

	for _, r := range set.Failed {
		failed = append(failed, r.Tool.Name())
		diagnostics = append(diagnostics, tool.Diagnostic{
			ToolName: r.Tool.Name(),
			Category: "Outdated",
			Message:  r.Error.Error(),
		})
	}

	// Index install results by tool name once so each candidate lookup is O(1)
	// instead of a linear scan over the entire result set per candidate.
	// Tool names are unique by construction (one entry per GOBIN file through
	// discovery, preserved 1:1 through resolution and installation), so each
	// name maps to exactly one result in practice. The first result wins on
	// duplicates to preserve the legacy first-match behavior.
	resultByName := make(map[string]tool.ToolUpdateResult, len(results))
	for _, r := range results {
		if _, exists := resultByName[r.Tool.Name()]; !exists {
			resultByName[r.Tool.Name()] = r
		}
	}

	// Build installation detail per resolved candidate whose install succeeded.
	// Each entry records the tool's previous installed version, the resolved
	// version, and its paths. Failed installs stay confined to the failure
	// reporting path above and must not surface as installations.
	for _, c := range set.Candidates {
		res, ok := resultByName[c.Tool.Name()]
		if !ok || !res.Success {
			continue
		}
		// Pre-resolution version cannot be inferred from result alone; we use
		// the tool's Version() field which reflects the installed binary version.
		updatedDetail = append(updatedDetail, UpdatedToolDetail{
			Name:        c.Tool.Name(),
			PackagePath: c.Tool.PackagePath(),
			ModulePath:  c.Tool.ModulePath(),
			Previous:    res.Tool.Version(),
			Resolved:    c.Version,
		})
	}

	skipped := make([]string, 0, len(loadRes.Invalid))
	for _, inv := range loadRes.Invalid {
		skipped = append(skipped, inv.Path)
	}

	return UpdateReport{
		OperationEnvelope: OperationEnvelope{
			Operation: OperationUpdate,
			Success:   len(failed) == 0,
		},
		Updated:       updated,
		UpToDate:      upToDate,
		UpdatedDetail: updatedDetail,
		Notes:         notes,
		Skipped:       skipped,
		Failed:        failed,
		Duration:      duration,
		Diagnostics:   diagnostics,
	}
}
