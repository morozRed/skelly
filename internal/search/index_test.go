package search

import (
	"testing"

	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/parser"
)

func TestSearchRanksSymbolNameMatches(t *testing.T) {
	parseResult := &parser.ParseResult{
		RootPath: ".",
		Files: []parser.FileSymbols{
			{
				Path:     "internal/parser/parser.go",
				Language: "go",
				Symbols: []parser.Symbol{
					{ID: "id-1", Name: "ParseDirectory", Kind: parser.SymbolFunction, Signature: "func ParseDirectory(root string)"},
					{ID: "id-2", Name: "ResolveImports", Kind: parser.SymbolFunction, Signature: "func ResolveImports(path string)"},
				},
			},
		},
	}

	g := graph.BuildFromParseResult(parseResult)
	index := Build(g)
	results := Search(index, "parse directory", 5)
	if len(results) == 0 {
		t.Fatalf("expected fuzzy results for partial query")
	}
	if results[0].ID != "id-1" {
		t.Fatalf("expected ParseDirectory to rank first, got %#v", results)
	}
}

func TestSearchTypoFallback(t *testing.T) {
	index := &Index{
		Version:       Version,
		DocumentCount: 1,
		AvgDocLength:  1,
		DocFreq:       map[string]int{},
		Documents: []Document{
			{ID: "id-1", Name: "ParseDirectory", Length: 1, Terms: map[string]int{"parsedirectory": 1}},
		},
	}

	results := Search(index, "ParzeDirectory", 3)
	if len(results) == 0 {
		t.Fatalf("expected typo fallback results")
	}
	if results[0].ID != "id-1" {
		t.Fatalf("expected typo fallback to pick ParseDirectory, got %#v", results)
	}
}

func TestSearchDeterministicOrdering(t *testing.T) {
	index := &Index{
		Version:       Version,
		DocumentCount: 2,
		AvgDocLength:  1,
		DocFreq:       map[string]int{"alpha": 2},
		Documents: []Document{
			{ID: "b", Length: 1, Terms: map[string]int{"alpha": 1}},
			{ID: "a", Length: 1, Terms: map[string]int{"alpha": 1}},
		},
	}

	results := Search(index, "alpha", 2)
	if len(results) != 2 {
		t.Fatalf("expected two results, got %d", len(results))
	}
	if results[0].ID != "a" || results[1].ID != "b" {
		t.Fatalf("expected stable tie-break by id, got %#v", results)
	}
}
