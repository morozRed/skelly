package enrich

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/parser"
	"github.com/morozRed/skelly/internal/state"
)

func CloneSymbolsSorted(file string, symbols []parser.Symbol) []parser.Symbol {
	out := append([]parser.Symbol(nil), symbols...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line == out[j].Line {
			leftID := out[i].ID
			if leftID == "" {
				leftID = parser.StableSymbolID(file, out[i])
			}
			rightID := out[j].ID
			if rightID == "" {
				rightID = parser.StableSymbolID(file, out[j])
			}
			return leftID < rightID
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func CollectWorkItems(targetFiles []string, st *state.State, g *graph.Graph) []WorkItem {
	items := make([]WorkItem, 0)
	for _, file := range targetFiles {
		fileState, ok := st.Files[file]
		if !ok {
			continue
		}
		symbols := CloneSymbolsSorted(file, fileState.Symbols)
		for _, sym := range symbols {
			node := g.Nodes[sym.ID]
			items = append(items, WorkItem{
				File:      file,
				FileState: fileState,
				Symbol:    sym,
				Node:      node,
			})
		}
	}
	return items
}

func FilterWorkItems(items []WorkItem, selector string) []WorkItem {
	normalizedSelector := NormalizeSelector(selector)
	if normalizedSelector == "" {
		return items
	}

	filtered := make([]WorkItem, 0, len(items))
	for _, item := range items {
		if MatchesSelector(item, normalizedSelector) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func MatchesSelector(item WorkItem, selector string) bool {
	file := NormalizeSelector(item.File)
	symbolID := NormalizeSelector(item.Symbol.ID)
	name := strings.ToLower(strings.TrimSpace(item.Symbol.Name))
	line := strconv.Itoa(item.Symbol.Line)

	candidates := []string{
		file,
		symbolID,
		name,
		file + ":" + name,
		file + ":" + line,
		file + ":" + line + ":" + name,
	}
	for _, candidate := range candidates {
		if selector == candidate {
			return true
		}
	}

	if strings.HasPrefix(file, selector) || strings.HasSuffix(file, selector) {
		return true
	}
	return strings.Contains(name, selector)
}

func NormalizeSelector(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	normalized = filepath.ToSlash(normalized)
	normalized = strings.TrimPrefix(normalized, "./")
	return strings.ToLower(normalized)
}

func SummarizeMatches(items []WorkItem, limit int) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = len(items)
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprintf("%s:%d:%s", item.File, item.Symbol.Line, item.Symbol.Name))
	}
	sort.Strings(names)
	if len(names) <= limit {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:limit], ", ") + fmt.Sprintf(", ... (+%d more)", len(names)-limit)
}
