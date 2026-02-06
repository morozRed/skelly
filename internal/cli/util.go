package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveWorkingDirectory() (string, error) {
	rootPath, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory: %w", err)
	}
	return rootPath, nil
}

func LoadIgnoreRules(rootPath string) ([]string, error) {
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
