# Architecture

This document describes Helm's durable structure and current invariants.

## Package Layout

```text
cmd/helm/         Canonical executable entrypoint.
internal/cli/     Invocation resolution, flags, operations, and exit codes.
internal/app/     Application orchestration and renderer coordination.
internal/tool/    Discovery, inspection, planning, updates, and outdated checks.
internal/testutil/ Hermetic fixture and offline test support.
```

## Execution Flow

```text
invocation
    ↓
ResolveInvocation
    ↓
environment resolution
    ↓
App construction
    ↓
tool discovery and loading
    ↓
operation
    ├── list
    ├── plan (--check / --dry-run)
    ├── outdated
    └── update
    ↓
rendering
    ↓
exit
```

The supported invocation names are `helm`, `Helm`, and `update-go-tools`.
They all execute through `cmd/helm`; there are no alias-specific executable
implementations.

## Boundaries

The CLI owns process-facing concerns: invocation names, flags, renderer mode,
and exit-code mapping. The application package sequences operations and passes
reports to renderers. The tool package owns domain behavior and does not know
about terminal formatting.

`Runner` isolates external `go` commands and carries context cancellation into
subprocesses. `Renderer` isolates terminal, JSON, quiet, and CI presentation
from domain logic.

`App` memoizes its loaded inventory for the lifetime of one application
instance. The memoization is invocation-local, not a persistent cache.

## Invariants

- There is one executable implementation in `cmd/helm`.
- Discovery is sorted before reports are produced.
- `--check` and `--dry-run` share one planning path.
- `Plan` owns update selection; renderers do not re-derive it.
- `InstallRef` and `InstallCommand` describe the same update command.
- JSON reports use stable operation names and empty arrays instead of `null`.
- Human and CI renderers use stable report ordering and summary structure.
- Environment-resolution errors wrap `tool.ErrGobinResolution` and map to
  `ExitEnv`; generic application failures map to `ExitFailure`.

## Rendering

The renderer interface covers headers and operation reports for inventory,
planning, outdated checks, updates, and tool information. Terminal, JSON,
quiet, and CI renderers implement presentation without owning business rules.

JSON is available for every operation. Its stable operation names are `list`,
`check`, `update`, and `outdated`.
