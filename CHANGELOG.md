# Changelog

All notable changes to Helm are documented here.

## [Unreleased]

## [v1.7.0]

### Changed

* Added bounded concurrency to `CheckOutdated` so independent `go list -m -json
  <module>@latest` checks execute in parallel with a small internal worker bound
  (default 4). This materially reduces wall-clock latency when checking many
  installed tools.
* The normal Helm execution path now uses the bounded concurrent implementation;
  concurrency is an internal optimization, not a user-facing option.
* Preserved the existing `CheckOutdated` contract: deterministic result ordering
  by discovered tool position, per-tool error attachment, and context
  cancellation propagation. No CLI flags, no persistent caching, and no Update
  concurrency were introduced.

### Deferred

* Update concurrency, persistent result caching, and Load/Verify
  parallelization remain separate investigations for later v1.7.x work.

## [v1.6.10]

### Changed

* Reconciled the supported invocation contract to `helm`, `Helm`, and
  `update-go-tools` through the single canonical `cmd/helm` entrypoint.
* Completed the v1.6 baseline documentation audit and consolidated engineering
  investigations outside the product documentation surface.
* Added coverage for environment-resolution errors mapping to `ExitEnv` (3)
  while generic application failures remain `ExitFailure` (1).

### Deferred

* Performance optimization, concurrency, and persistent caching remain v1.7+
  work.

## [v1.6.9]

### Changed

* Finalized the v1.6 performance measurement baseline and verification
  workflow.

### Deferred

* Concurrency and caching work remained deferred to v1.7.0.

## [v1.6.8]

### Changed

* Prepared Helm for v1.7 performance work with benchmark and profiling
  infrastructure.

### Deferred

* Performance implementation remained deferred to v1.7.0.

## [v1.6.7]

### Changed

* Added real propagation coverage for GOBIN/GOPATH resolution failures through
  `GetGobin`, `NewApp`, and `cli.Run`.

### Deferred

* Environment-resolution failures still used the generic failure exit code at
  this release point; the `ExitEnv` contract was finalized in v1.6.10.

## [v1.6.6]

### Changed

* Added deterministic, hermetic coverage for GOBIN/GOPATH resolution and its
  failure paths (`internal/tool/gobin.go`): GOBIN precedence, GOPATH fallback,
  GOBIN-query failure fallback, empty GOPATH error, and `go env GOPATH` failure.
  An internal test seam was introduced without changing the public `GetGobin()`
  API or any runtime behavior.
* Verified the current failure-propagation contract: a GOBIN/GOPATH resolution
  failure exits with the generic `ExitFailure` (1), unchanged.

### Deferred

* `ExitEnv = 3` remains defined but unused. Whether environment/configuration
  resolution failures should map to `ExitEnv` is an open semantic question and
  is intentionally not activated in this commit.

## [v1.6.5]

### Changed

* Consolidated the executable entrypoints into a single canonical `cmd/helm`
  implementation. `helm`, `Helm`, and the compatibility alias `update-go-tools`
  are now first-class invocation names resolved from the process basename at the
  CLI boundary (`internal/cli.ResolveInvocation`), not separate `main` packages.
* Removed the duplicate `cmd/update-go-tools` entrypoint. Aliases are exposed by
  installing the one binary under the appropriate name.

## [v1.6.0]

### Changed

* Renamed and rebranded from `update-go-tools` to `Helm`.
  The binary is now `helm`; `update-go-tools` is preserved as a
  first-class alias that delegates to the same entrypoint.
* Module path changed from `update-go-tools` to `helm`.
* Import paths updated from `update-go-tools/internal/...` to
  `helm/internal/...`.
* Primary CLI entrypoint moved to `cmd/helm`; `cmd/update-go-tools`
  is the alias entrypoint.
* App name in output changed from `update-go-tools` to `Helm`.
* Version bumped to v1.6.0.

## [v1.5.0]

### Changed

* CLI consolidation, UX consistency & interface finalization:
  * `--list` is now the single canonical inventory command and absorbs the
    health reporting formerly exposed by `--verify`. It reports tool, version,
    health status (`Healthy`/`Local`/`Unhealthy`/`Invalid`), and package.
  * `--verify` is removed as an independent implementation; `--list` covers
    verification.
  * `--check` and `--dry-run` are aliases of a single planning operation with
    one execution path. A new `--verbose`/`-V` output flag switches between the
    concise summary (default) and the detailed execution plan
    (packages + commands).
  * New `--quiet`/`-q` renderer for scripting: suppresses banner, discovery
    summary, progress, and per-tool status; emits only
    `Updated`/`Skipped`/`Failed`/`Duration` plus diagnostics and failures.
  * New `--ci` renderer: deterministic, ASCII-only, line-oriented terminal
    output with no ANSI, no Unicode, no progress renderer, and no cursor
    movement.
  * `--json` is now a pure output renderer available on every operation with a
    single stable schema; arrays are never `null` and ordering is deterministic.
    Every response carries a JSON envelope with frozen `operation` values
    (`list`, `check`, `update`, `outdated`; `--dry-run` reports `check`) and a
    `success` boolean. No CLI version or timestamps are embedded.
  * The discovery header (`Go:`, `Discovery`) moved from the CLI into renderers
    so every operation follows `Go → Discovery → Body → Summary`.
  * `Renderer` interface replaced `Verify`/`Check`/`DryRun` with a unified
    `Plan` report and `Header`; concrete renderers are `Terminal`, `Quiet`,
    `CI`, and `JSON`.
  * Unified symbol set (`✓`, `•`, `✗`, `↑`, `ⓘ`) and summary-block formatting
    across all human output; CI uses an ASCII equivalent.
  * Final presentation polish: every command follows the canonical
    `Go → Discovery → Body → Summary` rhythm, the `Scanning` action label was
    replaced with a `Discovery` header, all summary blocks are visually
    identical (aligned 14-character labels), and the plan command now ends
    with the same `Summary` block instead of a duplicated `Would update: N`
    count.
  * `--quiet`/`-q` works on every operation (not just update), suppressing the
    discovery header while keeping the requested data and summary.
  * Help output groups output flags under `Output modifiers`.

### Polish

* Release-polish pass (engineering certification) before the v1.5.x freeze:
  * Planning is now owned by the domain layer (`tool.Plan`); the app layer no
    longer re-derives the update plan, and update/plan filtering share one
    implementation.
  * `tool.InstallCommand`/`tool.InstallRef` are the single source of truth for
    the `go install <target>@latest` command, so the plan always displays
    exactly what execution would run.
  * `TerminalRenderer.Outdated` no longer keeps unused local counters; it
    consumes the report summary like every other renderer.
  * Update skipped/diagnostic bullets are indented consistently with every
    other section (`  • ...`), and an empty inventory now still ends with the
    canonical `Summary` block.
  * CI mode: the plan summary is preceded by the same blank line as the other
    operations, and update summary keys are unambiguous
    (`updated-count`/`skipped-count`/`failed-count`) instead of colliding with
    per-tool `updated:`/`skipped:`/`failed:` records.
  * The discovery header no longer runs `go env GOVERSION` in JSON or quiet
    modes via a renderer type-switch; the mode is decided once in `cmd`.
  * `go.mod` dependency metadata corrected (`golang.org/x/mod` is a direct
    requirement, not `indirect`).
  * Benchmarks fixed to measure the real code paths hermetically (discovery,
    planning, outdated) and recorded in the certification report.
  * New regression tests: plan ownership, install-command agreement,
    update skipped indentation, summary-from-report counting, CI count keys,
    `--info --json` envelope absence, and empty-inventory summary.

## [v1.4.0]

### Changed

* CLI version bumped to v1.4.0.
* v1.4.0 architectural consolidation:
  * Renderers now consume immutable report structures instead of deriving statistics independently.
  * `Renderer` interface updated with `Inventory`, `Verify`, `Outdated`, `Update`, and `DryRun` report types.
  * `App` caches discovery results per invocation, eliminating redundant `Load()` calls.
  * `Update()` pipeline separates planning from execution internally.
  * `--dry-run` is a planning-only command, distinct from `--check`.
  * Progress renderer dynamically expands only when subprocess output exists.
  * JSON output uses stable lowercase field names with `json` tags on all report structs.
  * All duplicated counting logic eliminated; `LoadSummary` is the single source of truth.

## [v1.3.0]

### Added

* `--outdated` flag to compare installed versions against upstream releases
  using semantic version ordering.
* `--json` global flag that emits stable machine-readable JSON for `--list`,
  `--info`, `--verify`, `--outdated`, and update output.
* `internal/app` orchestration layer separating CLI from domain logic.
* `Renderer` interface with `TerminalRenderer` and `JSONRenderer`
  implementations.
* `Runner` interface for injectable, context-aware subprocess execution.
* `testdata/` golden and JSON fixtures, plus a hermetic fixture builder.

## Changed

* Version is now injected at build time via `-ldflags "-X main.version=..."`
  instead of being hardcoded; it defaults to `dev`.
* GOBIN discovery is performed once per invocation; the update path no longer
  re-scans the filesystem after listing.
* JSON responses emit `[]` rather than `null` for empty lists, and honor
  `--check` via the `check_only` field.
* Local/devel tools are reported as skipped in JSON output rather than
  misclassified as failed.
* Invalid-binary diagnostics render a stable, categorized message instead of
  raw toolchain text.

### Fixed

* JSON update output previously classified skipped local/devel tools as
  failures.
* JSON update output previously discarded the skipped-tool list and the
  `--check` flag.
* The CLI loaded the tool inventory twice on the default update path.

### Tested

* Unit tests for `CanUpdate`, `InstallTarget`, `Update`, `CheckOutdated`, and
  `Verify`, including error paths.
* Integration tests exercising discovery, metadata parsing, and verification
  against a temporary GOBIN of real fixture binaries.
* Golden CLI tests covering stdout, stderr, and exit codes for every command.
