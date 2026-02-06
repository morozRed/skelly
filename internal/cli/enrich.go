package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/morozRed/skelly/internal/enrich"
	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/languages"
	"github.com/morozRed/skelly/internal/output"
	"github.com/morozRed/skelly/internal/state"
	"github.com/spf13/cobra"
)

func RunEnrich(cmd *cobra.Command, args []string) error {
	start := time.Now()
	rootPath, err := resolveWorkingDirectory()
	if err != nil {
		return err
	}
	targetSelector := strings.TrimSpace(args[0])
	description := strings.TrimSpace(strings.Join(args[1:], " "))
	if description == "" {
		return fmt.Errorf("description is required")
	}
	outputPayload := enrich.Output{
		Summary:     description,
		Purpose:     description,
		SideEffects: "Unknown from static analysis.",
		Confidence:  "medium",
	}
	if err := enrich.ValidateOutput(outputPayload); err != nil {
		return fmt.Errorf("invalid enrich output: %w", err)
	}

	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("failed to read --json flag: %w", err)
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	st, err := state.Load(contextDir)
	if err != nil {
		if IsCorruptStateError(err) {
			return fmt.Errorf("state is corrupt; run `skelly generate` first")
		}
		return fmt.Errorf("failed to load state: %w", err)
	}
	if len(st.Files) == 0 {
		return fmt.Errorf("no indexed files found; run `skelly generate` first")
	}

	registry := languages.NewDefaultRegistry()
	ignoreRules, err := LoadIgnoreRules(rootPath)
	if err != nil {
		return err
	}
	currentHashes, err := fileutil.ScanFileHashes(rootPath, registry, ignoreRules)
	if err != nil {
		return fmt.Errorf("failed to scan files: %w", err)
	}
	targetFiles := make([]string, 0, len(currentHashes))
	for file := range currentHashes {
		targetFiles = append(targetFiles, file)
	}
	sort.Strings(targetFiles)
	if len(targetFiles) == 0 {
		return PrintEnrichSummary(EnrichRunSummary{
			Mode:       "enrich",
			Agent:      "agent",
			Scope:      string(enrich.ScopeTarget),
			Target:     targetSelector,
			RootPath:   rootPath,
			OutputFile: filepath.Join(contextDir, enrich.OutputFile),
			Files:      0,
			Symbols:    0,
			DurationMS: time.Since(start).Milliseconds(),
		}, asJSON)
	}

	parseResult := fileutil.ParseResultFromState(st, rootPath, currentHashes)
	g := graph.BuildFromParseResult(parseResult)
	cachePath := filepath.Join(contextDir, enrich.OutputFile)
	cacheRecords, err := enrich.LoadCache(cachePath)
	if err != nil {
		return err
	}

	workItems := enrich.CollectWorkItems(targetFiles, st, g)
	workItems = enrich.FilterWorkItems(workItems, targetSelector)
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
			enrich.SummarizeMatches(workItems, 5),
		)
	}

	item := workItems[0]
	lineCache := make(map[string][]string)
	record, ok := enrich.BuildRecord(
		rootPath,
		item.File,
		item.FileState,
		item.Symbol,
		item.Node,
		lineCache,
		"agent",
		enrich.ScopeTarget,
	)
	if !ok {
		return fmt.Errorf("target %q could not be enriched", targetSelector)
	}
	record.AgentProfile = "agent"
	record.Model = "manual"
	record.PromptVersion = "agent-note-v1"
	record.CacheKey = enrich.CacheKey(record.SymbolID, record.FileHash, record.PromptVersion, record.AgentProfile, record.Model)
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
	enrich.PruneCacheForSymbol(cacheRecords, record.CacheKey, record.SymbolID, record.AgentProfile)
	if err := enrich.WriteCache(cachePath, cacheRecords); err != nil {
		return err
	}

	return PrintEnrichSummary(EnrichRunSummary{
		Mode:        "enrich",
		Agent:       "agent",
		Scope:       string(enrich.ScopeTarget),
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
