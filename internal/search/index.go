package search

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/output"
)

const (
	IndexFile = "search-index.json"
	Version   = "search-index-v1"
)

var tokenPattern = regexp.MustCompile(`[a-z0-9_]+`)

type Document struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Signature string         `json:"signature,omitempty"`
	File      string         `json:"file"`
	Line      int            `json:"line"`
	Doc       string         `json:"doc,omitempty"`
	Length    int            `json:"length"`
	Terms     map[string]int `json:"terms"`
}

type Index struct {
	Version       string         `json:"version"`
	DocumentCount int            `json:"document_count"`
	AvgDocLength  float64        `json:"avg_doc_length"`
	DocFreq       map[string]int `json:"doc_freq"`
	Documents     []Document     `json:"documents"`
}

type Result struct {
	ID    string
	Score float64
}

func Build(g *graph.Graph) *Index {
	if g == nil {
		return &Index{Version: Version, DocFreq: map[string]int{}}
	}

	documents := make([]Document, 0, len(g.Nodes))
	docFreq := make(map[string]int)
	totalLength := 0

	for _, file := range g.Files() {
		for _, node := range g.NodesForFile(file) {
			terms := buildTerms(node.Symbol.Name, node.Symbol.Signature, node.File, node.Symbol.Doc)
			length := 0
			for _, count := range terms {
				length += count
			}
			if length == 0 {
				continue
			}

			documents = append(documents, Document{
				ID:        node.ID,
				Name:      node.Symbol.Name,
				Kind:      node.Symbol.Kind.String(),
				Signature: node.Symbol.Signature,
				File:      node.File,
				Line:      node.Symbol.Line,
				Doc:       node.Symbol.Doc,
				Length:    length,
				Terms:     terms,
			})
			totalLength += length

			for term := range terms {
				docFreq[term]++
			}
		}
	}

	sort.Slice(documents, func(i, j int) bool {
		return documents[i].ID < documents[j].ID
	})

	avgDocLength := 0.0
	if len(documents) > 0 {
		avgDocLength = float64(totalLength) / float64(len(documents))
	}

	return &Index{
		Version:       Version,
		DocumentCount: len(documents),
		AvgDocLength:  avgDocLength,
		DocFreq:       docFreq,
		Documents:     documents,
	}
}

func Write(contextDir string, g *graph.Graph) error {
	index := Build(g)
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode search index: %w", err)
	}
	data = append(data, '\n')
	return fileutil.WriteIfChanged(filepath.Join(contextDir, IndexFile), data)
}

func Load(rootPath string) (*Index, error) {
	path := filepath.Join(rootPath, output.ContextDir, IndexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("search index missing at %s (run skelly update)", path)
		}
		return nil, fmt.Errorf("failed to read search index: %w", err)
	}

	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to decode search index: %w", err)
	}
	if index.DocFreq == nil {
		index.DocFreq = map[string]int{}
	}
	return &index, nil
}

func Search(index *Index, query string, limit int) []Result {
	if index == nil || len(index.Documents) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	seenTerms := make(map[string]bool, len(queryTerms))
	uniqueTerms := make([]string, 0, len(queryTerms))
	for _, term := range queryTerms {
		if seenTerms[term] {
			continue
		}
		seenTerms[term] = true
		uniqueTerms = append(uniqueTerms, term)
	}

	k1 := 1.2
	b := 0.75
	n := float64(index.DocumentCount)
	avgLen := index.AvgDocLength
	if avgLen <= 0 {
		avgLen = 1
	}

	results := make([]Result, 0)
	for _, doc := range index.Documents {
		score := 0.0
		docLen := float64(doc.Length)
		for _, term := range uniqueTerms {
			tf := float64(doc.Terms[term])
			if tf <= 0 {
				continue
			}
			df := float64(index.DocFreq[term])
			if df <= 0 {
				continue
			}
			idf := math.Log(1.0 + ((n - df + 0.5) / (df + 0.5)))
			numerator := tf * (k1 + 1.0)
			denominator := tf + k1*(1.0-b+b*(docLen/avgLen))
			score += idf * (numerator / denominator)
		}
		if score > 0 {
			results = append(results, Result{ID: doc.ID, Score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ID < results[j].ID
	})

	if len(results) > limit {
		results = results[:limit]
	}
	if len(results) == 0 {
		fallback := fuzzyNameFallback(index.Documents, query, limit)
		if len(fallback) > 0 {
			return fallback
		}
	}
	return results
}

func buildTerms(name, signature, filePath, doc string) map[string]int {
	terms := make(map[string]int)
	addWeighted(terms, name, 4)
	addWeighted(terms, signature, 2)
	addWeighted(terms, filePath, 2)
	addWeighted(terms, doc, 1)
	return terms
}

func addWeighted(terms map[string]int, value string, weight int) {
	if weight <= 0 {
		return
	}
	for _, token := range tokenize(value) {
		terms[token] += weight
	}
}

func tokenize(value string) []string {
	value = strings.ToLower(value)
	if value == "" {
		return nil
	}
	return tokenPattern.FindAllString(value, -1)
}

func fuzzyNameFallback(documents []Document, query string, limit int) []Result {
	needle := normalizeForFuzzy(query)
	if needle == "" {
		return nil
	}

	results := make([]Result, 0)
	for _, doc := range documents {
		candidate := normalizeForFuzzy(doc.Name)
		if candidate == "" {
			continue
		}
		distance := levenshteinDistance(needle, candidate)
		threshold := len(candidate) / 3
		if threshold < 2 {
			threshold = 2
		}
		if distance > threshold {
			continue
		}
		results = append(results, Result{ID: doc.ID, Score: 1.0 / float64(1+distance)})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ID < results[j].ID
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func normalizeForFuzzy(value string) string {
	tokens := tokenize(value)
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, "")
}

func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			ins := current[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			current[j] = minInt(ins, minInt(del, sub))
		}
		prev = current
	}

	return prev[len(b)]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
