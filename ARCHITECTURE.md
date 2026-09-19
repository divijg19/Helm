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
    └── update (selection → outdated resolution → exact-version install)
    ↓
rendering
    ↓
exit
```

The default update operation is outdated-first: it resolves the selected
updatable tools with the same bounded-concurrency outdated check that backs
`--outdated`, then installs only tools proven outdated, each at the exact
version the check resolved (`go install <package>@<resolved>`). Discovery
and eligibility identify tools Helm can manage; only a successful outdated
check authorizing a newer version permits mutation. There is no outdated
cache: each CLI invocation constructs a fresh `App` and performs one
operation, so update resolves within its own invocation. Installation stays
sequential while outdated resolution stays bounded-concurrent.

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
- Discovery is sorted before reports are produced; the installation tree
  additionally groups by module and sorts modules and children by name.
- `--check` and `--dry-run` share one planning path.
- Tool-name filters apply to updates and plans; `--list` and `--outdated`
  take no tool names and `--info` takes exactly one. Unknown names are
  usage errors, reported before any output.
- Flags an invocation discards (plan flags with explicit operations,
  `--verbose` with JSON/quiet output, shadowed output modes) print a
  `Warning:` to stderr without changing the outcome. Output modes resolve
  by precedence `--json` over `--ci` over `--quiet`.
- `Plan` owns update selection; renderers do not re-derive it.
- `InstallRef` and `InstallCommand` describe the update rule shown by
  `--check`/`--dry-run` for eligible tools (`<package>@latest`); executed
  installs pin the resolved version via `InstallExactRef`
  (`<package>@<resolved>`), so the version evaluated is the version installed.
- A tool is installed only when fresh outdated resolution reports
  `Outdated == true` with no error and a usable resolved version; resolution
  errors, retractions, cancellations, and unselected tools never authorize
  installation.
- JSON reports use stable operation names and empty arrays instead of `null`.
  `--info` is the exception: it emits a bare tool report with no operation
  envelope.
- Inventory counts reconcile: every discovered tool is Healthy, Local, or
  Unhealthy, with invalid binaries listed separately. `Skipped` means the
  same in plans and updates: selected but ineligible tools plus invalid
  binaries. Human and CI renderers use stable report ordering and summary structure.
- Environment-resolution errors wrap `tool.ErrGobinResolution` and map to
  `ExitEnv`; generic application failures map to `ExitFailure`.

## Rendering

The renderer interface covers headers and operation reports for inventory,
planning, outdated checks, updates, and tool information. Terminal, JSON,
quiet, and CI renderers implement presentation without owning business rules.

JSON is available for every operation. Its stable operation names are `list`,
`check`, `update`, and `outdated`.
