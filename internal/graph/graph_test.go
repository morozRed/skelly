package graph

import (
	"testing"

	"github.com/skelly-dev/skelly/internal/parser"
)

func TestBuildGraphUsesStableIDsAndScopedResolution(t *testing.T) {
	result := &parser.ParseResult{
		Files: []parser.FileSymbols{
			{
				Path: "a.go",
				Symbols: []parser.Symbol{
					{Name: "helper", Kind: parser.SymbolFunction, Line: 1},
					{
						Name: "run",
						Kind: parser.SymbolFunction,
						Line: 10,
						Calls: []parser.CallSite{
							{Name: "helper"},
							{Name: "onlyB"},
							{Name: "dup"},
						},
					},
				},
			},
			{
				Path: "b.go",
				Symbols: []parser.Symbol{
					{Name: "helper", Kind: parser.SymbolFunction, Line: 2},
					{Name: "onlyB", Kind: parser.SymbolFunction, Line: 3},
					{Name: "dup", Kind: parser.SymbolFunction, Line: 4},
				},
			},
			{
				Path: "c.go",
				Symbols: []parser.Symbol{
					{Name: "dup", Kind: parser.SymbolFunction, Line: 5},
				},
			},
		},
	}

	g := BuildFromParseResult(result)
	if len(g.Nodes) != 6 {
		t.Fatalf("expected 6 graph nodes, got %d", len(g.Nodes))
	}

	seenIDs := make(map[string]bool)
	for id := range g.Nodes {
		if seenIDs[id] {
			t.Fatalf("duplicate node id: %s", id)
		}
		seenIDs[id] = true
	}

	runNode := findNodeByName(t, g, "a.go", "run")
	helperNode := findNodeByName(t, g, "a.go", "helper")
	onlyBNode := findNodeByName(t, g, "b.go", "onlyB")

	if runNode.OutEdgeConfidence[helperNode.ID] != "resolved" {
		t.Fatalf("expected helper call confidence resolved, got %q", runNode.OutEdgeConfidence[helperNode.ID])
	}
	if runNode.OutEdgeConfidence[onlyBNode.ID] != "heuristic" {
		t.Fatalf("expected onlyB call confidence heuristic, got %q", runNode.OutEdgeConfidence[onlyBNode.ID])
	}

	dupEdges := 0
	for _, edgeID := range runNode.OutEdges {
		_, name := ParseNodeID(edgeID)
		if name == "dup" {
			dupEdges++
		}
	}
	if dupEdges != 0 {
		t.Fatalf("expected ambiguous dup calls to remain unresolved, got %d edges", dupEdges)
	}
}

func TestBuildGraphPrefersImportAliasMatches(t *testing.T) {
	result := &parser.ParseResult{
		Files: []parser.FileSymbols{
			{
				Path:          "api/main.ts",
				Imports:       []string{"./util"},
				ImportAliases: map[string]string{"util": "./util"},
				Symbols: []parser.Symbol{
					{
						Name: "run",
						Kind: parser.SymbolFunction,
						Line: 5,
						Calls: []parser.CallSite{
							{Name: "helper", Qualifier: "util"},
						},
					},
				},
			},
			{
				Path: "api/util.ts",
				Symbols: []parser.Symbol{
					{Name: "helper", Kind: parser.SymbolFunction, Line: 1},
				},
			},
			{
				Path: "other/util.ts",
				Symbols: []parser.Symbol{
					{Name: "helper", Kind: parser.SymbolFunction, Line: 1},
				},
			},
		},
	}

	g := BuildFromParseResult(result)
	runNode := findNodeByName(t, g, "api/main.ts", "run")
	utilNode := findNodeByName(t, g, "api/util.ts", "helper")
	otherNode := findNodeByName(t, g, "other/util.ts", "helper")

	if runNode.OutEdgeConfidence[utilNode.ID] != "heuristic" {
		t.Fatalf("expected import-alias edge confidence heuristic, got %q", runNode.OutEdgeConfidence[utilNode.ID])
	}
	for _, edgeID := range runNode.OutEdges {
		if edgeID == otherNode.ID {
			t.Fatalf("did not expect call to resolve via global fallback when import alias is present")
		}
	}
}

func findNodeByName(t *testing.T, g *Graph, file, name string) *Node {
	t.Helper()
	for _, node := range g.NodesForFile(file) {
		if node.Symbol.Name == name {
			return node
		}
	}
	t.Fatalf("node %s in %s not found", name, file)
	return nil
}
