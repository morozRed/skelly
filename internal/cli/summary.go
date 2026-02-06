package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
	Suggestions  []string        `json:"suggestions,omitempty"`
	Integrations map[string]bool `json:"integrations,omitempty"`
}

func PrintRunSummary(summary RunSummary, asJSON bool) error {
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
			fmt.Printf("changed files (%d): %s\n", len(summary.ChangedFiles), SummarizePaths(summary.ChangedFiles, 8))
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
		fmt.Printf("changed files (%d): %s\n", len(summary.ChangedFiles), SummarizePaths(summary.ChangedFiles, 8))
	}
	if len(summary.DeletedFiles) > 0 {
		fmt.Printf("deleted files (%d): %s\n", len(summary.DeletedFiles), SummarizePaths(summary.DeletedFiles, 8))
	}
	if len(summary.ImpactedFiles) > 0 && summary.Mode != "generate" {
		fmt.Printf("impacted files (%d): %s\n", len(summary.ImpactedFiles), SummarizePaths(summary.ImpactedFiles, 8))
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

func PrintEnrichSummary(summary EnrichRunSummary, asJSON bool) error {
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
		fmt.Printf("targets (%d): %s\n", len(summary.Targets), SummarizePaths(summary.Targets, 8))
	}
	return nil
}

func SummarizePaths(paths []string, max int) string {
	if len(paths) <= max {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s ... (+%d more)", strings.Join(paths[:max], ", "), len(paths)-max)
}
