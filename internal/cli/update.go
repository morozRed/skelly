package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

func RunUpdate(cmd *cobra.Command, args []string) error {
	start := time.Now()
	rootPath, err := resolveWorkingDirectory()
	if err != nil {
		return err
	}
	explain, err := cmd.Flags().GetBool("explain")
	if err != nil {
		return fmt.Errorf("failed to read --explain flag: %w", err)
	}
	format, err := ParseOutputFormat(cmd)
	if err != nil {
		return err
	}
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("failed to read --json flag: %w", err)
	}

	registry := languages.NewDefaultRegistry()
	ignoreRules, err := LoadIgnoreRules(rootPath)
	if err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	st, err := state.Load(contextDir)
	if err != nil {
		if IsCorruptStateError(err) {
			fmt.Fprintf(os.Stderr, "warning: corrupt state file detected (%v); running full regenerate\n", err)
			return GenerateContext(rootPath, nil, format, asJSON)
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
		return GenerateContext(rootPath, nil, format, asJSON)
	}
	if st.OutputVersion != state.CurrentOutputVersion {
		fmt.Fprintf(
			os.Stderr,
			"warning: output schema version changed (%s -> %s); running full regenerate\n",
			st.OutputVersion,
			state.CurrentOutputVersion,
		)
		return GenerateContext(rootPath, nil, format, asJSON)
	}

	currentHashes, err := fileutil.ScanFileHashes(rootPath, registry, ignoreRules)
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

	changed = fileutil.DedupeStrings(changed)
	sort.Strings(changed)
	sort.Strings(deleted)

	if len(changed) == 0 && len(deleted) == 0 {
		rewritten := 0
		if OutputsNeedRefresh(st, contextDir, format) {
			parseResult := fileutil.ParseResultFromState(st, rootPath, currentHashes)
			g := graph.BuildFromParseResult(parseResult)
			beforeOutputHashes := CloneOutputHashes(st.OutputHashes)

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
			if err := RecordOutputHashes(st, contextDir, format); err != nil {
				return fmt.Errorf("failed to update output hashes: %w", err)
			}
			if err := st.Save(contextDir); err != nil {
				return fmt.Errorf("failed to persist state: %w", err)
			}
			rewritten = CountRewrittenOutputs(beforeOutputHashes, st.OutputHashes)
		}

		return PrintRunSummary(RunSummary{
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

	progress := newParseProgressReporter("update", len(changed), asJSON)
	parsedCount := 0
	for _, file := range changed {
		parsedCount++
		progress.Update(file, parsedCount)
		absPath := filepath.Join(rootPath, file)
		parsed, err := registry.ParseFile(absPath)
		if err != nil {
			progress.Done(parsedCount)
			return fmt.Errorf("failed to parse %s: %w", file, err)
		}
		if parsed == nil {
			// No longer supported or ignored by parser rules.
			st.RemoveFile(file)
			continue
		}

		parsed.Path = file
		parsed.Hash = currentHashes[file]
		fileutil.EnsureSymbolIDs(parsed)
		st.SetFileData(*parsed)
	}
	progress.Done(parsedCount)

	for _, file := range deleted {
		st.RemoveFile(file)
	}

	impacted, reasons := fileutil.ImpactedWithReasons(st, changed, deleted)
	sort.Strings(impacted)

	parseResult := fileutil.ParseResultFromState(st, rootPath, currentHashes)
	impactedExisting := fileutil.ExistingFiles(impacted, currentHashes)
	impactedSet := fileutil.ToSet(impactedExisting)

	// Recompute dependency graph metadata only for impacted sources and preserve unchanged state.
	impactedGraph := graph.BuildFromParseResultForSources(parseResult, impactedSet)
	fileutil.ApplyGraphDependencies(st, impactedGraph, impactedSet)

	// Build full graph for final outputs from the merged state snapshots.
	g := graph.BuildFromParseResult(parseResult)
	beforeOutputHashes := CloneOutputHashes(st.OutputHashes)

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
	if err := RecordOutputHashes(st, contextDir, format); err != nil {
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
		Reused:        MaxInt(len(currentHashes)-len(changed), 0),
		Rewritten:     CountRewrittenOutputs(beforeOutputHashes, st.OutputHashes),
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
	return PrintRunSummary(summary, asJSON)
}
