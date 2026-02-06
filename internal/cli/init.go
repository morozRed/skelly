package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/languages"
	"github.com/morozRed/skelly/internal/llm"
	"github.com/morozRed/skelly/internal/nav"
	"github.com/morozRed/skelly/internal/output"
	"github.com/morozRed/skelly/internal/state"
	"github.com/spf13/cobra"
)

func RunInit(cmd *cobra.Command, args []string) error {
	rootPath, err := resolveWorkingDirectory()
	if err != nil {
		return err
	}

	writer := output.NewWriter(rootPath)
	if err := writer.Init(); err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	statePath := filepath.Join(contextDir, state.StateFile)
	if _, err := os.Stat(statePath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect state file: %w", err)
		}
		if err := state.NewState().Save(contextDir); err != nil {
			return fmt.Errorf("failed to write initial state: %w", err)
		}
	}

	llmRaw, err := OptionalStringFlag(cmd, "llm")
	if err != nil {
		return err
	}
	llmProviders, err := llm.ParseLLMProviders(llmRaw)
	if err != nil {
		return err
	}
	if len(llmProviders) > 0 {
		updatedFiles, err := llm.GenerateIntegrationFiles(rootPath, llmProviders)
		if err != nil {
			return err
		}
		if len(updatedFiles) > 0 {
			fmt.Printf("Updated LLM integration files: %s\n", strings.Join(updatedFiles, ", "))
		}
	}

	fmt.Printf("Initialized context directory at %s\n", contextDir)

	noGenerate, _ := nav.OptionalBoolFlag(cmd, "no-generate", false)
	if noGenerate {
		return nil
	}

	format, err := ParseOutputFormat(cmd)
	if err != nil {
		return err
	}

	// Auto-generate if there are parseable source files.
	ignoreRules, err := LoadIgnoreRules(rootPath)
	if err != nil {
		return err
	}
	hasSources, err := hasSourceFiles(rootPath, ignoreRules)
	if err != nil {
		return fmt.Errorf("failed to scan source files: %w", err)
	}
	if hasSources {
		fmt.Println("Running initial generate...")
		if err := GenerateContext(rootPath, nil, format, false); err != nil {
			return err
		}
	}

	return nil
}

func hasSourceFiles(rootPath string, ignoreRules []string) (bool, error) {
	hashes, err := fileutil.ScanFileHashes(rootPath, languages.NewDefaultRegistry(), ignoreRules)
	if err != nil {
		return false, err
	}
	return len(hashes) > 0, nil
}
