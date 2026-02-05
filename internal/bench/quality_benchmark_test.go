package bench

import (
	"strings"
	"testing"

	"github.com/skelly-dev/skelly/internal/graph"
	"github.com/skelly-dev/skelly/internal/parser"
)

func BenchmarkResolverQuality_Curated(b *testing.B) {
	result, expectedEdges := curatedResolverFixture()
	var precision float64
	var recall float64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g := graph.BuildFromParseResult(result)
		precision, recall = edgeMetrics(g, expectedEdges)
	}
	b.StopTimer()

	b.ReportMetric(precision, "precision")
	b.ReportMetric(recall, "recall")
}

func BenchmarkNavigationUsability_CommonQueries(b *testing.B) {
	result, _ := curatedResolverFixture()
	g := graph.BuildFromParseResult(result)

	tokenCount := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenCount = estimateTokenCount(commonQueryPack(g))
	}
	b.StopTimer()
	b.ReportMetric(float64(tokenCount), "tokens/query_pack")
}

func curatedResolverFixture() (*parser.ParseResult, map[string]bool) {
	result := &parser.ParseResult{
		Files: []parser.FileSymbols{
			{
				Path:          "app/main.py",
				Imports:       []string{"util"},
				ImportAliases: map[string]string{"myfoo": "util#foo"},
				Symbols: []parser.Symbol{
					{
						Name:  "runPy",
						Kind:  parser.SymbolFunction,
						Line:  1,
						Calls: []parser.CallSite{{Name: "myfoo"}},
					},
				},
			},
			{
				Path: "app/util.py",
				Symbols: []parser.Symbol{
					{Name: "foo", Kind: parser.SymbolFunction, Line: 1},
				},
			},
			{
				Path:          "web/main.ts",
				Imports:       []string{"./util"},
				ImportAliases: map[string]string{"util": "./util"},
				Symbols: []parser.Symbol{
					{
						Name:  "runTs",
						Kind:  parser.SymbolFunction,
						Line:  1,
						Calls: []parser.CallSite{{Name: "helper", Qualifier: "util"}},
					},
				},
			},
			{
				Path: "web/util.ts",
				Symbols: []parser.Symbol{
					{Name: "helper", Kind: parser.SymbolFunction, Line: 1},
				},
			},
			{
				Path: "svc/main.go",
				Symbols: []parser.Symbol{
					{
						Name:  "runSvc",
						Kind:  parser.SymbolFunction,
						Line:  1,
						Calls: []parser.CallSite{{Name: "Handle", Qualifier: "svc"}},
					},
				},
			},
			{
				Path: "svc/handler.go",
				Symbols: []parser.Symbol{
					{Name: "Handle", Kind: parser.SymbolMethod, Line: 1},
				},
			},
		},
	}

	expected := map[string]bool{
		"runPy->foo":     true,
		"runTs->helper":  true,
		"runSvc->Handle": true,
	}
	return result, expected
}

func edgeMetrics(g *graph.Graph, expected map[string]bool) (precision float64, recall float64) {
	actual := make(map[string]bool)
	for _, node := range g.Nodes {
		for _, targetID := range node.OutEdges {
			targetNode := g.Nodes[targetID]
			if targetNode == nil {
				continue
			}
			key := node.Symbol.Name + "->" + targetNode.Symbol.Name
			actual[key] = true
		}
	}

	tp := 0
	fp := 0
	fn := 0
	for key := range actual {
		if expected[key] {
			tp++
		} else {
			fp++
		}
	}
	for key := range expected {
		if !actual[key] {
			fn++
		}
	}

	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	return precision, recall
}

func commonQueryPack(g *graph.Graph) string {
	lines := make([]string, 0)
	for _, file := range g.Files() {
		nodes := g.NodesForFile(file)
		if len(nodes) == 0 {
			continue
		}
		lines = append(lines, "FILE "+file)
		for _, node := range nodes {
			lines = append(lines, node.Symbol.Name+" "+node.Symbol.Kind.String())
			if len(node.OutEdges) > 0 {
				lines = append(lines, "CALLEES "+node.Symbol.Name+"="+strings.Join(node.OutEdges, ","))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func estimateTokenCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return len(strings.Fields(text))
}
