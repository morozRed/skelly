package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/languages"
	"github.com/morozRed/skelly/internal/llm"
	"github.com/morozRed/skelly/internal/lsp"
	"github.com/morozRed/skelly/internal/nav"
	"github.com/morozRed/skelly/internal/output"
	"github.com/morozRed/skelly/internal/state"
	"github.com/spf13/cobra"
)

var suspiciousContextPrefixes = []string{
	"benchmark/agent_ab/results/",
	"benchmark/agent_ab/workspaces/",
	".bench/",
}

func RunDoctor(cmd *cobra.Command, args []string) error {
	rootPath, err := resolveWorkingDirectory()
	if err != nil {
		return err
	}
	asJSON, err := nav.OptionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	summary := DoctorSummary{
		Mode:         "doctor",
		RootPath:     rootPath,
		ContextDir:   contextDir,
		Format:       llm.DetectContextFormat(contextDir),
		Integrations: llm.DetectLLMIntegrations(rootPath),
		Clean:        false,
	}

	statePath := filepath.Join(contextDir, state.StateFile)
	_, statErr := os.Stat(statePath)
	hasState := statErr == nil
	if !hasState {
		summary.Missing = append(summary.Missing, state.StateFile)
	}
	if summary.Format == "none" {
		summary.Missing = append(summary.Missing, "context artifacts")
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
	currentPaths := make([]string, 0, len(currentHashes))
	currentFiles := make(map[string]bool, len(currentHashes))
	for file := range currentHashes {
		currentPaths = append(currentPaths, file)
		currentFiles[file] = true
	}
	sort.Strings(currentPaths)
	summary.LSP = lsp.ProbeCapabilities(lsp.DetectLanguagePresence(currentPaths))

	if hasState {
		st, err := state.Load(contextDir)
		if err != nil {
			summary.Missing = append(summary.Missing, "valid state file")
			summary.Suggestions = append(summary.Suggestions, "run skelly generate")
		} else {
			summary.IndexedFiles = len(st.Files)
			for file := range st.Files {
				for _, prefix := range suspiciousContextPrefixes {
					if strings.HasPrefix(file, prefix) {
						summary.SuspiciousIndexed++
						if len(summary.SuspiciousIndexedList) < 8 {
							summary.SuspiciousIndexedList = append(summary.SuspiciousIndexedList, file)
						}
						break
					}
				}
			}

			changed := st.ChangedFiles(currentHashes)
			deleted := st.DeletedFiles(currentFiles)
			for file, fileState := range st.Files {
				if currentFiles[file] && fileState.Language == "" && len(fileState.Symbols) == 0 {
					changed = append(changed, file)
				}
			}

			summary.Changed = len(fileutil.DedupeStrings(changed))
			summary.Deleted = len(deleted)
			summary.Clean = summary.Changed == 0 && summary.Deleted == 0

			if summary.SuspiciousIndexed > 0 {
				summary.Missing = append(summary.Missing, "context scope includes generated workspace artifacts")
				summary.Suggestions = append(summary.Suggestions, "add benchmark/agent_ab/results/ to .skellyignore")
				summary.Suggestions = append(summary.Suggestions, "run skelly update")
			}
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
		summary.Suggestions = append(summary.Suggestions, "run skelly init")
	}
	if hasState && !summary.Clean {
		summary.Suggestions = append(summary.Suggestions, "run skelly update")
	}
	if !summary.Integrations["skills"] || !summary.Integrations["context"] || !hasProviderAdapter {
		summary.Suggestions = append(summary.Suggestions, "run skelly init --llm codex,claude,cursor")
	}

	summary.Missing = fileutil.DedupeStrings(summary.Missing)
	sort.Strings(summary.Missing)
	summary.Suggestions = fileutil.DedupeStrings(summary.Suggestions)
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
	if summary.IndexedFiles > 0 {
		fmt.Printf("context size: indexed_files=%d\n", summary.IndexedFiles)
	}
	fmt.Printf("integrations: skills=%t context=%t codex=%t claude=%t cursor=%t\n",
		summary.Integrations["skills"],
		summary.Integrations["context"],
		summary.Integrations["codex"],
		summary.Integrations["claude"],
		summary.Integrations["cursor"],
	)
	availableLSP := 0
	presentLSP := 0
	for _, capability := range summary.LSP {
		if capability.Present {
			presentLSP++
			if capability.Available {
				availableLSP++
			}
		}
	}
	fmt.Printf("lsp: available=%d/%d present languages\n", availableLSP, presentLSP)
	if summary.SuspiciousIndexed > 0 {
		fmt.Printf("context scope warning: suspicious_indexed_files=%d (%s)\n",
			summary.SuspiciousIndexed,
			SummarizePaths(summary.SuspiciousIndexedList, 5),
		)
	}
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
