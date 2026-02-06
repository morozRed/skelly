package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/nav"
	"github.com/morozRed/skelly/internal/output"
	"github.com/morozRed/skelly/internal/parser"
	"github.com/morozRed/skelly/internal/state"
)

func RecordOutputHashes(st *state.State, contextDir string, format output.Format) error {
	st.OutputHashes = make(map[string]string)

	var outputPaths []string
	switch format {
	case output.FormatText:
		outputPaths = []string{
			filepath.Join(contextDir, output.IndexFile),
			filepath.Join(contextDir, output.GraphFile),
		}

		moduleFiles, err := filepath.Glob(filepath.Join(contextDir, output.ModulesDir, "*.txt"))
		if err != nil {
			return err
		}
		outputPaths = append(outputPaths, moduleFiles...)
	case output.FormatJSONL:
		outputPaths = []string{
			filepath.Join(contextDir, output.SymbolsFile),
			filepath.Join(contextDir, output.EdgesFile),
			filepath.Join(contextDir, output.ManifestFile),
		}
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
	outputPaths = append(outputPaths, filepath.Join(contextDir, nav.NavigationIndexFile))

	for _, outputPath := range outputPaths {
		hash, err := fileutil.HashFile(outputPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		relPath, err := filepath.Rel(contextDir, outputPath)
		if err != nil {
			return err
		}
		st.SetOutputHash(relPath, hash)
	}

	return nil
}

func IsCorruptStateError(err error) bool {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr)
}

func LoadOutputHashesFromState(contextDir string) (map[string]string, error) {
	st, err := state.Load(contextDir)
	if err != nil {
		return nil, err
	}
	return CloneOutputHashes(st.OutputHashes), nil
}

func CloneOutputHashes(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func CountRewrittenOutputs(before, after map[string]string) int {
	rewritten := 0
	seen := make(map[string]bool, len(before)+len(after))
	for file := range before {
		seen[file] = true
	}
	for file := range after {
		seen[file] = true
	}
	for file := range seen {
		if before[file] != after[file] {
			rewritten++
		}
	}
	return rewritten
}

func CollectFilePaths(files []parser.FileSymbols) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return paths
}

func OutputsNeedRefresh(st *state.State, contextDir string, format output.Format) bool {
	for _, file := range RequiredOutputFiles(format) {
		if _, ok := st.OutputHashes[file]; !ok {
			return true
		}
		if _, err := os.Stat(filepath.Join(contextDir, file)); err != nil {
			return true
		}
	}
	return false
}

func RequiredOutputFiles(format output.Format) []string {
	switch format {
	case output.FormatText:
		return []string{output.IndexFile, output.GraphFile, nav.NavigationIndexFile}
	case output.FormatJSONL:
		return []string{output.SymbolsFile, output.EdgesFile, output.ManifestFile, nav.NavigationIndexFile}
	default:
		return nil
	}
}

func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func FilterFilesByLanguage(files []parser.FileSymbols, languageFilter map[string]bool) []parser.FileSymbols {
	if len(languageFilter) == 0 {
		return files
	}

	filtered := make([]parser.FileSymbols, 0, len(files))
	for _, file := range files {
		if languageFilter[file.Language] {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func ReportParseIssues(issues []parser.ParseIssue) {
	for _, issue := range issues {
		if issue.Language != "" {
			fmt.Fprintf(os.Stderr, "[%s] %s (%s): %s\n", issue.Severity, issue.File, issue.Language, issue.Message)
			continue
		}
		fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", issue.Severity, issue.File, issue.Message)
	}
}

func PersistState(contextDir string, files []parser.FileSymbols, g *graph.Graph, format output.Format) error {
	st := state.NewState()
	for _, file := range files {
		st.SetFileData(file)
	}
	fileutil.ApplyGraphDependencies(st, g, nil)
	if err := RecordOutputHashes(st, contextDir, format); err != nil {
		return err
	}
	return st.Save(contextDir)
}
