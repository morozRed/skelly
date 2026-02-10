package lsp

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var outputLocationPattern = regexp.MustCompile(`^(.*):([0-9]+):([0-9]+)(?:[-:].*)?$`)

type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

type CommandRunner func(dir string, name string, args ...string) (string, error)

func QueryDefinition(rootPath string, file string, line int, column int, server string) ([]Location, error) {
	return QueryDefinitionWithRunner(rootPath, file, line, column, server, defaultRunner)
}

func QueryReferences(rootPath string, file string, line int, column int, server string) ([]Location, error) {
	return QueryReferencesWithRunner(rootPath, file, line, column, server, defaultRunner)
}

func QueryDefinitionWithRunner(rootPath string, file string, line int, column int, server string, runner CommandRunner) ([]Location, error) {
	return runQueryWithRunner(rootPath, file, line, column, server, "definition", runner)
}

func QueryReferencesWithRunner(rootPath string, file string, line int, column int, server string, runner CommandRunner) ([]Location, error) {
	return runQueryWithRunner(rootPath, file, line, column, server, "references", runner)
}

func runQueryWithRunner(rootPath string, file string, line int, column int, server string, command string, runner CommandRunner) ([]Location, error) {
	if runner == nil {
		return nil, errors.New("command runner is required")
	}
	if strings.TrimSpace(server) == "" {
		return nil, errors.New("lsp server is required")
	}
	if line <= 0 {
		return nil, errors.New("line must be > 0")
	}
	if column <= 0 {
		column = 1
	}

	absFile := file
	if !filepath.IsAbs(absFile) {
		absFile = filepath.Join(rootPath, file)
	}
	position := fmt.Sprintf("%s:%d:%d", absFile, line, column)

	switch server {
	case "gopls":
		output, err := runner(rootPath, server, command, position)
		if err != nil {
			return nil, fmt.Errorf("lsp %s query failed: %w", command, err)
		}
		locations := ParseLocationOutput(rootPath, output)
		return DeduplicateLocations(locations), nil
	default:
		return nil, fmt.Errorf("lsp server %q does not support %s query backend", server, command)
	}
}

func ParseLocationOutput(rootPath string, output string) []Location {
	lines := strings.Split(output, "\n")
	locations := make([]Location, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := outputLocationPattern.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		parsedLine, err := strconv.Atoi(match[2])
		if err != nil || parsedLine <= 0 {
			continue
		}
		parsedCol, err := strconv.Atoi(match[3])
		if err != nil || parsedCol <= 0 {
			parsedCol = 1
		}
		locations = append(locations, Location{
			File:   normalizeLocationPath(rootPath, match[1]),
			Line:   parsedLine,
			Column: parsedCol,
		})
	}
	return locations
}

func DeduplicateLocations(locations []Location) []Location {
	if len(locations) <= 1 {
		return locations
	}
	seen := make(map[string]bool, len(locations))
	out := make([]Location, 0, len(locations))
	for _, location := range locations {
		key := fmt.Sprintf("%s:%d:%d", location.File, location.Line, location.Column)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, location)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Column < out[j].Column
	})
	return out
}

func normalizeLocationPath(rootPath string, locationPath string) string {
	locationPath = strings.TrimSpace(locationPath)
	if locationPath == "" {
		return locationPath
	}
	if filepath.IsAbs(locationPath) {
		rel, err := filepath.Rel(rootPath, locationPath)
		if err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(locationPath)
}

func defaultRunner(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
