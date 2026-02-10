package cli

import (
	"fmt"

	"github.com/morozRed/skelly/internal/nav"
	"github.com/morozRed/skelly/internal/output"
	"github.com/spf13/cobra"
)

func NewRootCommand(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "skelly",
		Short: "Generate LLM-friendly codebase structure maps",
		Long: `Skelly extracts the skeleton of your codebase - functions, classes,
dependencies, and call graphs - into token-efficient text files
that help LLMs understand your code without reading every line.

Output is written to .skelly/.context/ and can be version-controlled.`,
	}

	// Core Commands
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .skelly/.context/ directory in current project",
		RunE:  RunInit,
	}
	initCmd.Flags().String("llm", "", "Generate LLM integration files (comma-separated: codex,claude,cursor)")
	initCmd.Flags().Bool("no-generate", false, "Create directory only, skip auto-generate")
	initCmd.Flags().String("format", string(output.FormatText), "Output format: text|jsonl")

	setupCmd := &cobra.Command{
		Use:    "setup",
		Short:  "Setup context files for agent workflows",
		Hidden: true,
		RunE:   RunSetup,
	}
	setupCmd.Flags().String("format", string(output.FormatText), "Generate output format: text|jsonl")

	generateCmd := &cobra.Command{
		Use:   "generate [path]",
		Short: "Generate or regenerate .skelly/.context/ files",
		Args:  cobra.MaximumNArgs(1),
		RunE:  RunGenerate,
	}
	generateCmd.Flags().StringSliceP("lang", "l", []string{}, "Languages to include (default: auto-detect)")
	generateCmd.Flags().String("format", string(output.FormatText), "Output format: text|jsonl")
	generateCmd.Flags().Bool("json", false, "Print machine-readable run summary")

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Incrementally update .skelly/.context/ for changed files",
		RunE:  RunUpdate,
	}
	updateCmd.Flags().Bool("explain", false, "Explain why each impacted file is included")
	updateCmd.Flags().String("format", string(output.FormatText), "Output format: text|jsonl")
	updateCmd.Flags().Bool("json", false, "Print machine-readable run summary")

	// Inspect Commands
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show what changed and what update would regenerate",
		RunE:  RunStatus,
	}
	statusCmd.Flags().Bool("json", false, "Print machine-readable status output")

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate skelly setup and context freshness",
		RunE:  RunDoctor,
	}
	doctorCmd.Flags().Bool("json", false, "Print machine-readable doctor output")

	// Navigate Commands
	symbolCmd := &cobra.Command{
		Use:   "symbol <name|id>",
		Short: "Lookup symbols by name or stable ID",
		Args:  cobra.ExactArgs(1),
		RunE:  nav.RunSymbol,
	}
	symbolCmd.Flags().Bool("json", false, "Print machine-readable symbol matches")
	symbolCmd.Flags().Bool("fuzzy", false, "Enable BM25 fuzzy fallback when exact lookup misses")
	symbolCmd.Flags().Int("limit", 10, "Maximum number of symbol matches to return")

	callersCmd := &cobra.Command{
		Use:   "callers <name|id>",
		Short: "Show direct callers of a symbol",
		Args:  cobra.ExactArgs(1),
		RunE:  nav.RunCallers,
	}
	callersCmd.Flags().Bool("json", false, "Print machine-readable caller results")
	callersCmd.Flags().Bool("lsp", false, "Enable optional LSP-assisted navigation metadata")

	calleesCmd := &cobra.Command{
		Use:   "callees <name|id>",
		Short: "Show direct callees of a symbol",
		Args:  cobra.ExactArgs(1),
		RunE:  nav.RunCallees,
	}
	calleesCmd.Flags().Bool("json", false, "Print machine-readable callee results")
	calleesCmd.Flags().Bool("lsp", false, "Enable optional LSP-assisted navigation metadata")

	traceCmd := &cobra.Command{
		Use:   "trace <name|id>",
		Short: "Trace outgoing calls from a symbol up to depth N",
		Args:  cobra.ExactArgs(1),
		RunE:  nav.RunTrace,
	}
	traceCmd.Flags().Int("depth", 2, "Traversal depth (>=1)")
	traceCmd.Flags().Bool("json", false, "Print machine-readable trace results")
	traceCmd.Flags().Bool("lsp", false, "Enable optional LSP-assisted navigation metadata")

	pathCmd := &cobra.Command{
		Use:   "path <from> <to>",
		Short: "Find shortest call path between two symbols",
		Args:  cobra.ExactArgs(2),
		RunE:  nav.RunPath,
	}
	pathCmd.Flags().Bool("json", false, "Print machine-readable path results")
	pathCmd.Flags().Bool("lsp", false, "Enable optional LSP-assisted navigation metadata")

	definitionCmd := &cobra.Command{
		Use:   "definition <symbol|file:line>",
		Short: "Resolve the definition symbol for an identifier or location",
		Args:  cobra.ExactArgs(1),
		RunE:  nav.RunDefinition,
	}
	definitionCmd.Flags().Bool("json", false, "Print machine-readable definition result")
	definitionCmd.Flags().Bool("lsp", false, "Enable optional LSP-assisted navigation metadata")

	referencesCmd := &cobra.Command{
		Use:   "references <symbol|file:line>",
		Short: "Show references for a symbol or location",
		Args:  cobra.ExactArgs(1),
		RunE:  nav.RunReferences,
	}
	referencesCmd.Flags().Bool("json", false, "Print machine-readable references result")
	referencesCmd.Flags().Bool("lsp", false, "Enable optional LSP-assisted navigation metadata")

	// Annotate Commands
	enrichCmd := &cobra.Command{
		Use:   "enrich <target> <description>",
		Short: "Add or update one symbol description in .skelly/.context/enrich.jsonl",
		Args:  cobra.MinimumNArgs(2),
		RunE:  RunEnrich,
	}
	enrichCmd.Flags().Bool("json", false, "Print machine-readable summary")

	// Additional Commands
	installHookCmd := &cobra.Command{
		Use:   "install-hook",
		Short: "Install git pre-commit hook for auto-updates",
		RunE:  RunInstallHook,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("skelly %s\n", version)
		},
	}

	rootCmd.AddCommand(
		initCmd,
		setupCmd,
		generateCmd,
		updateCmd,
		statusCmd,
		doctorCmd,
		symbolCmd,
		callersCmd,
		calleesCmd,
		traceCmd,
		pathCmd,
		definitionCmd,
		referencesCmd,
		enrichCmd,
		installHookCmd,
		versionCmd,
	)

	return rootCmd
}
