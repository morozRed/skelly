package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/ignore"
	"github.com/morozRed/skelly/internal/output"
	"github.com/morozRed/skelly/internal/parser"
	"github.com/morozRed/skelly/internal/state"
	"github.com/morozRed/skelly/pkg/languages"
	"github.com/spf13/cobra"
)

var version = "0.1.0-dev"

const (
	hookStart = "# >>> skelly update hook >>>"
	hookEnd   = "# <<< skelly update hook <<<"

	navigationIndexFile = "nav-index.json"

	llmManagedBlockStart = "<!-- skelly:managed:start -->"
	llmManagedBlockEnd   = "<!-- skelly:managed:end -->"
)

type RunSummary struct {
	Mode          string              `json:"mode"`
	Format        string              `json:"format,omitempty"`
	RootPath      string              `json:"root_path"`
	OutputDir     string              `json:"output_dir,omitempty"`
	Scanned       int                 `json:"scanned"`
	Parsed        int                 `json:"parsed"`
	Reused        int                 `json:"reused"`
	Rewritten     int                 `json:"rewritten"`
	Changed       int                 `json:"changed"`
	Deleted       int                 `json:"deleted"`
	Impacted      int                 `json:"impacted"`
	DurationMS    int64               `json:"duration_ms"`
	ChangedFiles  []string            `json:"changed_files,omitempty"`
	DeletedFiles  []string            `json:"deleted_files,omitempty"`
	ImpactedFiles []string            `json:"impacted_files,omitempty"`
	Reasons       map[string][]string `json:"reasons,omitempty"`
}

type EnrichRunSummary struct {
	Mode        string   `json:"mode"`
	Agent       string   `json:"agent"`
	Scope       string   `json:"scope"`
	Target      string   `json:"target,omitempty"`
	RootPath    string   `json:"root_path"`
	OutputFile  string   `json:"output_file,omitempty"`
	Files       int      `json:"files"`
	Symbols     int      `json:"symbols"`
	Succeeded   int      `json:"succeeded,omitempty"`
	Failed      int      `json:"failed,omitempty"`
	CacheHits   int      `json:"cache_hits,omitempty"`
	CacheMisses int      `json:"cache_misses,omitempty"`
	DryRun      bool     `json:"dry_run"`
	DurationMS  int64    `json:"duration_ms"`
	Targets     []string `json:"targets,omitempty"`
}

type DoctorSummary struct {
	Mode         string          `json:"mode"`
	RootPath     string          `json:"root_path"`
	ContextDir   string          `json:"context_dir"`
	Format       string          `json:"format"`
	Healthy      bool            `json:"healthy"`
	Clean        bool            `json:"clean"`
	Changed      int             `json:"changed"`
	Deleted      int             `json:"deleted"`
	Missing      []string        `json:"missing,omitempty"`
	Integrations map[string]bool `json:"integrations,omitempty"`
	Suggestions  []string        `json:"suggestions,omitempty"`
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "skelly",
		Short: "Generate LLM-friendly codebase structure maps",
		Long: `Skelly extracts the skeleton of your codebase - functions, classes,
dependencies, and call graphs - into token-efficient text files
that help LLMs understand your code without reading every line.

Output is written to .skelly/.context/ and can be version-controlled.`,
	}

	// Init command - create .skelly/.context/ structure
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .skelly/.context/ directory in current project",
		RunE:  runInit,
	}
	initCmd.Flags().String("llm", "", "Generate LLM integration files (comma-separated: codex,claude,cursor)")

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Setup context files for agent workflows",
		RunE:  runSetup,
	}
	setupCmd.Flags().String("format", string(output.FormatText), "Generate output format: text|jsonl")

	// Generate command - full regeneration
	generateCmd := &cobra.Command{
		Use:   "generate [path]",
		Short: "Generate or regenerate .skelly/.context/ files",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runGenerate,
	}
	generateCmd.Flags().StringSliceP("lang", "l", []string{}, "Languages to include (default: auto-detect)")
	generateCmd.Flags().String("format", string(output.FormatText), "Output format: text|jsonl")
	generateCmd.Flags().Bool("json", false, "Print machine-readable run summary")

	// Update command - incremental update (changed files only)
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Incrementally update .skelly/.context/ for changed files",
		RunE:  runUpdate,
	}
	updateCmd.Flags().Bool("explain", false, "Explain why each impacted file is included")
	updateCmd.Flags().String("format", string(output.FormatText), "Output format: text|jsonl")
	updateCmd.Flags().Bool("json", false, "Print machine-readable run summary")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show what changed and what update would regenerate",
		RunE:  runStatus,
	}
	statusCmd.Flags().Bool("json", false, "Print machine-readable status output")

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate skelly setup and context freshness",
		RunE:  runDoctor,
	}
	doctorCmd.Flags().Bool("json", false, "Print machine-readable doctor output")

	symbolCmd := &cobra.Command{
		Use:   "symbol <name|id>",
		Short: "Lookup symbols by name or stable ID",
		Args:  cobra.ExactArgs(1),
		RunE:  runSymbol,
	}
	symbolCmd.Flags().Bool("json", false, "Print machine-readable symbol matches")

	callersCmd := &cobra.Command{
		Use:   "callers <name|id>",
		Short: "Show direct callers of a symbol",
		Args:  cobra.ExactArgs(1),
		RunE:  runCallers,
	}
	callersCmd.Flags().Bool("json", false, "Print machine-readable caller results")

	calleesCmd := &cobra.Command{
		Use:   "callees <name|id>",
		Short: "Show direct callees of a symbol",
		Args:  cobra.ExactArgs(1),
		RunE:  runCallees,
	}
	calleesCmd.Flags().Bool("json", false, "Print machine-readable callee results")

	traceCmd := &cobra.Command{
		Use:   "trace <name|id>",
		Short: "Trace outgoing calls from a symbol up to depth N",
		Args:  cobra.ExactArgs(1),
		RunE:  runTrace,
	}
	traceCmd.Flags().Int("depth", 2, "Traversal depth (>=1)")
	traceCmd.Flags().Bool("json", false, "Print machine-readable trace results")

	pathCmd := &cobra.Command{
		Use:   "path <from> <to>",
		Short: "Find shortest call path between two symbols",
		Args:  cobra.ExactArgs(2),
		RunE:  runPath,
	}
	pathCmd.Flags().Bool("json", false, "Print machine-readable path results")

	enrichCmd := &cobra.Command{
		Use:   "enrich <target> <description>",
		Short: "Add or update one symbol description in .skelly/.context/enrich.jsonl",
		Args:  cobra.MinimumNArgs(2),
		RunE:  runEnrich,
	}
	enrichCmd.Flags().Bool("json", false, "Print machine-readable summary")

	// Install-hook command - add git pre-commit hook
	installHookCmd := &cobra.Command{
		Use:   "install-hook",
		Short: "Install git pre-commit hook for auto-updates",
		RunE:  runInstallHook,
	}

	// Version command
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
		enrichCmd,
		installHookCmd,
		versionCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}

	writer := output.NewWriter(rootPath)
	if err := writer.Init(); err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	if err := state.NewState().Save(contextDir); err != nil {
		return fmt.Errorf("failed to write initial state: %w", err)
	}

	llmRaw, err := optionalStringFlag(cmd, "llm")
	if err != nil {
		return err
	}
	llmProviders, err := parseLLMProviders(llmRaw)
	if err != nil {
		return err
	}
	if len(llmProviders) > 0 {
		updatedFiles, err := generateLLMIntegrationFiles(rootPath, llmProviders)
		if err != nil {
			return err
		}
		if len(updatedFiles) > 0 {
			fmt.Printf("Updated LLM integration files: %s\n", strings.Join(updatedFiles, ", "))
		}
	}

	fmt.Printf("Initialized context directory at %s\n", contextDir)
	return nil
}

func optionalStringFlag(cmd *cobra.Command, name string) (string, error) {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return "", nil
	}
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", fmt.Errorf("failed to read --%s flag: %w", name, err)
	}
	return strings.TrimSpace(value), nil
}

func parseLLMProviders(raw string) ([]string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return nil, nil
	}

	allowed := map[string]bool{
		"codex":  true,
		"claude": true,
		"cursor": true,
	}
	seen := make(map[string]bool)
	out := make([]string, 0, 3)

	addProvider := func(value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if value == "all" {
			for _, provider := range []string{"codex", "claude", "cursor"} {
				if !seen[provider] {
					seen[provider] = true
					out = append(out, provider)
				}
			}
			return nil
		}
		if !allowed[value] {
			return fmt.Errorf("unsupported --llm provider %q (supported: codex, claude, cursor, all)", value)
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
		return nil
	}

	for _, chunk := range strings.Split(raw, ",") {
		for _, value := range strings.Fields(chunk) {
			if err := addProvider(value); err != nil {
				return nil, err
			}
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func generateLLMIntegrationFiles(rootPath string, providers []string) ([]string, error) {
	updated := make([]string, 0)

	skillPath := filepath.Join(rootPath, ".skelly", "skills", "skelly.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create skills directory: %w", err)
	}
	skillContent := buildSkellySkillContent()
	wrote, err := writeFileIfChangedTracked(skillPath, []byte(skillContent))
	if err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", skillPath, err)
	}
	if wrote {
		updated = append(updated, filepath.ToSlash(filepath.Clean(filepath.Join(".skelly", "skills", "skelly.md"))))
	}

	contextPath := filepath.Join(rootPath, "CONTEXT.md")
	contextDoc, err := upsertManagedMarkdownFile(contextPath, buildContextBlock())
	if err != nil {
		return nil, err
	}
	if contextDoc {
		updated = append(updated, "CONTEXT.md")
	}

	for _, provider := range providers {
		switch provider {
		case "codex":
			changed, err := upsertManagedMarkdownFile(
				filepath.Join(rootPath, "AGENTS.md"),
				buildRootAdapterBlock("Codex"),
			)
			if err != nil {
				return nil, err
			}
			if changed {
				updated = append(updated, "AGENTS.md")
			}
		case "claude":
			changed, err := upsertManagedMarkdownFile(
				filepath.Join(rootPath, "CLAUDE.md"),
				buildRootAdapterBlock("Claude"),
			)
			if err != nil {
				return nil, err
			}
			if changed {
				updated = append(updated, "CLAUDE.md")
			}
		case "cursor":
			cursorPath := filepath.Join(rootPath, ".cursor", "rules", "skelly-context.mdc")
			if err := os.MkdirAll(filepath.Dir(cursorPath), 0755); err != nil {
				return nil, fmt.Errorf("failed to create cursor rules directory: %w", err)
			}
			changed, err := writeFileIfChangedTracked(cursorPath, []byte(buildCursorRuleContent()))
			if err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", cursorPath, err)
			}
			if changed {
				updated = append(updated, filepath.ToSlash(filepath.Clean(filepath.Join(".cursor", "rules", "skelly-context.mdc"))))
			}
		}
	}

	sort.Strings(updated)
	return updated, nil
}

func upsertManagedMarkdownFile(path, body string) (bool, error) {
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}

	managed := fmt.Sprintf("%s\n%s\n%s", llmManagedBlockStart, strings.TrimSpace(body), llmManagedBlockEnd)
	updated := upsertManagedBlock(existing, llmManagedBlockStart, llmManagedBlockEnd, managed)
	return writeFileIfChangedTracked(path, []byte(updated))
}

func upsertManagedBlock(existing, startMarker, endMarker, managedContent string) string {
	if existing == "" {
		return managedContent + "\n"
	}

	start := strings.Index(existing, startMarker)
	end := strings.Index(existing, endMarker)
	if start >= 0 && end >= start {
		end += len(endMarker)
		updated := existing[:start] + managedContent + existing[end:]
		return ensureTrailingNewline(updated)
	}

	base := ensureTrailingNewline(existing)
	return base + "\n" + managedContent + "\n"
}

func buildSkellySkillContent() string {
	return `# Skelly Skill

Use Skelly context artifacts and commands before opening many source files.

Workflow:
1. Run skelly doctor to validate context freshness.
2. If stale, run skelly update.
3. Prefer .skelly/.context/manifest.json, symbols.jsonl, and edges.jsonl when present.
4. Fall back to index.txt and graph.txt for text mode repos.
5. Use skelly status before major changes to understand impacted files.
`
}

func buildContextBlock() string {
	return `# Skelly Context

This repository uses Skelly as the canonical code context layer for LLM tools.

- Canonical skill instructions: .skelly/skills/skelly.md
- Context directory: .skelly/.context/
- Preferred machine-readable artifacts:
  - .skelly/.context/symbols.jsonl
  - .skelly/.context/edges.jsonl
  - .skelly/.context/manifest.json

Recommended command sequence:
1. skelly doctor
2. skelly update (if doctor reports stale context)
3. skelly status (to inspect impact)
`
}

func buildRootAdapterBlock(agentName string) string {
	return fmt.Sprintf(`# Skelly Integration (%s)

Use Skelly outputs before broad code reads.

1. Run skelly doctor at session start.
2. If stale, run skelly update.
3. Follow .skelly/skills/skelly.md.
4. Use CONTEXT.md for canonical artifact paths.
`, agentName)
}

func buildCursorRuleContent() string {
	return `---
description: Use Skelly context artifacts for code navigation
alwaysApply: true
---

Run skelly doctor first. If context is stale, run skelly update.
Use .skelly/skills/skelly.md and CONTEXT.md as primary guidance.
Prefer .skelly/.context/symbols.jsonl, .skelly/.context/edges.jsonl, and .skelly/.context/manifest.json when available.
`
}

func runSetup(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}

	format, err := parseOutputFormat(cmd)
	if err != nil {
		return err
	}
	fmt.Printf("setup: format=%s\n", format)
	fmt.Println("setup: running generate...")
	if err := generateContext(rootPath, nil, format, false); err != nil {
		return err
	}
	fmt.Println(`setup: done. Agents can add descriptions with:
  skelly enrich <target> "<description>"`)
	return nil
}

func runGenerate(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	languageFilter, err := parseLanguageFilter(cmd)
	if err != nil {
		return err
	}
	format, err := parseOutputFormat(cmd)
	if err != nil {
		return err
	}
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("failed to read --json flag: %w", err)
	}

	rootPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path %q: %w", path, err)
	}

	info, err := os.Stat(rootPath)
	if err != nil {
		return fmt.Errorf("failed to access path %q: %w", rootPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %q is not a directory", rootPath)
	}

	return generateContext(rootPath, languageFilter, format, asJSON)
}

func generateContext(rootPath string, languageFilter map[string]bool, format output.Format, asJSON bool) error {
	start := time.Now()
	ignoreRules, err := loadIgnoreRules(rootPath)
	if err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	previousOutputHashes, _ := loadOutputHashesFromState(contextDir)

	registry := languages.NewDefaultRegistry()
	parseResult, err := registry.ParseDirectory(rootPath, ignoreRules)
	if err != nil {
		return fmt.Errorf("failed to parse source files: %w", err)
	}
	reportParseIssues(parseResult.Issues)
	parseResult.Files = filterFilesByLanguage(parseResult.Files, languageFilter)
	for i := range parseResult.Files {
		ensureSymbolIDs(&parseResult.Files[i])
	}

	g := graph.BuildFromParseResult(parseResult)
	writer := output.NewWriter(rootPath)
	if err := writer.WriteAll(g, parseResult, format); err != nil {
		return fmt.Errorf("failed to write output files: %w", err)
	}
	if err := writeNavigationIndex(contextDir, g); err != nil {
		return fmt.Errorf("failed to write navigation index: %w", err)
	}

	if err := persistState(contextDir, parseResult.Files, g, format); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	updatedState, err := state.Load(contextDir)
	if err != nil {
		return fmt.Errorf("failed to reload state after generate: %w", err)
	}

	summary := RunSummary{
		Mode:          "generate",
		Format:        string(format),
		RootPath:      rootPath,
		OutputDir:     filepath.Join(rootPath, output.ContextDir),
		Scanned:       len(parseResult.Files),
		Parsed:        len(parseResult.Files),
		Reused:        0,
		Rewritten:     countRewrittenOutputs(previousOutputHashes, updatedState.OutputHashes),
		Changed:       len(parseResult.Files),
		Deleted:       0,
		Impacted:      len(parseResult.Files),
		DurationMS:    time.Since(start).Milliseconds(),
		ChangedFiles:  collectPaths(parseResult.Files),
		ImpactedFiles: collectPaths(parseResult.Files),
	}

	return printRunSummary(summary, asJSON)
}

func parseLanguageFilter(cmd *cobra.Command) (map[string]bool, error) {
	langs, err := cmd.Flags().GetStringSlice("lang")
	if err != nil {
		return nil, fmt.Errorf("failed to read --lang flag: %w", err)
	}
	if len(langs) == 0 {
		return nil, nil
	}

	aliases := map[string]string{
		"go":         "go",
		"python":     "python",
		"py":         "python",
		"ruby":       "ruby",
		"rb":         "ruby",
		"typescript": "typescript",
		"ts":         "typescript",
		"javascript": "javascript",
		"js":         "javascript",
	}

	filter := make(map[string]bool, len(langs))
	for _, lang := range langs {
		key := strings.ToLower(strings.TrimSpace(lang))
		canonical, ok := aliases[key]
		if !ok {
			return nil, fmt.Errorf("unsupported language %q (supported: go, python, ruby, typescript, javascript)", lang)
		}
		filter[canonical] = true
	}

	return filter, nil
}

func parseOutputFormat(cmd *cobra.Command) (output.Format, error) {
	value, err := cmd.Flags().GetString("format")
	if err != nil {
		return "", fmt.Errorf("failed to read --format flag: %w", err)
	}
	return output.ParseFormat(value)
}

func filterFilesByLanguage(files []parser.FileSymbols, languageFilter map[string]bool) []parser.FileSymbols {
	if len(languageFilter) == 0 {
		return files
	}

	filtered := make([]parser.FileSymbols, 0, len(files))
	for _, file := range files {
		if languageFilter[file.Language] {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func reportParseIssues(issues []parser.ParseIssue) {
	for _, issue := range issues {
		if issue.Language != "" {
			fmt.Fprintf(os.Stderr, "[%s] %s (%s): %s\n", issue.Severity, issue.File, issue.Language, issue.Message)
			continue
		}
		fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", issue.Severity, issue.File, issue.Message)
	}
}

func loadIgnoreRules(rootPath string) ([]string, error) {
	ignorePath := filepath.Join(rootPath, ".skellyignore")
	f, err := os.Open(ignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read .skellyignore: %w", err)
	}
	defer f.Close()

	rules := make([]string, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = append(rules, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse .skellyignore: %w", err)
	}

	return rules, nil
}

func persistState(contextDir string, files []parser.FileSymbols, g *graph.Graph, format output.Format) error {
	st := state.NewState()
	for _, file := range files {
		st.SetFileData(file)
	}
	applyGraphDependencies(st, g, nil)
	if err := recordOutputHashes(st, contextDir, format); err != nil {
		return err
	}
	return st.Save(contextDir)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	start := time.Now()
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	explain, err := cmd.Flags().GetBool("explain")
	if err != nil {
		return fmt.Errorf("failed to read --explain flag: %w", err)
	}
	format, err := parseOutputFormat(cmd)
	if err != nil {
		return err
	}
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("failed to read --json flag: %w", err)
	}

	registry := languages.NewDefaultRegistry()
	ignoreRules, err := loadIgnoreRules(rootPath)
	if err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	st, err := state.Load(contextDir)
	if err != nil {
		if isCorruptStateError(err) {
			fmt.Fprintf(os.Stderr, "warning: corrupt state file detected (%v); running full regenerate\n", err)
			return generateContext(rootPath, nil, format, asJSON)
		}
		return fmt.Errorf("failed to load state: %w", err)
	}
	if st.ParserVersion != state.CurrentParserVersion {
		fmt.Fprintf(
			os.Stderr,
			"warning: parser version changed (%s -> %s); running full regenerate\n",
			st.ParserVersion,
			state.CurrentParserVersion,
		)
		return generateContext(rootPath, nil, format, asJSON)
	}
	if st.OutputVersion != state.CurrentOutputVersion {
		fmt.Fprintf(
			os.Stderr,
			"warning: output schema version changed (%s -> %s); running full regenerate\n",
			st.OutputVersion,
			state.CurrentOutputVersion,
		)
		return generateContext(rootPath, nil, format, asJSON)
	}

	currentHashes, err := scanFileHashes(rootPath, registry, ignoreRules)
	if err != nil {
		return fmt.Errorf("failed to scan files: %w", err)
	}

	currentFiles := make(map[string]bool, len(currentHashes))
	for file := range currentHashes {
		currentFiles[file] = true
	}

	changed := st.ChangedFiles(currentHashes)
	deleted := st.DeletedFiles(currentFiles)

	// Backward compatibility: reparse files that have hash state but no cached symbols.
	for file, fileState := range st.Files {
		if currentFiles[file] && fileState.Language == "" && len(fileState.Symbols) == 0 {
			changed = append(changed, file)
		}
	}

	changed = dedupeStrings(changed)
	sort.Strings(changed)
	sort.Strings(deleted)

	if len(changed) == 0 && len(deleted) == 0 {
		rewritten := 0
		if outputsNeedRefresh(st, contextDir, format) {
			parseResult := parseResultFromState(st, rootPath, currentHashes)
			g := graph.BuildFromParseResult(parseResult)
			beforeOutputHashes := cloneOutputHashes(st.OutputHashes)

			writer := output.NewWriter(rootPath)
			if err := writer.WriteAll(g, parseResult, format); err != nil {
				return fmt.Errorf("failed to write output files: %w", err)
			}
			if err := writeNavigationIndex(contextDir, g); err != nil {
				return fmt.Errorf("failed to write navigation index: %w", err)
			}
			if err := recordOutputHashes(st, contextDir, format); err != nil {
				return fmt.Errorf("failed to update output hashes: %w", err)
			}
			if err := st.Save(contextDir); err != nil {
				return fmt.Errorf("failed to persist state: %w", err)
			}
			rewritten = countRewrittenOutputs(beforeOutputHashes, st.OutputHashes)
		}

		return printRunSummary(RunSummary{
			Mode:       "update",
			Format:     string(format),
			RootPath:   rootPath,
			OutputDir:  filepath.Join(rootPath, output.ContextDir),
			Scanned:    len(currentHashes),
			Parsed:     0,
			Reused:     len(currentHashes),
			Rewritten:  rewritten,
			Changed:    0,
			Deleted:    0,
			Impacted:   0,
			DurationMS: time.Since(start).Milliseconds(),
		}, asJSON)
	}

	for _, file := range changed {
		absPath := filepath.Join(rootPath, file)
		parsed, err := registry.ParseFile(absPath)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", file, err)
		}
		if parsed == nil {
			// No longer supported or ignored by parser rules.
			st.RemoveFile(file)
			continue
		}

		parsed.Path = file
		parsed.Hash = currentHashes[file]
		ensureSymbolIDs(parsed)
		st.SetFileData(*parsed)
	}

	for _, file := range deleted {
		st.RemoveFile(file)
	}

	impacted, reasons := impactedWithReasons(st, changed, deleted)
	sort.Strings(impacted)

	parseResult := parseResultFromState(st, rootPath, currentHashes)
	impactedExisting := existingFiles(impacted, currentHashes)
	impactedSet := toSet(impactedExisting)

	// Recompute dependency graph metadata only for impacted sources and preserve unchanged state.
	impactedGraph := graph.BuildFromParseResultForSources(parseResult, impactedSet)
	applyGraphDependencies(st, impactedGraph, impactedSet)

	// Build full graph for final outputs from the merged state snapshots.
	g := graph.BuildFromParseResult(parseResult)
	beforeOutputHashes := cloneOutputHashes(st.OutputHashes)

	writer := output.NewWriter(rootPath)
	if err := writer.WriteAll(g, parseResult, format); err != nil {
		return fmt.Errorf("failed to write output files: %w", err)
	}
	if err := writeNavigationIndex(contextDir, g); err != nil {
		return fmt.Errorf("failed to write navigation index: %w", err)
	}
	if err := recordOutputHashes(st, contextDir, format); err != nil {
		return fmt.Errorf("failed to update output hashes: %w", err)
	}

	if err := st.Save(contextDir); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	summary := RunSummary{
		Mode:          "update",
		Format:        string(format),
		RootPath:      rootPath,
		OutputDir:     filepath.Join(rootPath, output.ContextDir),
		Scanned:       len(currentHashes),
		Parsed:        len(changed),
		Reused:        maxInt(len(currentHashes)-len(changed), 0),
		Rewritten:     countRewrittenOutputs(beforeOutputHashes, st.OutputHashes),
		Changed:       len(changed),
		Deleted:       len(deleted),
		Impacted:      len(impacted),
		DurationMS:    time.Since(start).Milliseconds(),
		ChangedFiles:  changed,
		DeletedFiles:  deleted,
		ImpactedFiles: impacted,
	}
	if explain {
		summary.Reasons = reasons
	}
	return printRunSummary(summary, asJSON)
}

func runStatus(cmd *cobra.Command, args []string) error {
	start := time.Now()
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("failed to read --json flag: %w", err)
	}

	registry := languages.NewDefaultRegistry()
	ignoreRules, err := loadIgnoreRules(rootPath)
	if err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	st, err := state.Load(contextDir)
	if err != nil {
		if isCorruptStateError(err) {
			fmt.Fprintf(os.Stderr, "warning: corrupt state file detected (%v); treating all files as changed\n", err)
			st = state.NewState()
		} else {
			return fmt.Errorf("failed to load state: %w", err)
		}
	}

	currentHashes, err := scanFileHashes(rootPath, registry, ignoreRules)
	if err != nil {
		return fmt.Errorf("failed to scan files: %w", err)
	}

	currentFiles := make(map[string]bool, len(currentHashes))
	for file := range currentHashes {
		currentFiles[file] = true
	}

	changed := st.ChangedFiles(currentHashes)
	deleted := st.DeletedFiles(currentFiles)
	for file, fileState := range st.Files {
		if currentFiles[file] && fileState.Language == "" && len(fileState.Symbols) == 0 {
			changed = append(changed, file)
		}
	}
	changed = dedupeStrings(changed)
	sort.Strings(changed)
	sort.Strings(deleted)
	impacted, reasons := impactedWithReasons(st, changed, deleted)

	summary := RunSummary{
		Mode:          "status",
		RootPath:      rootPath,
		Scanned:       len(currentHashes),
		Parsed:        len(changed),
		Reused:        maxInt(len(currentHashes)-len(changed), 0),
		Rewritten:     0,
		Changed:       len(changed),
		Deleted:       len(deleted),
		Impacted:      len(impacted),
		DurationMS:    time.Since(start).Milliseconds(),
		ChangedFiles:  changed,
		DeletedFiles:  deleted,
		ImpactedFiles: impacted,
		Reasons:       reasons,
	}

	return printRunSummary(summary, asJSON)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := optionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	summary := DoctorSummary{
		Mode:         "doctor",
		RootPath:     rootPath,
		ContextDir:   contextDir,
		Format:       detectContextFormat(contextDir),
		Integrations: detectLLMIntegrations(rootPath),
		Clean:        false,
	}

	statePath := filepath.Join(contextDir, state.StateFile)
	hasState := fileExists(statePath)
	if !hasState {
		summary.Missing = append(summary.Missing, state.StateFile)
	}
	if summary.Format == "none" {
		summary.Missing = append(summary.Missing, "context artifacts")
	}

	if hasState {
		st, err := state.Load(contextDir)
		if err != nil {
			summary.Missing = append(summary.Missing, "valid state file")
			summary.Suggestions = append(summary.Suggestions, "run skelly generate")
		} else {
			registry := languages.NewDefaultRegistry()
			ignoreRules, err := loadIgnoreRules(rootPath)
			if err != nil {
				return err
			}
			currentHashes, err := scanFileHashes(rootPath, registry, ignoreRules)
			if err != nil {
				return fmt.Errorf("failed to scan files: %w", err)
			}
			currentFiles := make(map[string]bool, len(currentHashes))
			for file := range currentHashes {
				currentFiles[file] = true
			}

			changed := st.ChangedFiles(currentHashes)
			deleted := st.DeletedFiles(currentFiles)
			for file, fileState := range st.Files {
				if currentFiles[file] && fileState.Language == "" && len(fileState.Symbols) == 0 {
					changed = append(changed, file)
				}
			}

			summary.Changed = len(dedupeStrings(changed))
			summary.Deleted = len(deleted)
			summary.Clean = summary.Changed == 0 && summary.Deleted == 0
		}
	}

	if !summary.Integrations["skills"] {
		summary.Missing = append(summary.Missing, ".skelly/skills/skelly.md")
	}
	if !summary.Integrations["context"] {
		summary.Missing = append(summary.Missing, "CONTEXT.md managed block")
	}
	hasProviderAdapter := summary.Integrations["codex"] || summary.Integrations["claude"] || summary.Integrations["cursor"]
	if !hasProviderAdapter {
		summary.Missing = append(summary.Missing, "LLM adapter file")
	}

	if !hasState || summary.Format == "none" {
		summary.Suggestions = append(summary.Suggestions, "run skelly init && skelly generate")
	}
	if hasState && !summary.Clean {
		summary.Suggestions = append(summary.Suggestions, "run skelly update")
	}
	if !summary.Integrations["skills"] || !summary.Integrations["context"] || !hasProviderAdapter {
		summary.Suggestions = append(summary.Suggestions, "run skelly init --llm codex,claude,cursor")
	}

	summary.Missing = dedupeStrings(summary.Missing)
	sort.Strings(summary.Missing)
	summary.Suggestions = dedupeStrings(summary.Suggestions)
	sort.Strings(summary.Suggestions)
	summary.Healthy = summary.Clean && summary.Format != "none" && len(summary.Missing) == 0

	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}

	status := "issues"
	if summary.Healthy {
		status = "ok"
	}
	fmt.Printf("doctor: %s\n", status)
	fmt.Printf("context: format=%s clean=%t changed=%d deleted=%d\n", summary.Format, summary.Clean, summary.Changed, summary.Deleted)
	fmt.Printf("integrations: skills=%t context=%t codex=%t claude=%t cursor=%t\n",
		summary.Integrations["skills"],
		summary.Integrations["context"],
		summary.Integrations["codex"],
		summary.Integrations["claude"],
		summary.Integrations["cursor"],
	)
	if len(summary.Missing) > 0 {
		fmt.Printf("missing (%d): %s\n", len(summary.Missing), strings.Join(summary.Missing, ", "))
	}
	if len(summary.Suggestions) > 0 {
		for _, suggestion := range summary.Suggestions {
			fmt.Printf("next: %s\n", suggestion)
		}
	}
	return nil
}

type navigationIndex struct {
	Version string                `json:"version"`
	Nodes   []navigationIndexNode `json:"nodes"`
}

type navigationIndexNode struct {
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	Kind          string                     `json:"kind"`
	Signature     string                     `json:"signature,omitempty"`
	File          string                     `json:"file"`
	Line          int                        `json:"line"`
	OutEdges      []string                   `json:"out_edges,omitempty"`
	InEdges       []string                   `json:"in_edges,omitempty"`
	OutConfidence []navigationEdgeConfidence `json:"out_confidence,omitempty"`
}

type navigationEdgeConfidence struct {
	TargetID   string `json:"target_id"`
	Confidence string `json:"confidence,omitempty"`
}

type navigationLookup struct {
	byID   map[string]*navigationIndexNode
	byName map[string][]string
}

type navigationSymbolRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature,omitempty"`
	File      string `json:"file"`
	Line      int    `json:"line"`
}

type navigationEdgeRecord struct {
	Symbol     navigationSymbolRecord `json:"symbol"`
	Confidence string                 `json:"confidence,omitempty"`
}

type navigationTraceHop struct {
	Depth      int                    `json:"depth"`
	From       navigationSymbolRecord `json:"from"`
	To         navigationSymbolRecord `json:"to"`
	Confidence string                 `json:"confidence,omitempty"`
}

func writeNavigationIndex(contextDir string, g *graph.Graph) error {
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		return err
	}

	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	nodes := make([]navigationIndexNode, 0, len(ids))
	for _, id := range ids {
		node := g.Nodes[id]
		outConf := make([]navigationEdgeConfidence, 0, len(node.OutEdges))
		for _, targetID := range node.OutEdges {
			outConf = append(outConf, navigationEdgeConfidence{
				TargetID:   targetID,
				Confidence: node.OutEdgeConfidence[targetID],
			})
		}
		sort.Slice(outConf, func(i, j int) bool {
			return outConf[i].TargetID < outConf[j].TargetID
		})

		nodes = append(nodes, navigationIndexNode{
			ID:            node.ID,
			Name:          node.Symbol.Name,
			Kind:          node.Symbol.Kind.String(),
			Signature:     node.Symbol.Signature,
			File:          node.File,
			Line:          node.Symbol.Line,
			OutEdges:      append([]string(nil), node.OutEdges...),
			InEdges:       append([]string(nil), node.InEdges...),
			OutConfidence: outConf,
		})
	}

	index := navigationIndex{
		Version: "nav-index-v1",
		Nodes:   nodes,
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return writeFileIfChanged(filepath.Join(contextDir, navigationIndexFile), data)
}

func loadNavigationLookup(rootPath string) (*navigationLookup, error) {
	path := filepath.Join(rootPath, output.ContextDir, navigationIndexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("navigation index missing at %s (run skelly update)", path)
		}
		return nil, fmt.Errorf("failed to read navigation index: %w", err)
	}

	var index navigationIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to decode navigation index: %w", err)
	}

	lookup := &navigationLookup{
		byID:   make(map[string]*navigationIndexNode, len(index.Nodes)),
		byName: make(map[string][]string),
	}
	for i := range index.Nodes {
		node := &index.Nodes[i]
		lookup.byID[node.ID] = node
		lookup.byName[node.Name] = append(lookup.byName[node.Name], node.ID)
	}
	for name := range lookup.byName {
		sort.Strings(lookup.byName[name])
	}
	return lookup, nil
}

func (l *navigationLookup) resolve(query string) []*navigationIndexNode {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if node, ok := l.byID[query]; ok {
		return []*navigationIndexNode{node}
	}
	ids := l.byName[query]
	out := make([]*navigationIndexNode, 0, len(ids))
	for _, id := range ids {
		if node := l.byID[id]; node != nil {
			out = append(out, node)
		}
	}
	return out
}

func resolveSingleSymbol(l *navigationLookup, query string) (*navigationIndexNode, error) {
	matches := l.resolve(query)
	if len(matches) == 0 {
		return nil, fmt.Errorf("symbol %q not found", query)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	options := make([]string, 0, len(matches))
	for _, match := range matches {
		options = append(options, match.ID)
	}
	sort.Strings(options)
	return nil, fmt.Errorf("symbol %q is ambiguous; use one of: %s", query, strings.Join(options, ", "))
}

func symbolRecordFromNode(node *navigationIndexNode) navigationSymbolRecord {
	if node == nil {
		return navigationSymbolRecord{}
	}
	return navigationSymbolRecord{
		ID:        node.ID,
		Name:      node.Name,
		Kind:      node.Kind,
		Signature: node.Signature,
		File:      node.File,
		Line:      node.Line,
	}
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func runSymbol(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := optionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := loadNavigationLookup(rootPath)
	if err != nil {
		return err
	}
	matches := lookup.resolve(args[0])
	if len(matches) == 0 {
		return fmt.Errorf("symbol %q not found", args[0])
	}

	records := make([]navigationSymbolRecord, 0, len(matches))
	for _, match := range matches {
		records = append(records, symbolRecordFromNode(match))
	}

	if asJSON {
		return printJSON(map[string]any{
			"query":   args[0],
			"matches": records,
		})
	}

	fmt.Printf("symbol matches for %q (%d)\n", args[0], len(records))
	for _, record := range records {
		fmt.Printf("- %s [%s] %s:%d\n", record.ID, record.Kind, record.File, record.Line)
		if record.Signature != "" {
			fmt.Printf("  sig: %s\n", record.Signature)
		}
	}
	return nil
}

func collectCallers(l *navigationLookup, node *navigationIndexNode) []navigationEdgeRecord {
	out := make([]navigationEdgeRecord, 0, len(node.InEdges))
	for _, callerID := range node.InEdges {
		caller := l.byID[callerID]
		if caller == nil {
			continue
		}
		out = append(out, navigationEdgeRecord{
			Symbol:     symbolRecordFromNode(caller),
			Confidence: l.edgeConfidence(caller.ID, node.ID),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Symbol.ID < out[j].Symbol.ID
	})
	return out
}

func collectCallees(l *navigationLookup, node *navigationIndexNode) []navigationEdgeRecord {
	out := make([]navigationEdgeRecord, 0, len(node.OutEdges))
	for _, calleeID := range node.OutEdges {
		callee := l.byID[calleeID]
		if callee == nil {
			continue
		}
		out = append(out, navigationEdgeRecord{
			Symbol:     symbolRecordFromNode(callee),
			Confidence: l.edgeConfidence(node.ID, callee.ID),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Symbol.ID < out[j].Symbol.ID
	})
	return out
}

func (l *navigationLookup) edgeConfidence(fromID, toID string) string {
	from := l.byID[fromID]
	if from == nil {
		return ""
	}
	for _, item := range from.OutConfidence {
		if item.TargetID == toID {
			return item.Confidence
		}
	}
	return ""
}

func runCallers(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := optionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := loadNavigationLookup(rootPath)
	if err != nil {
		return err
	}
	node, err := resolveSingleSymbol(lookup, args[0])
	if err != nil {
		return err
	}

	callers := collectCallers(lookup, node)
	if asJSON {
		return printJSON(map[string]any{
			"query":   args[0],
			"symbol":  symbolRecordFromNode(node),
			"callers": callers,
		})
	}

	fmt.Printf("callers for %s (%d)\n", node.ID, len(callers))
	if len(callers) == 0 {
		fmt.Println("no callers found")
		return nil
	}
	for _, caller := range callers {
		fmt.Printf("- %s [%s] %s:%d", caller.Symbol.ID, caller.Symbol.Kind, caller.Symbol.File, caller.Symbol.Line)
		if caller.Confidence != "" {
			fmt.Printf(" (%s)", caller.Confidence)
		}
		fmt.Println()
	}
	return nil
}

func runCallees(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := optionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := loadNavigationLookup(rootPath)
	if err != nil {
		return err
	}
	node, err := resolveSingleSymbol(lookup, args[0])
	if err != nil {
		return err
	}

	callees := collectCallees(lookup, node)
	if asJSON {
		return printJSON(map[string]any{
			"query":   args[0],
			"symbol":  symbolRecordFromNode(node),
			"callees": callees,
		})
	}

	fmt.Printf("callees for %s (%d)\n", node.ID, len(callees))
	if len(callees) == 0 {
		fmt.Println("no callees found")
		return nil
	}
	for _, callee := range callees {
		fmt.Printf("- %s [%s] %s:%d", callee.Symbol.ID, callee.Symbol.Kind, callee.Symbol.File, callee.Symbol.Line)
		if callee.Confidence != "" {
			fmt.Printf(" (%s)", callee.Confidence)
		}
		fmt.Println()
	}
	return nil
}

func runTrace(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	depth, err := cmd.Flags().GetInt("depth")
	if err != nil {
		return fmt.Errorf("failed to read --depth flag: %w", err)
	}
	if depth < 1 {
		return fmt.Errorf("--depth must be >= 1")
	}
	asJSON, err := optionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := loadNavigationLookup(rootPath)
	if err != nil {
		return err
	}
	startNode, err := resolveSingleSymbol(lookup, args[0])
	if err != nil {
		return err
	}

	type queueItem struct {
		id    string
		depth int
	}
	queue := []queueItem{{id: startNode.ID, depth: 0}}
	seenDepth := map[string]int{startNode.ID: 0}
	hops := make([]navigationTraceHop, 0)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= depth {
			continue
		}

		fromNode := lookup.byID[current.id]
		if fromNode == nil {
			continue
		}
		for _, nextID := range fromNode.OutEdges {
			toNode := lookup.byID[nextID]
			if toNode == nil {
				continue
			}
			nextDepth := current.depth + 1
			hops = append(hops, navigationTraceHop{
				Depth:      nextDepth,
				From:       symbolRecordFromNode(fromNode),
				To:         symbolRecordFromNode(toNode),
				Confidence: lookup.edgeConfidence(fromNode.ID, toNode.ID),
			})
			if previousDepth, exists := seenDepth[nextID]; !exists || nextDepth < previousDepth {
				seenDepth[nextID] = nextDepth
				queue = append(queue, queueItem{id: nextID, depth: nextDepth})
			}
		}
	}

	sort.Slice(hops, func(i, j int) bool {
		if hops[i].Depth != hops[j].Depth {
			return hops[i].Depth < hops[j].Depth
		}
		if hops[i].From.ID != hops[j].From.ID {
			return hops[i].From.ID < hops[j].From.ID
		}
		return hops[i].To.ID < hops[j].To.ID
	})

	if asJSON {
		return printJSON(map[string]any{
			"query": args[0],
			"start": symbolRecordFromNode(startNode),
			"depth": depth,
			"hops":  hops,
		})
	}

	fmt.Printf("trace from %s depth=%d hops=%d\n", startNode.ID, depth, len(hops))
	if len(hops) == 0 {
		fmt.Println("no outgoing hops found")
		return nil
	}
	for _, hop := range hops {
		fmt.Printf("- d=%d %s -> %s", hop.Depth, hop.From.ID, hop.To.ID)
		if hop.Confidence != "" {
			fmt.Printf(" (%s)", hop.Confidence)
		}
		fmt.Println()
	}
	return nil
}

func runPath(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := optionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := loadNavigationLookup(rootPath)
	if err != nil {
		return err
	}
	fromNode, err := resolveSingleSymbol(lookup, args[0])
	if err != nil {
		return err
	}
	toNode, err := resolveSingleSymbol(lookup, args[1])
	if err != nil {
		return err
	}

	pathIDs := shortestPath(lookup, fromNode.ID, toNode.ID)
	if len(pathIDs) == 0 {
		return fmt.Errorf("no path found between %s and %s", fromNode.ID, toNode.ID)
	}

	pathNodes := make([]navigationSymbolRecord, 0, len(pathIDs))
	edges := make([]map[string]string, 0, len(pathIDs)-1)
	for i, id := range pathIDs {
		node := lookup.byID[id]
		if node == nil {
			continue
		}
		pathNodes = append(pathNodes, symbolRecordFromNode(node))
		if i == 0 {
			continue
		}
		prevID := pathIDs[i-1]
		edges = append(edges, map[string]string{
			"from_id":    prevID,
			"to_id":      id,
			"confidence": lookup.edgeConfidence(prevID, id),
		})
	}

	if asJSON {
		return printJSON(map[string]any{
			"from":   symbolRecordFromNode(fromNode),
			"to":     symbolRecordFromNode(toNode),
			"length": len(pathNodes) - 1,
			"path":   pathNodes,
			"edges":  edges,
		})
	}

	fmt.Printf("path %s -> %s length=%d\n", fromNode.ID, toNode.ID, len(pathNodes)-1)
	for i, node := range pathNodes {
		fmt.Printf("%d. %s [%s] %s:%d\n", i+1, node.ID, node.Kind, node.File, node.Line)
	}
	return nil
}

func shortestPath(lookup *navigationLookup, fromID, toID string) []string {
	if fromID == toID {
		return []string{fromID}
	}

	queue := []string{fromID}
	visited := map[string]bool{fromID: true}
	parent := map[string]string{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		node := lookup.byID[current]
		if node == nil {
			continue
		}
		for _, nextID := range node.OutEdges {
			if visited[nextID] {
				continue
			}
			visited[nextID] = true
			parent[nextID] = current
			if nextID == toID {
				return reconstructPath(parent, fromID, toID)
			}
			queue = append(queue, nextID)
		}
	}

	return nil
}

func reconstructPath(parent map[string]string, fromID, toID string) []string {
	out := []string{toID}
	for current := toID; current != fromID; {
		prev, ok := parent[current]
		if !ok {
			return nil
		}
		out = append(out, prev)
		current = prev
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func optionalBoolFlag(cmd *cobra.Command, name string, defaultValue bool) (bool, error) {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return defaultValue, nil
	}
	value, err := cmd.Flags().GetBool(name)
	if err != nil {
		return false, fmt.Errorf("failed to read --%s flag: %w", name, err)
	}
	return value, nil
}

func detectContextFormat(contextDir string) string {
	hasText := fileExists(filepath.Join(contextDir, output.IndexFile)) &&
		fileExists(filepath.Join(contextDir, output.GraphFile))
	hasJSONL := fileExists(filepath.Join(contextDir, output.SymbolsFile)) &&
		fileExists(filepath.Join(contextDir, output.EdgesFile)) &&
		fileExists(filepath.Join(contextDir, output.ManifestFile))

	switch {
	case hasText && hasJSONL:
		return "mixed"
	case hasJSONL:
		return string(output.FormatJSONL)
	case hasText:
		return string(output.FormatText)
	default:
		return "none"
	}
}

func detectLLMIntegrations(rootPath string) map[string]bool {
	return map[string]bool{
		"skills":  fileExists(filepath.Join(rootPath, ".skelly", "skills", "skelly.md")),
		"context": containsManagedBlock(filepath.Join(rootPath, "CONTEXT.md")),
		"codex":   containsManagedBlock(filepath.Join(rootPath, "AGENTS.md")),
		"claude":  containsManagedBlock(filepath.Join(rootPath, "CLAUDE.md")),
		"cursor":  fileExists(filepath.Join(rootPath, ".cursor", "rules", "skelly-context.mdc")),
	}
}

func containsManagedBlock(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, llmManagedBlockStart) && strings.Contains(text, llmManagedBlockEnd)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type enrichScope string

const (
	enrichScopeTarget enrichScope = "target"
	enrichOutputFile              = "enrich.jsonl"
)

type enrichRecord struct {
	SymbolID      string             `json:"symbol_id"`
	Agent         string             `json:"agent"`
	AgentProfile  string             `json:"agent_profile,omitempty"`
	Model         string             `json:"model,omitempty"`
	PromptVersion string             `json:"prompt_version,omitempty"`
	CacheKey      string             `json:"cache_key,omitempty"`
	Scope         string             `json:"scope"`
	FileHash      string             `json:"file_hash"`
	Input         enrichInputPayload `json:"input"`
	Output        enrichOutput       `json:"output,omitempty"`
	Status        string             `json:"status,omitempty"`
	Error         string             `json:"error,omitempty"`
	GeneratedAt   string             `json:"generated_at,omitempty"`
	UpdatedAt     string             `json:"updated_at,omitempty"`
}

type enrichInputPayload struct {
	Symbol   enrichSymbolMetadata `json:"symbol"`
	Source   enrichSourceSpan     `json:"source"`
	Imports  []string             `json:"imports,omitempty"`
	Calls    []string             `json:"calls,omitempty"`
	CalledBy []string             `json:"called_by,omitempty"`
}

type enrichSymbolMetadata struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Path      string `json:"path"`
	Language  string `json:"language"`
	Line      int    `json:"line"`
}

type enrichSourceSpan struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Body      string `json:"body,omitempty"`
}

type enrichOutput struct {
	Summary     string `json:"summary"`
	Purpose     string `json:"purpose"`
	SideEffects string `json:"side_effects"`
	Confidence  string `json:"confidence"`
}

func runEnrich(cmd *cobra.Command, args []string) error {
	start := time.Now()
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	targetSelector := strings.TrimSpace(args[0])
	description := strings.TrimSpace(strings.Join(args[1:], " "))
	if description == "" {
		return fmt.Errorf("description is required")
	}
	outputPayload := enrichOutput{
		Summary:     description,
		Purpose:     description,
		SideEffects: "Unknown from static analysis.",
		Confidence:  "medium",
	}
	if err := validateEnrichOutput(outputPayload); err != nil {
		return fmt.Errorf("invalid enrich output: %w", err)
	}

	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("failed to read --json flag: %w", err)
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	st, err := state.Load(contextDir)
	if err != nil {
		if isCorruptStateError(err) {
			return fmt.Errorf("state is corrupt; run `skelly generate` first")
		}
		return fmt.Errorf("failed to load state: %w", err)
	}
	if len(st.Files) == 0 {
		return fmt.Errorf("no indexed files found; run `skelly generate` first")
	}

	registry := languages.NewDefaultRegistry()
	ignoreRules, err := loadIgnoreRules(rootPath)
	if err != nil {
		return err
	}
	currentHashes, err := scanFileHashes(rootPath, registry, ignoreRules)
	if err != nil {
		return fmt.Errorf("failed to scan files: %w", err)
	}
	targetFiles := make([]string, 0, len(currentHashes))
	for file := range currentHashes {
		targetFiles = append(targetFiles, file)
	}
	sort.Strings(targetFiles)
	if len(targetFiles) == 0 {
		return printEnrichSummary(EnrichRunSummary{
			Mode:       "enrich",
			Agent:      "agent",
			Scope:      string(enrichScopeTarget),
			Target:     targetSelector,
			RootPath:   rootPath,
			OutputFile: filepath.Join(contextDir, enrichOutputFile),
			Files:      0,
			Symbols:    0,
			DurationMS: time.Since(start).Milliseconds(),
		}, asJSON)
	}

	parseResult := parseResultFromState(st, rootPath, currentHashes)
	g := graph.BuildFromParseResult(parseResult)
	cachePath := filepath.Join(contextDir, enrichOutputFile)
	cacheRecords, err := loadEnrichCache(cachePath)
	if err != nil {
		return err
	}

	workItems := collectEnrichWorkItems(targetFiles, st, g)
	workItems = filterEnrichWorkItems(workItems, targetSelector)
	if len(workItems) == 0 {
		return fmt.Errorf(
			"no symbols matched enrich target %q (try file path, file:symbol, file:line, or stable symbol id)",
			targetSelector,
		)
	}
	if len(workItems) > 1 {
		return fmt.Errorf(
			"enrich target %q matched %d symbols; be more specific. matches: %s",
			targetSelector,
			len(workItems),
			summarizeEnrichMatches(workItems, 5),
		)
	}

	item := workItems[0]
	lineCache := make(map[string][]string)
	record, ok := buildEnrichRecord(
		rootPath,
		item.File,
		item.FileState,
		item.Symbol,
		item.Node,
		lineCache,
		"agent",
		enrichScopeTarget,
	)
	if !ok {
		return fmt.Errorf("target %q could not be enriched", targetSelector)
	}
	record.AgentProfile = "agent"
	record.Model = "manual"
	record.PromptVersion = "agent-note-v1"
	record.CacheKey = enrichCacheKey(record.SymbolID, record.FileHash, record.PromptVersion, record.AgentProfile, record.Model)
	record.Output = outputPayload
	record.Status = "success"
	record.Error = ""
	timestamp := time.Now().UTC().Format(time.RFC3339)
	record.GeneratedAt = timestamp
	record.UpdatedAt = timestamp

	cacheHits := 0
	cacheMisses := 1
	if existing, exists := cacheRecords[record.CacheKey]; exists {
		cacheHits = 1
		cacheMisses = 0
		if existing.GeneratedAt != "" {
			record.GeneratedAt = existing.GeneratedAt
		}
	}

	cacheRecords[record.CacheKey] = record
	pruneEnrichCacheForSymbol(cacheRecords, record.CacheKey, record.SymbolID, record.AgentProfile)
	if err := writeEnrichCache(cachePath, cacheRecords); err != nil {
		return err
	}

	return printEnrichSummary(EnrichRunSummary{
		Mode:        "enrich",
		Agent:       "agent",
		Scope:       string(enrichScopeTarget),
		Target:      targetSelector,
		RootPath:    rootPath,
		OutputFile:  cachePath,
		Files:       1,
		Symbols:     1,
		Succeeded:   1,
		Failed:      0,
		CacheHits:   cacheHits,
		CacheMisses: cacheMisses,
		DurationMS:  time.Since(start).Milliseconds(),
		Targets:     []string{item.File},
	}, asJSON)
}

func validateEnrichOutput(output enrichOutput) error {
	if strings.TrimSpace(output.Summary) == "" {
		return fmt.Errorf("missing output.summary")
	}
	if strings.TrimSpace(output.Purpose) == "" {
		return fmt.Errorf("missing output.purpose")
	}
	if strings.TrimSpace(output.SideEffects) == "" {
		return fmt.Errorf("missing output.side_effects")
	}
	confidence := strings.ToLower(strings.TrimSpace(output.Confidence))
	switch confidence {
	case "low", "medium", "high":
		return nil
	default:
		return fmt.Errorf("output.confidence must be one of low|medium|high")
	}
}

func cloneSymbolsSorted(file string, symbols []parser.Symbol) []parser.Symbol {
	out := append([]parser.Symbol(nil), symbols...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line == out[j].Line {
			leftID := out[i].ID
			if leftID == "" {
				leftID = parser.StableSymbolID(file, out[i])
			}
			rightID := out[j].ID
			if rightID == "" {
				rightID = parser.StableSymbolID(file, out[j])
			}
			return leftID < rightID
		}
		return out[i].Line < out[j].Line
	})
	return out
}

type enrichWorkItem struct {
	File      string
	FileState state.FileState
	Symbol    parser.Symbol
	Node      *graph.Node
}

func collectEnrichWorkItems(targetFiles []string, st *state.State, g *graph.Graph) []enrichWorkItem {
	items := make([]enrichWorkItem, 0)
	for _, file := range targetFiles {
		fileState, ok := st.Files[file]
		if !ok {
			continue
		}
		symbols := cloneSymbolsSorted(file, fileState.Symbols)
		for _, sym := range symbols {
			node := g.Nodes[sym.ID]
			items = append(items, enrichWorkItem{
				File:      file,
				FileState: fileState,
				Symbol:    sym,
				Node:      node,
			})
		}
	}
	return items
}

func filterEnrichWorkItems(items []enrichWorkItem, selector string) []enrichWorkItem {
	normalizedSelector := normalizeEnrichSelector(selector)
	if normalizedSelector == "" {
		return items
	}

	filtered := make([]enrichWorkItem, 0, len(items))
	for _, item := range items {
		if matchesEnrichSelector(item, normalizedSelector) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func matchesEnrichSelector(item enrichWorkItem, selector string) bool {
	file := normalizeEnrichSelector(item.File)
	symbolID := normalizeEnrichSelector(item.Symbol.ID)
	name := strings.ToLower(strings.TrimSpace(item.Symbol.Name))
	line := strconv.Itoa(item.Symbol.Line)

	candidates := []string{
		file,
		symbolID,
		name,
		file + ":" + name,
		file + ":" + line,
		file + ":" + line + ":" + name,
	}
	for _, candidate := range candidates {
		if selector == candidate {
			return true
		}
	}

	if strings.HasPrefix(file, selector) || strings.HasSuffix(file, selector) {
		return true
	}
	return strings.Contains(name, selector)
}

func normalizeEnrichSelector(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	normalized = filepath.ToSlash(normalized)
	normalized = strings.TrimPrefix(normalized, "./")
	return strings.ToLower(normalized)
}

func summarizeEnrichMatches(items []enrichWorkItem, limit int) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = len(items)
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprintf("%s:%d:%s", item.File, item.Symbol.Line, item.Symbol.Name))
	}
	sort.Strings(names)
	if len(names) <= limit {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:limit], ", ") + fmt.Sprintf(", ... (+%d more)", len(names)-limit)
}

func enrichCacheKey(symbolID, fileHash, promptVersion, agentProfile, model string) string {
	seed := strings.Join([]string{
		strings.TrimSpace(symbolID),
		strings.TrimSpace(fileHash),
		strings.TrimSpace(promptVersion),
		strings.TrimSpace(agentProfile),
		strings.TrimSpace(model),
	}, "|")
	sum := sha256.Sum256([]byte(seed))
	return "cache-v1-" + hex.EncodeToString(sum[:8])
}

func loadEnrichCache(path string) (map[string]enrichRecord, error) {
	cache := make(map[string]enrichRecord)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return nil, fmt.Errorf("failed to read enrich cache: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var record enrichRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.AgentProfile == "" {
			record.AgentProfile = record.Agent
		}
		if record.PromptVersion == "" {
			record.PromptVersion = "prompt-v1-legacy"
		}
		if record.Model == "" {
			record.Model = "unknown"
		}
		if record.CacheKey == "" {
			record.CacheKey = enrichCacheKey(record.SymbolID, record.FileHash, record.PromptVersion, record.AgentProfile, record.Model)
		}
		if record.CacheKey == "" {
			continue
		}

		existing, exists := cache[record.CacheKey]
		if !exists {
			cache[record.CacheKey] = record
			continue
		}
		existingTS := existing.UpdatedAt
		if existingTS == "" {
			existingTS = existing.GeneratedAt
		}
		recordTS := record.UpdatedAt
		if recordTS == "" {
			recordTS = record.GeneratedAt
		}
		if recordTS >= existingTS {
			cache[record.CacheKey] = record
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse enrich cache: %w", err)
	}
	return cache, nil
}

func writeEnrichCache(path string, cache map[string]enrichRecord) error {
	records := make([]enrichRecord, 0, len(cache))
	for _, record := range cache {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].SymbolID == records[j].SymbolID {
			if records[i].AgentProfile == records[j].AgentProfile {
				return records[i].CacheKey < records[j].CacheKey
			}
			return records[i].AgentProfile < records[j].AgentProfile
		}
		return records[i].SymbolID < records[j].SymbolID
	})

	data, err := encodeJSONLines(records)
	if err != nil {
		return fmt.Errorf("failed to encode enrich output: %w", err)
	}
	if err := writeFileIfChanged(path, data); err != nil {
		return fmt.Errorf("failed to write enrich output: %w", err)
	}
	return nil
}

func pruneEnrichCacheForSymbol(cache map[string]enrichRecord, keepKey, symbolID, agentProfile string) {
	for key, record := range cache {
		if key == keepKey {
			continue
		}
		if record.SymbolID != symbolID {
			continue
		}
		profile := record.AgentProfile
		if profile == "" {
			profile = record.Agent
		}
		if profile != agentProfile {
			continue
		}
		delete(cache, key)
	}
}

func buildEnrichRecord(
	rootPath string,
	file string,
	fileState state.FileState,
	sym parser.Symbol,
	node *graph.Node,
	lineCache map[string][]string,
	agent string,
	scope enrichScope,
) (enrichRecord, bool) {
	if sym.ID == "" {
		sym.ID = parser.StableSymbolID(file, sym)
	}

	sourceLine := readSourceLine(rootPath, file, sym.Line, lineCache)
	calls := make([]string, 0)
	calledBy := make([]string, 0)
	if node != nil {
		calls = append(calls, node.OutEdges...)
		calledBy = append(calledBy, node.InEdges...)
		sort.Strings(calls)
		sort.Strings(calledBy)
	}

	record := enrichRecord{
		SymbolID:     sym.ID,
		Agent:        agent,
		AgentProfile: agent,
		Scope:        string(scope),
		FileHash:     fileState.Hash,
		Status:       "pending",
		Input: enrichInputPayload{
			Symbol: enrichSymbolMetadata{
				ID:        sym.ID,
				Name:      sym.Name,
				Kind:      sym.Kind.String(),
				Signature: sym.Signature,
				Path:      file,
				Language:  fileState.Language,
				Line:      sym.Line,
			},
			Source: enrichSourceSpan{
				StartLine: sym.Line,
				EndLine:   sym.Line,
				Body:      sourceLine,
			},
			Imports:  append([]string(nil), fileState.Imports...),
			Calls:    calls,
			CalledBy: calledBy,
		},
	}
	return record, true
}

func readSourceLine(rootPath, file string, line int, lineCache map[string][]string) string {
	if line <= 0 {
		return ""
	}
	lines, ok := lineCache[file]
	if !ok {
		data, err := os.ReadFile(filepath.Join(rootPath, file))
		if err != nil {
			lineCache[file] = nil
			return ""
		}
		lines = strings.Split(string(data), "\n")
		lineCache[file] = lines
	}
	if line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}

func printEnrichSummary(summary EnrichRunSummary, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}

	mode := "enrich"
	if summary.DryRun {
		mode = "enrich (dry-run)"
	}
	parts := []string{
		fmt.Sprintf("%s:", mode),
		fmt.Sprintf("agent=%s", summary.Agent),
		fmt.Sprintf("scope=%s", summary.Scope),
	}
	if summary.Target != "" {
		parts = append(parts, fmt.Sprintf("target=%s", summary.Target))
	}
	parts = append(parts,
		fmt.Sprintf("files=%d", summary.Files),
		fmt.Sprintf("symbols=%d", summary.Symbols),
		fmt.Sprintf("succeeded=%d", summary.Succeeded),
		fmt.Sprintf("failed=%d", summary.Failed),
		fmt.Sprintf("cache_hits=%d", summary.CacheHits),
		fmt.Sprintf("cache_misses=%d", summary.CacheMisses),
		fmt.Sprintf("duration=%dms", summary.DurationMS),
	)
	fmt.Println(strings.Join(parts, " "))
	if summary.OutputFile != "" {
		fmt.Printf("output: %s\n", summary.OutputFile)
	}
	if len(summary.Targets) > 0 {
		fmt.Printf("targets (%d): %s\n", len(summary.Targets), summarizePaths(summary.Targets, 8))
	}
	return nil
}

func scanFileHashes(rootPath string, registry *parser.Registry, ignoreRules []string) (map[string]string, error) {
	hashes := make(map[string]string)
	ignoreMatcher := ignore.NewMatcher(ignoreRules)

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		if ignoreMatcher.ShouldIgnore(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if _, ok := registry.GetParserForFile(path); !ok {
			return nil
		}

		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		hashes[relPath] = hash

		return nil
	})

	return hashes, err
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

func parseResultFromState(st *state.State, rootPath string, currentHashes map[string]string) *parser.ParseResult {
	files := make([]parser.FileSymbols, 0, len(currentHashes))

	for path, hash := range currentHashes {
		fileState, ok := st.Files[path]
		if !ok {
			continue
		}

		files = append(files, parser.FileSymbols{
			Path:          path,
			Language:      fileState.Language,
			Symbols:       fileState.Symbols,
			Imports:       fileState.Imports,
			ImportAliases: fileState.ImportAliases,
			Hash:          hash,
		})
		ensureSymbolIDs(&files[len(files)-1])
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return &parser.ParseResult{
		Files:    files,
		RootPath: rootPath,
	}
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func ensureSymbolIDs(file *parser.FileSymbols) {
	for i := range file.Symbols {
		if file.Symbols[i].ID != "" {
			continue
		}
		file.Symbols[i].ID = parser.StableSymbolID(file.Path, file.Symbols[i])
	}
}

func applyGraphDependencies(st *state.State, g *graph.Graph, files map[string]bool) {
	targetFiles := make([]string, 0)
	if files != nil {
		for file := range files {
			targetFiles = append(targetFiles, file)
		}
	} else {
		for file := range st.Files {
			targetFiles = append(targetFiles, file)
		}
	}
	sort.Strings(targetFiles)

	for _, file := range targetFiles {
		fileState, ok := st.Files[file]
		if !ok {
			continue
		}

		nodeIDs := g.FileNodes[file]
		deps := make(map[string]bool)
		for _, nodeID := range nodeIDs {
			node, ok := g.Nodes[nodeID]
			if !ok {
				continue
			}

			for _, edge := range node.OutEdges {
				targetFile, _ := graph.ParseNodeID(edge)
				if targetFile == "" || targetFile == file {
					continue
				}
				deps[targetFile] = true
			}
		}

		fileState.Dependencies = mapKeysSorted(deps)
		st.Files[file] = fileState
	}
}

func existingFiles(paths []string, existing map[string]string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := existing[path]; ok {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func toSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, path := range paths {
		set[path] = true
	}
	return set
}

func mapKeysSorted(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func impactedWithReasons(st *state.State, changed, deleted []string) ([]string, map[string][]string) {
	reverseDeps := make(map[string][]string)
	for file, fileState := range st.Files {
		for _, dep := range fileState.Dependencies {
			reverseDeps[dep] = append(reverseDeps[dep], file)
		}
	}
	for file := range reverseDeps {
		sort.Strings(reverseDeps[file])
	}

	reasons := make(map[string][]string)
	seen := make(map[string]bool)
	queue := make([]string, 0, len(changed)+len(deleted))

	for _, file := range changed {
		queue = append(queue, file)
		seen[file] = true
		reasons[file] = appendReason(reasons[file], "changed")
	}
	for _, file := range deleted {
		if !seen[file] {
			queue = append(queue, file)
		}
		seen[file] = true
		reasons[file] = appendReason(reasons[file], "deleted")
	}

	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]

		for _, dependent := range reverseDeps[file] {
			reason := "depends on " + file
			reasons[dependent] = appendReason(reasons[dependent], reason)
			if seen[dependent] {
				continue
			}
			seen[dependent] = true
			queue = append(queue, dependent)
		}
	}

	// Also include files that reference names declared in changed files, even if previous
	// dependency metadata did not capture them (e.g. newly introduced symbols).
	changedNames := make(map[string]bool)
	for _, file := range changed {
		fileState, ok := st.Files[file]
		if !ok {
			continue
		}
		for _, sym := range fileState.Symbols {
			if sym.Name != "" {
				changedNames[sym.Name] = true
			}
		}
	}

	for file, fileState := range st.Files {
		if seen[file] || len(changedNames) == 0 {
			continue
		}

		matchedName := ""
		for _, sym := range fileState.Symbols {
			for _, call := range sym.Calls {
				if changedNames[call.Name] {
					matchedName = call.Name
					break
				}
			}
			if matchedName != "" {
				break
			}
		}
		if matchedName == "" {
			continue
		}

		seen[file] = true
		reasons[file] = appendReason(reasons[file], "calls changed symbol "+matchedName)
	}

	impacted := make([]string, 0, len(seen))
	for file := range seen {
		impacted = append(impacted, file)
		sort.Strings(reasons[file])
	}
	sort.Strings(impacted)
	return impacted, reasons
}

func recordOutputHashes(st *state.State, contextDir string, format output.Format) error {
	st.OutputHashes = make(map[string]string)

	var outputPaths []string
	switch format {
	case output.FormatText:
		outputPaths = []string{
			filepath.Join(contextDir, output.IndexFile),
			filepath.Join(contextDir, output.GraphFile),
		}

		moduleFiles, err := filepath.Glob(filepath.Join(contextDir, output.ModulesDir, "*.txt"))
		if err != nil {
			return err
		}
		outputPaths = append(outputPaths, moduleFiles...)
	case output.FormatJSONL:
		outputPaths = []string{
			filepath.Join(contextDir, output.SymbolsFile),
			filepath.Join(contextDir, output.EdgesFile),
			filepath.Join(contextDir, output.ManifestFile),
		}
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
	outputPaths = append(outputPaths, filepath.Join(contextDir, navigationIndexFile))

	for _, outputPath := range outputPaths {
		hash, err := hashFile(outputPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		relPath, err := filepath.Rel(contextDir, outputPath)
		if err != nil {
			return err
		}
		st.SetOutputHash(relPath, hash)
	}

	return nil
}

func isCorruptStateError(err error) bool {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr)
}

func loadOutputHashesFromState(contextDir string) (map[string]string, error) {
	st, err := state.Load(contextDir)
	if err != nil {
		return nil, err
	}
	return cloneOutputHashes(st.OutputHashes), nil
}

func cloneOutputHashes(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func countRewrittenOutputs(before, after map[string]string) int {
	rewritten := 0
	seen := make(map[string]bool, len(before)+len(after))
	for file := range before {
		seen[file] = true
	}
	for file := range after {
		seen[file] = true
	}
	for file := range seen {
		if before[file] != after[file] {
			rewritten++
		}
	}
	return rewritten
}

func collectPaths(files []parser.FileSymbols) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return paths
}

func appendReason(existing []string, reason string) []string {
	for _, item := range existing {
		if item == reason {
			return existing
		}
	}
	return append(existing, reason)
}

func outputsNeedRefresh(st *state.State, contextDir string, format output.Format) bool {
	for _, file := range requiredOutputFiles(format) {
		if _, ok := st.OutputHashes[file]; !ok {
			return true
		}
		if _, err := os.Stat(filepath.Join(contextDir, file)); err != nil {
			return true
		}
	}
	return false
}

func requiredOutputFiles(format output.Format) []string {
	switch format {
	case output.FormatText:
		return []string{output.IndexFile, output.GraphFile, navigationIndexFile}
	case output.FormatJSONL:
		return []string{output.SymbolsFile, output.EdgesFile, output.ManifestFile, navigationIndexFile}
	default:
		return nil
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func printRunSummary(summary RunSummary, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}

	if summary.Mode == "generate" {
		fmt.Printf("generate complete in %dms\n", summary.DurationMS)
		if summary.OutputDir != "" {
			if summary.Format != "" {
				fmt.Printf("output: %s (%s)\n", summary.OutputDir, summary.Format)
			} else {
				fmt.Printf("output: %s\n", summary.OutputDir)
			}
		}
		fmt.Printf("files: scanned=%d parsed=%d reused=%d\n", summary.Scanned, summary.Parsed, summary.Reused)
		fmt.Printf("changes: changed=%d deleted=%d impacted=%d rewritten=%d\n", summary.Changed, summary.Deleted, summary.Impacted, summary.Rewritten)
		if len(summary.ChangedFiles) > 0 {
			fmt.Printf("changed files (%d): %s\n", len(summary.ChangedFiles), summarizePaths(summary.ChangedFiles, 8))
		}
		return nil
	}

	fmt.Printf(
		"%s: scanned=%d parsed=%d reused=%d rewritten=%d changed=%d deleted=%d impacted=%d duration=%dms\n",
		summary.Mode,
		summary.Scanned,
		summary.Parsed,
		summary.Reused,
		summary.Rewritten,
		summary.Changed,
		summary.Deleted,
		summary.Impacted,
		summary.DurationMS,
	)

	if len(summary.ChangedFiles) > 0 {
		fmt.Printf("changed files (%d): %s\n", len(summary.ChangedFiles), summarizePaths(summary.ChangedFiles, 8))
	}
	if len(summary.DeletedFiles) > 0 {
		fmt.Printf("deleted files (%d): %s\n", len(summary.DeletedFiles), summarizePaths(summary.DeletedFiles, 8))
	}
	if len(summary.ImpactedFiles) > 0 && summary.Mode != "generate" {
		fmt.Printf("impacted files (%d): %s\n", len(summary.ImpactedFiles), summarizePaths(summary.ImpactedFiles, 8))
	}
	if len(summary.Reasons) > 0 {
		for _, file := range summary.ImpactedFiles {
			reasons := summary.Reasons[file]
			if len(reasons) == 0 {
				continue
			}
			fmt.Printf("  %s <- %s\n", file, strings.Join(reasons, "; "))
		}
	}

	return nil
}

func summarizePaths(paths []string, max int) string {
	if len(paths) <= max {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s ... (+%d more)", strings.Join(paths[:max], ", "), len(paths)-max)
}

func encodeJSONLines[T any](records []T) ([]byte, error) {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	for _, record := range records {
		if err := enc.Encode(record); err != nil {
			return nil, err
		}
	}
	return []byte(sb.String()), nil
}

func writeFileIfChanged(path string, data []byte) error {
	_, err := writeFileIfChangedTracked(path, data)
	return err
}

func writeFileIfChangedTracked(path string, data []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return false, err
	}
	return true, nil
}

func writeFileIfMissing(path string, data []byte, perm os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func runInstallHook(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}

	repoRoot, gitDir, err := resolveGitPaths(rootPath)
	if err != nil {
		return err
	}

	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hookPath), 0755); err != nil {
		return fmt.Errorf("failed to create hook directory: %w", err)
	}

	existing := ""
	if data, err := os.ReadFile(hookPath); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing hook: %w", err)
	}

	updated := upsertSkellyHook(existing, repoRoot)
	if err := os.WriteFile(hookPath, []byte(updated), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}

	fmt.Printf("Installed pre-commit hook at %s\n", hookPath)
	return nil
}

func resolveGitPaths(workingDir string) (repoRoot string, gitDir string, err error) {
	repoRootOut, err := exec.Command("git", "-C", workingDir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", fmt.Errorf("not inside a git repository")
	}

	gitDirOut, err := exec.Command("git", "-C", workingDir, "rev-parse", "--git-dir").Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve git directory: %w", err)
	}

	repoRoot = strings.TrimSpace(string(repoRootOut))
	gitDir = strings.TrimSpace(string(gitDirOut))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoRoot, gitDir)
	}
	return repoRoot, gitDir, nil
}

func upsertSkellyHook(existingHook, repoRoot string) string {
	block := buildSkellyHookBlock(repoRoot)

	if existingHook == "" {
		return "#!/bin/sh\n\n" + block + "\n"
	}

	start := strings.Index(existingHook, hookStart)
	end := strings.Index(existingHook, hookEnd)
	if start >= 0 && end >= start {
		end += len(hookEnd)
		updated := existingHook[:start] + block + existingHook[end:]
		return ensureTrailingNewline(updated)
	}

	base := ensureTrailingNewline(existingHook)
	if !strings.HasPrefix(base, "#!") {
		base = "#!/bin/sh\n" + base
	}
	return base + "\n" + block + "\n"
}

func buildSkellyHookBlock(repoRoot string) string {
	return fmt.Sprintf(
		"%s\nrepo_root=%q\ncontext_dir=\"$repo_root/%s\"\nif command -v skelly >/dev/null 2>&1; then\n  if [ -f \"$context_dir/manifest.json\" ] && [ -f \"$context_dir/symbols.jsonl\" ] && [ -f \"$context_dir/edges.jsonl\" ]; then\n    (cd \"$repo_root\" && skelly update --format jsonl) || exit 1\n  else\n    (cd \"$repo_root\" && skelly update) || exit 1\n  fi\nfi\n%s",
		hookStart,
		repoRoot,
		output.ContextDir,
		hookEnd,
	)
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
