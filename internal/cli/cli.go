package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"helm/internal/app"
	"helm/internal/tool"
)

var (
	version    = "v1.9.0"
	commitHash = ""
	buildDate  = ""
)

const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
	ExitEnv     = 3
)

// Invocation identifies how the canonical Helm executable was invoked.
//
// Executable identity is resolved at the CLI boundary from the process
// basename and must not leak into the application or domain layers.
type Invocation struct {
	// Name is the invocation basename, e.g. "helm" or "update-go-tools".
	Name string
}

// ResolveInvocation maps an executable basename to its invocation identity.
//
// Supported names:
//
//	helm, Helm         -> canonical Helm behavior
//	update-go-tools    -> preserved compatibility alias
//
// Any other name defaults to canonical Helm behavior. Case is matched only for
// the explicitly supported canonical spellings; arbitrary variants such as
// "HELM" are not normalized.
func ResolveInvocation(rawName string) Invocation {
	return Invocation{Name: rawName}
}

type cliOptions struct {
	jsonOutput bool
	quiet      bool
	ci         bool
	verbose    bool
	plan       bool
	positional []string
}

// newApp constructs the application. It is a package-private seam so tests can
// deterministically exercise environment-resolution failure propagation through
// the real cli.Run failure path without changing production behavior. It
// defaults to app.NewApp.
var newApp = app.NewApp

func Run(inv Invocation, args []string) int {
	ctx := context.Background()
	opts, code := parseFlags(args)
	if code != 0 {
		return code
	}

	mode := app.ModeTerminal
	switch {
	case opts.jsonOutput:
		mode = app.ModeJSON
	case opts.ci:
		mode = app.ModeCI
	case opts.quiet:
		mode = app.ModeQuiet
	}

	renderer := app.NewRenderer(mode, opts.verbose)
	application, err := newApp(renderer, tool.DefaultRunner{})
	if err != nil {
		if errors.Is(err, tool.ErrGobinResolution) {
			return ExitEnv
		}
		return fail("Error:", err)
	}

	operation, toolArgs := splitOperation(opts.positional)

	switch operation {
	case "--help", "-h":
		printHelp()
		return ExitSuccess
	case "--version", "-v":
		printVersion()
		return ExitSuccess
	}

	warnUnusedModifiers(opts, operation, mode)

	// Every remaining operation loads the tool set once and renders the
	// discovery header before dispatch. Both the explicit-operation path and
	// the default update/plan path share this setup.
	loadRes, err := application.LoadTools()
	if err != nil {
		return fail("Error loading tools:", err)
	}

	// The default update and plan paths filter by tool name. Reject unknown
	// names before rendering anything: a foreign or misspelled invocation
	// (for example, another program's shell completion calling Helm with
	// unexpected words) must fail with a diagnostic on stderr and no report
	// on stdout, never run as a silently empty successful operation.
	if operation == "" {
		if unknown := tool.UnknownFilterNames(loadRes.Tools, toolArgs); len(unknown) > 0 {
			fmt.Fprintf(os.Stderr, "Error: Unknown tool(s): %s. Run 'helm --list' to see known tools.\n", strings.Join(unknown, ", "))
			return ExitUsage
		}
	}

	// Explicit operations that accept no filters reject any positional
	// outright: silently ignoring them would widen scope beyond what the
	// user expressed (for example, copying a filtered update command onto
	// --outdated or --list). --info consumes exactly one target.
	if operation == "--list" || operation == "--outdated" {
		if len(toolArgs) > 0 {
			fmt.Fprintf(os.Stderr, "Error: Option %s takes no tool names.\n", operation)
			return ExitUsage
		}
	}
	if operation == "--info" && len(toolArgs) > 1 {
		fmt.Fprintln(os.Stderr, "Error: Option --info takes exactly one tool name.")
		return ExitUsage
	}

	renderHeader(ctx, application, renderer, loadRes, mode)

	switch operation {
	case "--list":
		if err := application.RunInventory(); err != nil {
			return fail("Error:", err)
		}
	case "--outdated":
		if err := application.RunOutdated(ctx); err != nil {
			return fail("Error:", err)
		}
	case "--info":
		if len(toolArgs) == 0 {
			fmt.Fprintln(os.Stderr, "Error: Option --info requires a tool name.")
			return ExitUsage
		}
		// An unknown tool name is an operational lookup failure, not a
		// usage error: the invocation syntax is valid but the target does
		// not resolve to a known tool.
		if err := application.RunInfo(toolArgs[0]); err != nil {
			return fail("Error:", err)
		}
	}

	if operation != "" {
		return ExitSuccess
	}

	if opts.plan {
		if err := application.RunPlan(ctx, toolArgs); err != nil {
			return fail("Error:", err)
		}
		return ExitSuccess
	}

	if err := application.RunUpdate(ctx, toolArgs); err != nil {
		return fail("Error:", err)
	}
	return ExitSuccess
}

func parseFlags(args []string) (cliOptions, int) {
	opts := cliOptions{}
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--json":
			opts.jsonOutput = true
		case "--quiet", "-q":
			opts.quiet = true
		case "--ci":
			opts.ci = true
		case "--verbose", "-V":
			opts.verbose = true
		case "--check", "--dry-run":
			opts.plan = true
		case "--help", "-h", "--version", "-v", "--list", "--outdated", "--info":
			positional = append(positional, arg)
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "Error: Unknown option: %s. Run 'helm --help' for usage.\n", arg)
				return cliOptions{}, ExitUsage
			}
			positional = append(positional, arg)
		}
	}
	opts.positional = positional
	return opts, 0
}

func splitOperation(positional []string) (string, []string) {
	if len(positional) == 0 {
		return "", nil
	}
	switch positional[0] {
	case "--list", "--outdated", "--help", "-h", "--version", "-v", "--info":
		return positional[0], positional[1:]
	}
	return "", positional
}

// warnUnusedModifiers tells the user about flags the requested invocation
// silently discards, instead of pretending they applied. Warnings never
// change the exit status; the operation proceeds as documented.
func warnUnusedModifiers(opts cliOptions, operation string, mode app.RenderMode) {
	if opts.plan && operation != "" {
		fmt.Fprintf(os.Stderr, "Warning: --check/--dry-run has no effect with %s.\n", operation)
	}
	if opts.verbose && mode == app.ModeJSON {
		fmt.Fprintln(os.Stderr, "Warning: --verbose has no effect with --json.")
	}
	if opts.verbose && mode == app.ModeQuiet {
		fmt.Fprintln(os.Stderr, "Warning: --verbose has no effect with --quiet.")
	}
	set := make([]string, 0, 3)
	if opts.jsonOutput {
		set = append(set, "--json")
	}
	if opts.ci {
		set = append(set, "--ci")
	}
	if opts.quiet {
		set = append(set, "--quiet")
	}
	// Output modes resolve by precedence json > ci > quiet; name the losers.
	if len(set) > 1 {
		fmt.Fprintf(os.Stderr, "Warning: %s ignored; %s takes precedence.\n", strings.Join(set[1:], " and "), set[0])
	}
}

func renderHeader(ctx context.Context, application *app.App, renderer app.Renderer, loadRes tool.LoadResult, mode app.RenderMode) {
	hdr := app.HeaderInfo{Gobin: application.Gobin, LoadRes: loadRes}
	if mode != app.ModeJSON && mode != app.ModeQuiet {
		if goVer, err := getGoVersion(ctx, application.Runner); err == nil && goVer != "" {
			hdr.GoVersion = goVer
		}
	}
	_ = renderer.Header(hdr)
}

func fail(prefix string, err error) int {
	fmt.Fprintln(os.Stderr, prefix, err)
	return ExitFailure
}

func getGoVersion(ctx context.Context, runner tool.Runner) (string, error) {
	output, err := runner.Run(ctx, tool.Command{
		Name: "go",
		Args: []string{"env", "GOVERSION"},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func printVersion() {
	fmt.Printf("Helm %s\n", version)
	if commitHash != "" {
		fmt.Printf("Commit    %s\n", commitHash)
	}
	if buildDate != "" {
		fmt.Printf("Built     %s\n", buildDate)
	}
	fmt.Println()
}

func printHelp() {
	fmt.Printf(`Helm %s - Inspect, inventory, and update Go-managed tools

Usage:
    helm [tool...]
    helm --list [--json|--ci|--quiet]
    helm --check/--dry-run [--verbose|-V]
    helm --outdated [--json|--ci]
    helm --info <tool>
    helm --json
    helm --quiet | -q
    helm --ci
    helm --help
    helm --version

Operations:
    --list             List all Go-managed tools with versions, health status, and packages
    --check/--dry-run  Summarize pending updates without executing them (aliases)
    --outdated         Check upstream releases for installed tools
    --info             Show detailed metadata for a specific tool

Output modifiers:
    --json       Emit machine-readable JSON for any operation
    --quiet, -q  Suppress the discovery header and chatter; emit only the requested data and summary
    --ci         Deterministic, ASCII-only, line-oriented terminal output
    --verbose, -V  Detailed planning view (packages and install commands)

Utility:
    --help       Display this help message
    --version    Display version information

Without arguments, updates all discovered Go tools.
With one or more tool names, updates only those specified tools.
Unknown tool names are rejected; --list and --outdated take no tool names
and --info takes exactly one.
When several output modes are given, --json wins over --ci, which wins
over --quiet. Flags with no effect print a Warning to stderr.
Exit codes: 0 success, 1 operation failure, 2 usage error, 3 environment error.
`, version)
}
