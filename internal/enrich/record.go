package enrich

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/parser"
	"github.com/morozRed/skelly/internal/state"
)

func BuildRecord(
	rootPath string,
	file string,
	fileState state.FileState,
	sym parser.Symbol,
	node *graph.Node,
	lineCache map[string][]string,
	agent string,
	scope Scope,
) (Record, bool) {
	if sym.ID == "" {
		sym.ID = parser.StableSymbolID(file, sym)
	}

	sourceLine := ReadSourceLine(rootPath, file, sym.Line, lineCache)
	calls := make([]string, 0)
	calledBy := make([]string, 0)
	if node != nil {
		calls = append(calls, node.OutEdges...)
		calledBy = append(calledBy, node.InEdges...)
		sort.Strings(calls)
		sort.Strings(calledBy)
	}

	record := Record{
		SymbolID:     sym.ID,
		Agent:        agent,
		AgentProfile: agent,
		Scope:        string(scope),
		FileHash:     fileState.Hash,
		Status:       "pending",
		Input: InputPayload{
			Symbol: SymbolMetadata{
				ID:        sym.ID,
				Name:      sym.Name,
				Kind:      sym.Kind.String(),
				Signature: sym.Signature,
				Path:      file,
				Language:  fileState.Language,
				Line:      sym.Line,
			},
			Source: SourceSpan{
				StartLine: sym.Line,
				EndLine:   sym.Line,
				Body:      sourceLine,
			},
			Imports:  append([]string(nil), fileState.Imports...),
			Calls:    calls,
			CalledBy: calledBy,
		},
	}
	return record, true
}

func ReadSourceLine(rootPath, file string, line int, lineCache map[string][]string) string {
	if line <= 0 {
		return ""
	}
	lines, ok := lineCache[file]
	if !ok {
		data, err := os.ReadFile(filepath.Join(rootPath, file))
		if err != nil {
			lineCache[file] = nil
			return ""
		}
		lines = strings.Split(string(data), "\n")
		lineCache[file] = lines
	}
	if line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}
