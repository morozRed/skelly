package nav

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/output"
)

const NavigationIndexFile = "nav-index.json"

func WriteIndex(contextDir string, g *graph.Graph) error {
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		return err
	}

	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	nodes := make([]IndexNode, 0, len(ids))
	for _, id := range ids {
		node := g.Nodes[id]
		outConf := make([]EdgeConfidence, 0, len(node.OutEdges))
		for _, targetID := range node.OutEdges {
			outConf = append(outConf, EdgeConfidence{
				TargetID:   targetID,
				Confidence: node.OutEdgeConfidence[targetID],
			})
		}
		sort.Slice(outConf, func(i, j int) bool {
			return outConf[i].TargetID < outConf[j].TargetID
		})

		nodes = append(nodes, IndexNode{
			ID:            node.ID,
			Name:          node.Symbol.Name,
			Kind:          node.Symbol.Kind.String(),
			Signature:     node.Symbol.Signature,
			File:          node.File,
			Line:          node.Symbol.Line,
			OutEdges:      append([]string(nil), node.OutEdges...),
			InEdges:       append([]string(nil), node.InEdges...),
			OutConfidence: outConf,
		})
	}

	index := Index{
		Version: "nav-index-v1",
		Nodes:   nodes,
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteIfChanged(filepath.Join(contextDir, NavigationIndexFile), data)
}

func LoadLookup(rootPath string) (*Lookup, error) {
	path := filepath.Join(rootPath, output.ContextDir, NavigationIndexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("navigation index missing at %s (run skelly update)", path)
		}
		return nil, fmt.Errorf("failed to read navigation index: %w", err)
	}

	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to decode navigation index: %w", err)
	}

	lookup := &Lookup{
		ByID:   make(map[string]*IndexNode, len(index.Nodes)),
		ByName: make(map[string][]string),
	}
	for i := range index.Nodes {
		node := &index.Nodes[i]
		lookup.ByID[node.ID] = node
		lookup.ByName[node.Name] = append(lookup.ByName[node.Name], node.ID)
	}
	for name := range lookup.ByName {
		sort.Strings(lookup.ByName[name])
	}
	return lookup, nil
}

func Resolve(l *Lookup, query string) []*IndexNode {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if node, ok := l.ByID[query]; ok {
		return []*IndexNode{node}
	}
	ids := l.ByName[query]
	out := make([]*IndexNode, 0, len(ids))
	for _, id := range ids {
		if node := l.ByID[id]; node != nil {
			out = append(out, node)
		}
	}
	return out
}

func ResolveSingleSymbol(l *Lookup, query string) (*IndexNode, error) {
	matches := Resolve(l, query)
	if len(matches) == 0 {
		return nil, fmt.Errorf("symbol %q not found", query)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	options := make([]string, 0, len(matches))
	for _, match := range matches {
		options = append(options, match.ID)
	}
	sort.Strings(options)
	return nil, fmt.Errorf("symbol %q is ambiguous; use one of: %s", query, strings.Join(options, ", "))
}

func SymbolRecordFromNode(node *IndexNode) SymbolRecord {
	if node == nil {
		return SymbolRecord{}
	}
	return SymbolRecord{
		ID:        node.ID,
		Name:      node.Name,
		Kind:      node.Kind,
		Signature: node.Signature,
		File:      node.File,
		Line:      node.Line,
	}
}

func (l *Lookup) EdgeConfidenceValue(fromID, toID string) string {
	from := l.ByID[fromID]
	if from == nil {
		return ""
	}
	for _, item := range from.OutConfidence {
		if item.TargetID == toID {
			return item.Confidence
		}
	}
	return ""
}
