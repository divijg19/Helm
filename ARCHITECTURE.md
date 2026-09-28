# Architecture

This document describes Helm's durable structure and current invariants.

## Package Layout

```text
cmd/helm/         Canonical executable entrypoint.
internal/cli/     Flag parsing, operation dispatch, and exit codes.
internal/app/     Application orchestration and renderer coordination.
internal/tool/    Discovery, inspection, planning, updates, and outdated checks.
internal/testutil/ Hermetic fixture and offline test support.
```

## Execution Flow

```text
invocation (the invoked basename is not inspected; every alias behaves alike)
    ↓
flag parsing → --help / --version answer here (before the toolchain is touched)
    ↓
environment resolution (GOBIN/GOPATH) → App construction
    ↓
discarded-flag warnings (stderr only)
    ↓
tool discovery and loading
    ↓
usage validation (unknown filters, positional scope) → ExitUsage
    ↓
--info target resolution (operational failure) → ExitFailure
    ↓
discovery header (suppressed by --json / --quiet)
    ↓
operation
    ├── list
    ├── plan (--check / --dry-run)
    ├── outdated
    ├── info (exactly one target)
    └── update (selection → outdated resolution → exact-version install)
    ↓
report rendering → operation Err() → exit code
```

`--help` and `--version` are answered before environment resolution on
purpose: they must keep working in exactly the broken-toolchain state a user
would reach for them in. Everything after flag parsing needs a working `go` on
`PATH`, so a resolution failure there is `ExitEnv` (3) rather than a usage
error. Validation sits between loading and rendering so a rejected invocation
never produces partial output.

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
They are all symlinks or copies of the one binary and are not distinguished:
the CLI never reads the invoked basename, so every name must behave
identically. `TestAliasUpdateGoToolsMatchesHelm` and
`TestAliasUpperCaseHelmMatchesHelm` in `cmd/helm` guard that, so per-name
dispatch cannot be reintroduced unnoticed.

## Boundaries

The CLI owns process-facing concerns: flags, renderer mode, and exit-code
mapping. The application package sequences operations and passes
reports to renderers. The tool package owns domain behavior and does not know
about terminal formatting.

`Runner` isolates external `go` commands and carries context cancellation into
subprocesses. `Renderer` isolates terminal, JSON, quiet, and CI presentation
from domain logic.

`App` memoizes its loaded inventory for the lifetime of one application
instance. The memoization is invocation-local, not a persistent cache.

## Invariants

- There is one executable implementation in `cmd/helm`.
- Discovery is sorted before reports are produced; the terminal
  installation tree additionally groups by module and sorts modules and
  children by name (CI, quiet, and JSON render the same installations
  flat or not at all).
- Discovery scans a single GOBIN level: subdirectories are skipped, file
  symlinks are followed, and a symlink to a directory lands in Invalid
  rather than being traversed. Entry names cannot escape the directory.
- `--check` and `--dry-run` share one planning path.
- Filter scope and discarded-flag warnings follow the CLI dispatch contract
  (`internal/cli/cli.go`); README and `--help` carry the user wording.
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
  Unhealthy, so those three always sum to the number of loaded tools. (The
  header's `Executables` is larger: it counts every GOBIN entry, including the
  invalid binaries, so the two totals are deliberately not additive.)
  Invalid binaries are counted only in `Summary.Invalid` and are never folded
  into `Unhealthy`; renderers add `Unhealthy + Invalid` for the issue total, so
  each problem is counted exactly once. `Skipped` means the same in plans and
  updates: selected but ineligible tools plus invalid binaries, and both
  operations build that list with one shared helper. Human and CI renderers
  use stable report ordering and summary structure.
- Environment-resolution errors wrap `tool.ErrGobinResolution` and map to
  `ExitEnv`; generic application failures map to `ExitFailure`. `--help` and
  `--version` precede environment resolution entirely.

## Rendering

The renderer interface covers headers and operation reports for inventory,
planning, outdated checks, updates, and tool information. Terminal, JSON,
quiet, and CI renderers implement presentation without owning business rules.

Progress streams the toolchain's own output under the installing tool,
unmodified apart from surrounding whitespace and with blank lines omitted.
Only TerminalRenderer implements ProgressSink, so only the terminal emits it
live; because the live subtree is the record, installTool drops the captured
lines from a successful completion event instead of repeating them. The
report-level `notes` field is a list of tool *names* that were successfully
updated and produced output (CI prints them as `note: <name>`, JSON as a
string array, quiet omits them), never a copy of the text; the terminal
prints no notes section at all. A failed install keeps its error and captured
output only in the terminal's live stream. Fetch events are never filtered
and never classified as diagnostics.

JSON is available for every operation. Its stable operation names are `list`,
`check`, `update`, and `outdated`.

Machine consumers should note what JSON omits by design: the outdated
summary and the update-only `UpdatedDetail`, `Diagnostics`, and `Duration`
fields are excluded from serialization, so outdated counts must be derived
from the result arrays and `success` flags rather than expected as JSON keys.
`Notes` and `FailedDetail` are `omitempty`, so a clean update emits neither
key. Inventory reports serialize in full, including per-tool status and summary.
`--info` is narrower than the human view on purpose: `ToolReport` carries only
`name`, `version`, `package_path`, and `module_path`, while the terminal and
CI renderers additionally show the Go version the tool was built with, its
install path, and whether it can be updated. Do not expect `go-version`,
`path`, or `can-update` keys from `--info --json`.

Update failures are reported at two levels so no mode loses the reason:
`failed` lists every tool whose install did not succeed, and
`failed_detail` pairs each such tool with its error. The CI renderer prints
the pair as `failed: <name>` plus `failed-reason: <name>: <error>`, quiet
prints one combined `failed: <name>: <error>` line on stderr, and JSON
carries the array. The terminal deliberately does not repeat the cause,
because it already streamed it live under the installing tool. Resolution
failures are a separate class and arrive as `diagnostics` instead.

Every fallible operation has exactly one success/failure decision, expressed
as `Err()` on the report type (`InventoryReport`, `OutdatedReport`,
`UpdateReport`). All four renderers return that error, and the report
builders derive the JSON `success` flag from the same condition, so the
exit code, the stderr message, and `success` cannot disagree. Planning is
the exception: it executes no subprocess and cannot fail, so `PlanReport`
has no `Err()` and its `success` is unconditionally true.
