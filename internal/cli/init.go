package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	if err := state.NewState().Save(contextDir); err != nil {
		return fmt.Errorf("failed to write initial state: %w", err)
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
		// If --format flag is not set (e.g. tests calling with bare Command), default to text.
		format = output.FormatText
	}

	// Auto-generate if there are parseable source files.
	if hasSourceFiles(rootPath) {
		fmt.Println("Running initial generate...")
		if err := GenerateContext(rootPath, nil, format, false); err != nil {
			return err
		}
	}

	return nil
}

func hasSourceFiles(rootPath string) bool {
	extensions := []string{".go", ".py", ".rb", ".ts", ".js"}
	found := false
	_ = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == ".skelly" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		for _, ext := range extensions {
			if strings.HasSuffix(path, ext) {
				found = true
				return filepath.SkipDir
			}
		}
		return nil
	})
	return found
}
