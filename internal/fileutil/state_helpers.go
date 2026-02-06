package fileutil

import (
	"sort"

	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/parser"
	"github.com/morozRed/skelly/internal/state"
)

func ParseResultFromState(st *state.State, rootPath string, currentHashes map[string]string) *parser.ParseResult {
	files := make([]parser.FileSymbols, 0, len(currentHashes))

	for path, hash := range currentHashes {
		fileState, ok := st.Files[path]
		if !ok {
			continue
		}

		files = append(files, parser.FileSymbols{
			Path:          path,
			Language:      fileState.Language,
			Symbols:       fileState.Symbols,
			Imports:       fileState.Imports,
			ImportAliases: fileState.ImportAliases,
			Hash:          hash,
		})
		EnsureSymbolIDs(&files[len(files)-1])
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return &parser.ParseResult{
		Files:    files,
		RootPath: rootPath,
	}
}

func EnsureSymbolIDs(file *parser.FileSymbols) {
	for i := range file.Symbols {
		if file.Symbols[i].ID != "" {
			continue
		}
		file.Symbols[i].ID = parser.StableSymbolID(file.Path, file.Symbols[i])
	}
}

func ApplyGraphDependencies(st *state.State, g *graph.Graph, files map[string]bool) {
	targetFiles := make([]string, 0)
	if files != nil {
		for file := range files {
			targetFiles = append(targetFiles, file)
		}
	} else {
		for file := range st.Files {
			targetFiles = append(targetFiles, file)
		}
	}
	sort.Strings(targetFiles)

	for _, file := range targetFiles {
		fileState, ok := st.Files[file]
		if !ok {
			continue
		}

		nodeIDs := g.FileNodes[file]
		deps := make(map[string]bool)
		for _, nodeID := range nodeIDs {
			node, ok := g.Nodes[nodeID]
			if !ok {
				continue
			}

			for _, edge := range node.OutEdges {
				targetFile, _ := graph.ParseNodeID(edge)
				if targetFile == "" || targetFile == file {
					continue
				}
				deps[targetFile] = true
			}
		}

		fileState.Dependencies = MapKeysSorted(deps)
		st.Files[file] = fileState
	}
}

func ImpactedWithReasons(st *state.State, changed, deleted []string) ([]string, map[string][]string) {
	reverseDeps := make(map[string][]string)
	for file, fileState := range st.Files {
		for _, dep := range fileState.Dependencies {
			reverseDeps[dep] = append(reverseDeps[dep], file)
		}
	}
	for file := range reverseDeps {
		sort.Strings(reverseDeps[file])
	}

	reasons := make(map[string][]string)
	seen := make(map[string]bool)
	queue := make([]string, 0, len(changed)+len(deleted))

	for _, file := range changed {
		queue = append(queue, file)
		seen[file] = true
		reasons[file] = appendReason(reasons[file], "changed")
	}
	for _, file := range deleted {
		if !seen[file] {
			queue = append(queue, file)
		}
		seen[file] = true
		reasons[file] = appendReason(reasons[file], "deleted")
	}

	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]

		for _, dependent := range reverseDeps[file] {
			reason := "depends on " + file
			reasons[dependent] = appendReason(reasons[dependent], reason)
			if seen[dependent] {
				continue
			}
			seen[dependent] = true
			queue = append(queue, dependent)
		}
	}

	changedNames := make(map[string]bool)
	for _, file := range changed {
		fileState, ok := st.Files[file]
		if !ok {
			continue
		}
		for _, sym := range fileState.Symbols {
			if sym.Name != "" {
				changedNames[sym.Name] = true
			}
		}
	}

	for file, fileState := range st.Files {
		if seen[file] || len(changedNames) == 0 {
			continue
		}

		matchedName := ""
		for _, sym := range fileState.Symbols {
			for _, call := range sym.Calls {
				if changedNames[call.Name] {
					matchedName = call.Name
					break
				}
			}
			if matchedName != "" {
				break
			}
		}
		if matchedName == "" {
			continue
		}

		seen[file] = true
		reasons[file] = appendReason(reasons[file], "calls changed symbol "+matchedName)
	}

	impacted := make([]string, 0, len(seen))
	for file := range seen {
		impacted = append(impacted, file)
		sort.Strings(reasons[file])
	}
	sort.Strings(impacted)
	return impacted, reasons
}

func appendReason(existing []string, reason string) []string {
	for _, item := range existing {
		if item == reason {
			return existing
		}
	}
	return append(existing, reason)
}
