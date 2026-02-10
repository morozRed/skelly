package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/languages"
	"github.com/morozRed/skelly/internal/nav"
	"github.com/morozRed/skelly/internal/output"
	"github.com/morozRed/skelly/internal/search"
	"github.com/morozRed/skelly/internal/state"
	"github.com/spf13/cobra"
)

func RunSetup(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(os.Stderr, "warning: 'skelly setup' is deprecated; use 'skelly init' instead")

	rootPath, err := resolveWorkingDirectory()
	if err != nil {
		return err
	}

	format, err := ParseOutputFormat(cmd)
	if err != nil {
		return err
	}
	fmt.Printf("setup: format=%s\n", format)
	fmt.Println("setup: running generate...")
	if err := GenerateContext(rootPath, nil, format, false); err != nil {
		return err
	}
	fmt.Println(`setup: done. Agents can add descriptions with:
  skelly enrich <target> "<description>"`)
	return nil
}

func RunGenerate(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	languageFilter, err := ParseLanguageFilter(cmd)
	if err != nil {
		return err
	}
	format, err := ParseOutputFormat(cmd)
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

	return GenerateContext(rootPath, languageFilter, format, asJSON)
}

func GenerateContext(rootPath string, languageFilter map[string]bool, format output.Format, asJSON bool) error {
	start := time.Now()
	ignoreRules, err := LoadIgnoreRules(rootPath)
	if err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	previousOutputHashes, _ := LoadOutputHashesFromState(contextDir)

	registry := languages.NewDefaultRegistry()
	parseResult, err := registry.ParseDirectory(rootPath, ignoreRules)
	if err != nil {
		return fmt.Errorf("failed to parse source files: %w", err)
	}
	ReportParseIssues(parseResult.Issues)
	parseResult.Files = FilterFilesByLanguage(parseResult.Files, languageFilter)
	for i := range parseResult.Files {
		fileutil.EnsureSymbolIDs(&parseResult.Files[i])
	}

	g := graph.BuildFromParseResult(parseResult)
	writer := output.NewWriter(rootPath)
	if err := writer.WriteAll(g, parseResult, format); err != nil {
		return fmt.Errorf("failed to write output files: %w", err)
	}
	if err := nav.WriteIndex(contextDir, g); err != nil {
		return fmt.Errorf("failed to write navigation index: %w", err)
	}
	if err := search.Write(contextDir, g); err != nil {
		return fmt.Errorf("failed to write search index: %w", err)
	}

	if err := PersistState(contextDir, parseResult.Files, g, format); err != nil {
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
		Rewritten:     CountRewrittenOutputs(previousOutputHashes, updatedState.OutputHashes),
		Changed:       len(parseResult.Files),
		Deleted:       0,
		Impacted:      len(parseResult.Files),
		DurationMS:    time.Since(start).Milliseconds(),
		ChangedFiles:  CollectFilePaths(parseResult.Files),
		ImpactedFiles: CollectFilePaths(parseResult.Files),
	}

	return PrintRunSummary(summary, asJSON)
}
