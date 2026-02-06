package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/output"
)

func ParseLLMProviders(raw string) ([]string, error) {
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

func GenerateIntegrationFiles(rootPath string, providers []string) ([]string, error) {
	updated := make([]string, 0)

	skillPath := filepath.Join(rootPath, ".skelly", "skills", "skelly.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create skills directory: %w", err)
	}
	skillContent := BuildSkellySkillContent()
	wrote, err := fileutil.WriteIfChangedTracked(skillPath, []byte(skillContent))
	if err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", skillPath, err)
	}
	if wrote {
		updated = append(updated, filepath.ToSlash(filepath.Clean(filepath.Join(".skelly", "skills", "skelly.md"))))
	}

	contextPath := filepath.Join(rootPath, "CONTEXT.md")
	contextDoc, err := UpsertManagedMarkdownFile(contextPath, BuildContextBlock())
	if err != nil {
		return nil, err
	}
	if contextDoc {
		updated = append(updated, "CONTEXT.md")
	}

	for _, provider := range providers {
		switch provider {
		case "codex":
			changed, err := UpsertManagedMarkdownFile(
				filepath.Join(rootPath, "AGENTS.md"),
				BuildRootAdapterBlock("Codex"),
			)
			if err != nil {
				return nil, err
			}
			if changed {
				updated = append(updated, "AGENTS.md")
			}
		case "claude":
			changed, err := UpsertManagedMarkdownFile(
				filepath.Join(rootPath, "CLAUDE.md"),
				BuildRootAdapterBlock("Claude"),
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
			changed, err := fileutil.WriteIfChangedTracked(cursorPath, []byte(BuildCursorRuleContent()))
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

func DetectContextFormat(contextDir string) string {
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

func DetectLLMIntegrations(rootPath string) map[string]bool {
	return map[string]bool{
		"skills":  fileExists(filepath.Join(rootPath, ".skelly", "skills", "skelly.md")),
		"context": ContainsManagedBlock(filepath.Join(rootPath, "CONTEXT.md")),
		"codex":   ContainsManagedBlock(filepath.Join(rootPath, "AGENTS.md")),
		"claude":  ContainsManagedBlock(filepath.Join(rootPath, "CLAUDE.md")),
		"cursor":  fileExists(filepath.Join(rootPath, ".cursor", "rules", "skelly-context.mdc")),
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
