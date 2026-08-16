# Helm
> alias: update-go-tools

A lightweight utility to discover, inspect, and maintain Go developer tools installed with `go install`.
It reads embedded module metadata through debug/buildinfo and can list, inspect, plan, check, or update the tools in your Go binary directory.

## Install

```bash
go build -o helm ./cmd/helm
```

## Usage

```text
helm [tool...]          # update specific tools (or all if omitted)
helm --list             # inventory with health status
helm --check            # plan updates without executing
helm --outdated         # check which tools have newer releases
helm --info <tool>      # detailed metadata for a single tool
helm --json             # machine-readable JSON output (any operation)
helm --ci               # deterministic, script-friendly output
helm --quiet            # suppress headers; emit only data
helm --help / --version
```

Use `--json` for machine-readable output, `--ci` for deterministic text, and
`--quiet`/`-q` to suppress headers. `--dry-run` aliases `--check`.

There are no subcommands; all interactions are flag-driven operating modes.

## Documentation

See [Architecture](ARCHITECTURE.md), [Development](DEVELOPMENT.md), and
[Changelog](CHANGELOG.md) for project documentation.

## License

MIT License

Copyright (c) 2026 Divij Ganjoo

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
