package tool

import (
	"errors"
	"fmt"
)

type InvalidBinary struct {
	Path  string
	Error error
}

// Message returns a stable, user-visible reason for why the binary could not
// be inspected. Error also wraps the underlying toolchain failure, so callers
// that need the cause (for example errors.Is on fs.ErrNotExist) can reach it;
// Message is the stable string renderers should present to users.
func (inv InvalidBinary) Message() string {
	switch {
	case errors.Is(inv.Error, ErrMissingBuildInfo):
		return ErrMissingBuildInfo.Error()
	case errors.Is(inv.Error, ErrMissingPackagePath):
		return ErrMissingPackagePath.Error()
	default:
		return "unable to inspect binary"
	}
}

// LoadSummary counts what discovery found. Executables is the total number of
// entries in GOBIN, so it includes the invalid binaries also counted
// separately in Invalid; the two are not additive.
type LoadSummary struct {
	Executables int
	Updatable   int
	Local       int
	Invalid     int
}

type LoadResult struct {
	Tools   []Tool
	Invalid []InvalidBinary
	Summary LoadSummary
}

func Load(gobin string) (LoadResult, error) {
	candidates, err := discover(gobin)
	if err != nil {
		return LoadResult{}, err
	}

	tools := make([]Tool, 0, len(candidates))
	invalids := make([]InvalidBinary, 0, len(candidates))
	var executables, updatable, local int

	for _, c := range candidates {
		t, err := inspect(c)
		if err != nil {
			// Both sentinels stay unwrappable: the classification sentinel
			// first for errors.Is, the underlying toolchain error second so
			// callers can still reach the real cause.
			invalids = append(invalids, InvalidBinary{
				Path:  c.path,
				Error: fmt.Errorf("%w: %w", ErrMissingBuildInfo, err),
			})
			continue
		}
		if !t.IsValid() {
			invalids = append(invalids, InvalidBinary{
				Path:  c.path,
				Error: ErrMissingPackagePath,
			})
			continue
		}
		executables++
		if t.CanUpdate() {
			updatable++
		} else {
			local++
		}
		tools = append(tools, t)
	}

	return LoadResult{
		Tools:   tools,
		Invalid: invalids,
		Summary: LoadSummary{
			Executables: executables + len(invalids),
			Updatable:   updatable,
			Local:       local,
			Invalid:     len(invalids),
		},
	}, nil
}
