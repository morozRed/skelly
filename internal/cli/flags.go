package cli

import (
	"fmt"
	"strings"

	"github.com/morozRed/skelly/internal/output"
	"github.com/spf13/cobra"
)

func OptionalStringFlag(cmd *cobra.Command, name string) (string, error) {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return "", nil
	}
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", fmt.Errorf("failed to read --%s flag: %w", name, err)
	}
	return strings.TrimSpace(value), nil
}

func ParseLanguageFilter(cmd *cobra.Command) (map[string]bool, error) {
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

func ParseOutputFormat(cmd *cobra.Command) (output.Format, error) {
	value, err := cmd.Flags().GetString("format")
	if err != nil {
		return "", fmt.Errorf("failed to read --format flag: %w", err)
	}
	return output.ParseFormat(value)
}
