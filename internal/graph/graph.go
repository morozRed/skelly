package graph

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/skelly-dev/skelly/internal/parser"
)

// Node represents a symbol in the dependency graph
type Node struct {
	ID                string // stable symbol ID (file|line|kind|name|sig-hash)
	Symbol            parser.Symbol
	File              string
	OutEdges          []string          // symbols this node calls/references
	OutEdgeConfidence map[string]string // target ID -> resolved|ambiguous|heuristic
	InEdges           []string          // symbols that call/reference this node
	PageRank          float64           // importance score
}

// Graph represents the codebase dependency graph
type Graph struct {
	Nodes     map[string]*Node    // ID -> Node
	FileNodes map[string][]string // file -> list of node IDs in that file
}

type symbolLookups struct {
	global                map[string][]string
	byFile                map[string]map[string][]string
	byFileMethods         map[string]map[string][]string
	byModule              map[string]map[string][]string
	importAliasCandidates map[string]map[string][]string
}

// NewGraph creates a new empty graph
func NewGraph() *Graph {
	return &Graph{
		Nodes:     make(map[string]*Node),
		FileNodes: make(map[string][]string),
	}
}

// BuildFromParseResult constructs the graph from parsed files
func BuildFromParseResult(result *parser.ParseResult) *Graph {
	return buildFromParseResult(result, nil, true)
}

// BuildFromParseResultForSources constructs a graph by recomputing edges only for source files.
// Nodes for every file are still materialized so edges can target unchanged symbols.
func BuildFromParseResultForSources(result *parser.ParseResult, sourceFiles map[string]bool) *Graph {
	return buildFromParseResult(result, sourceFiles, false)
}

func buildFromParseResult(result *parser.ParseResult, sourceFiles map[string]bool, withPageRank bool) *Graph {
	g := NewGraph()

	// First pass: create all nodes
	for _, file := range result.Files {
		for _, sym := range file.Symbols {
			id := makeNodeID(file.Path, sym)
			node := &Node{
				ID:                id,
				Symbol:            sym,
				File:              file.Path,
				OutEdges:          make([]string, 0),
				OutEdgeConfidence: make(map[string]string),
				InEdges:           make([]string, 0),
			}
			g.Nodes[id] = node
			g.FileNodes[file.Path] = append(g.FileNodes[file.Path], id)
		}
	}

	// Build symbol lookup for edge resolution
	lookups := buildSymbolLookup(result)

	// Second pass: build edges based on calls
	for _, file := range result.Files {
		if sourceFiles != nil && !sourceFiles[file.Path] {
			continue
		}
		for _, sym := range file.Symbols {
			srcID := makeNodeID(file.Path, sym)
			srcNode := g.Nodes[srcID]

			for _, call := range sym.Calls {
				// Try to resolve the call to a node
				if targetIDs, confidence, ok := lookups.resolve(file.Path, sym, call); ok {
					for _, targetID := range targetIDs {
						if targetID != srcID { // Don't self-reference
							srcNode.OutEdges = append(srcNode.OutEdges, targetID)
							srcNode.OutEdgeConfidence[targetID] = mergeConfidence(
								srcNode.OutEdgeConfidence[targetID],
								confidence,
							)
							if targetNode, ok := g.Nodes[targetID]; ok {
								targetNode.InEdges = append(targetNode.InEdges, srcID)
							}
						}
					}
				}
			}
		}
	}

	g.normalizeEdges()

	// Calculate PageRank
	if withPageRank {
		g.calculatePageRank(20, 0.85)
	}

	return g
}

// buildSymbolLookup indexes symbols by name with file/module scopes for resolution.
func buildSymbolLookup(result *parser.ParseResult) symbolLookups {
	lookup := symbolLookups{
		global:                make(map[string][]string),
		byFile:                make(map[string]map[string][]string),
		byFileMethods:         make(map[string]map[string][]string),
		byModule:              make(map[string]map[string][]string),
		importAliasCandidates: make(map[string]map[string][]string),
	}

	for _, file := range result.Files {
		if _, ok := lookup.byFile[file.Path]; !ok {
			lookup.byFile[file.Path] = make(map[string][]string)
		}
		if _, ok := lookup.byFileMethods[file.Path]; !ok {
			lookup.byFileMethods[file.Path] = make(map[string][]string)
		}

		module := moduleName(file.Path)
		if _, ok := lookup.byModule[module]; !ok {
			lookup.byModule[module] = make(map[string][]string)
		}

		for _, sym := range file.Symbols {
			id := makeNodeID(file.Path, sym)
			lookup.global[sym.Name] = append(lookup.global[sym.Name], id)
			lookup.byFile[file.Path][sym.Name] = append(lookup.byFile[file.Path][sym.Name], id)
			lookup.byModule[module][sym.Name] = append(lookup.byModule[module][sym.Name], id)
			if sym.Kind == parser.SymbolMethod {
				lookup.byFileMethods[file.Path][sym.Name] = append(lookup.byFileMethods[file.Path][sym.Name], id)
			}
		}
	}

	for name, ids := range lookup.global {
		lookup.global[name] = dedupeAndSort(ids)
	}
	for file, byName := range lookup.byFile {
		for name, ids := range byName {
			lookup.byFile[file][name] = dedupeAndSort(ids)
		}
	}
	for file, byName := range lookup.byFileMethods {
		for name, ids := range byName {
			lookup.byFileMethods[file][name] = dedupeAndSort(ids)
		}
	}
	for module, byName := range lookup.byModule {
		for name, ids := range byName {
			lookup.byModule[module][name] = dedupeAndSort(ids)
		}
	}

	lookup.importAliasCandidates = buildImportAliasCandidates(result)

	return lookup
}

// calculatePageRank computes importance scores for all nodes
func (g *Graph) calculatePageRank(iterations int, dampingFactor float64) {
	n := float64(len(g.Nodes))
	if n == 0 {
		return
	}

	// Initialize all nodes with equal rank
	for _, node := range g.Nodes {
		node.PageRank = 1.0 / n
	}

	// Iterate
	for i := 0; i < iterations; i++ {
		newRanks := make(map[string]float64)

		for id, node := range g.Nodes {
			rank := (1 - dampingFactor) / n

			// Sum contributions from incoming edges
			for _, inID := range node.InEdges {
				if inNode, ok := g.Nodes[inID]; ok {
					outDegree := float64(len(inNode.OutEdges))
					if outDegree > 0 {
						rank += dampingFactor * (inNode.PageRank / outDegree)
					}
				}
			}

			newRanks[id] = rank
		}

		// Update ranks
		for id, rank := range newRanks {
			g.Nodes[id].PageRank = rank
		}
	}
}

// TopNodes returns the most important nodes by PageRank
func (g *Graph) TopNodes(n int) []*Node {
	nodes := make([]*Node, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].PageRank == nodes[j].PageRank {
			return nodes[i].ID < nodes[j].ID
		}
		return nodes[i].PageRank > nodes[j].PageRank
	})

	if n > len(nodes) {
		n = len(nodes)
	}
	return nodes[:n]
}

func (g *Graph) normalizeEdges() {
	for _, node := range g.Nodes {
		node.OutEdges = dedupeAndSort(node.OutEdges)
		node.InEdges = dedupeAndSort(node.InEdges)

		// Keep confidence metadata aligned with normalized edge list.
		cleanedConfidence := make(map[string]string, len(node.OutEdges))
		for _, edge := range node.OutEdges {
			cleanedConfidence[edge] = node.OutEdgeConfidence[edge]
		}
		node.OutEdgeConfidence = cleanedConfidence
	}
}

func dedupeAndSort(values []string) []string {
	if len(values) == 0 {
		return values
	}

	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// NodesForFile returns all nodes in a file, sorted by line number
func (g *Graph) NodesForFile(file string) []*Node {
	ids, ok := g.FileNodes[file]
	if !ok {
		return nil
	}

	nodes := make([]*Node, 0, len(ids))
	for _, id := range ids {
		if node, ok := g.Nodes[id]; ok {
			nodes = append(nodes, node)
		}
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Symbol.Line == nodes[j].Symbol.Line {
			return nodes[i].ID < nodes[j].ID
		}
		return nodes[i].Symbol.Line < nodes[j].Symbol.Line
	})

	return nodes
}

// Files returns all unique files in the graph
func (g *Graph) Files() []string {
	files := make([]string, 0, len(g.FileNodes))
	for file := range g.FileNodes {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

func makeNodeID(file string, symbol parser.Symbol) string {
	if symbol.ID != "" {
		return symbol.ID
	}
	return parser.StableSymbolID(file, symbol)
}

// ParseNodeID extracts file and symbol from a node ID
func ParseNodeID(id string) (file, symbol string) {
	parts := strings.Split(id, "|")
	if len(parts) >= 4 {
		return parts[0], parts[3]
	}
	return id, ""
}

func (l symbolLookups) resolve(sourceFile string, sourceSymbol parser.Symbol, call parser.CallSite) (targetIDs []string, confidence string, ok bool) {
	callName := strings.TrimSpace(call.Name)
	if callName == "" {
		return nil, "", false
	}

	if callIsReceiverScoped(call) {
		if ids := l.byFileMethods[sourceFile][callName]; len(ids) > 0 {
			return chooseUnique(ids, "resolved")
		}
		if sourceSymbol.Kind == parser.SymbolMethod {
			if ids := l.byFile[sourceFile][callName]; len(ids) > 0 {
				return chooseUnique(ids, "resolved")
			}
		}
	}

	if byName, exists := l.byFile[sourceFile]; exists {
		if ids := byName[callName]; len(ids) > 0 {
			return chooseUnique(ids, "resolved")
		}
	}

	qualifier := primaryQualifier(call.Qualifier)
	aliasMatched := false
	if qualifier != "" {
		if byAlias := l.importAliasCandidates[sourceFile]; byAlias != nil {
			if candidateFiles, exists := byAlias[qualifier]; exists {
				aliasMatched = true
				if ids := l.collectFromFiles(candidateFiles, callName); len(ids) > 0 {
					return chooseUnique(ids, "heuristic")
				}
			}
		}
	}
	if qualifier != "" && !callIsReceiverScoped(call) && !aliasMatched {
		return nil, "", false
	}

	module := moduleName(sourceFile)
	if byName, exists := l.byModule[module]; exists {
		if ids := byName[callName]; len(ids) > 0 {
			return chooseUnique(ids, "heuristic")
		}
	}

	if ids := l.global[callName]; len(ids) > 0 {
		return chooseUnique(ids, "heuristic")
	}

	return nil, "", false
}

func callIsReceiverScoped(call parser.CallSite) bool {
	switch strings.TrimSpace(call.Receiver) {
	case "self", "this", "cls":
		return true
	}
	switch strings.TrimSpace(call.Qualifier) {
	case "self", "this", "cls":
		return true
	}
	return false
}

func primaryQualifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, "."); idx != -1 {
		value = value[:idx]
	}
	value = strings.TrimPrefix(value, "self.")
	value = strings.TrimPrefix(value, "this.")
	return strings.TrimSpace(value)
}

func chooseUnique(targetIDs []string, confidence string) ([]string, string, bool) {
	targetIDs = dedupeAndSort(targetIDs)
	if len(targetIDs) == 1 {
		return targetIDs, confidence, true
	}
	return nil, "", false
}

func (l symbolLookups) collectFromFiles(files []string, callName string) []string {
	out := make([]string, 0)
	for _, file := range files {
		byName := l.byFile[file]
		if byName == nil {
			continue
		}
		out = append(out, byName[callName]...)
	}
	return dedupeAndSort(out)
}

func buildImportAliasCandidates(result *parser.ParseResult) map[string]map[string][]string {
	out := make(map[string]map[string][]string)
	allFiles := make([]string, 0, len(result.Files))
	for _, file := range result.Files {
		allFiles = append(allFiles, file.Path)
	}

	for _, source := range result.Files {
		aliases := make(map[string]string, len(source.ImportAliases)+len(source.Imports))
		for alias, target := range source.ImportAliases {
			aliases[strings.TrimSpace(alias)] = strings.TrimSpace(target)
		}
		for _, importPath := range source.Imports {
			alias := defaultAliasFromImport(importPath)
			if alias == "" {
				continue
			}
			if _, exists := aliases[alias]; !exists {
				aliases[alias] = importPath
			}
		}
		if len(aliases) == 0 {
			continue
		}

		sourceCandidates := make(map[string][]string)
		for alias, importPath := range aliases {
			candidates := matchImportCandidates(source.Path, importPath, allFiles)
			if len(candidates) == 0 {
				continue
			}
			sourceCandidates[alias] = dedupeAndSort(candidates)
		}
		if len(sourceCandidates) > 0 {
			out[source.Path] = sourceCandidates
		}
	}

	return out
}

func matchImportCandidates(sourceFile, importPath string, allFiles []string) []string {
	importPath = strings.TrimSpace(strings.Trim(importPath, `"'`))
	if importPath == "" {
		return nil
	}

	matches := make([]string, 0)
	for _, target := range allFiles {
		if importMatchesFile(sourceFile, importPath, target) {
			matches = append(matches, target)
		}
	}
	return matches
}

func importMatchesFile(sourceFile, importPath, targetFile string) bool {
	targetNoExt := strings.TrimSuffix(targetFile, filepath.Ext(targetFile))
	targetDir := filepath.Dir(targetFile)
	targetBase := strings.TrimSuffix(filepath.Base(targetFile), filepath.Ext(targetFile))

	if strings.HasPrefix(importPath, ".") {
		resolved := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), importPath))
		return resolved == targetNoExt || resolved == targetDir
	}

	normalizedImport := strings.TrimPrefix(filepath.ToSlash(importPath), "/")
	normalizedNoExt := filepath.ToSlash(targetNoExt)
	normalizedDir := filepath.ToSlash(targetDir)
	normalizedBase := filepath.ToSlash(targetBase)

	return normalizedImport == normalizedNoExt ||
		normalizedImport == normalizedDir ||
		normalizedImport == normalizedBase ||
		strings.HasSuffix(normalizedImport, "/"+normalizedNoExt) ||
		strings.HasSuffix(normalizedImport, "/"+normalizedDir) ||
		strings.HasSuffix(normalizedImport, "/"+normalizedBase)
}

func defaultAliasFromImport(importPath string) string {
	importPath = strings.TrimSpace(strings.Trim(importPath, `"'`))
	if importPath == "" {
		return ""
	}
	segments := strings.Split(importPath, "/")
	return strings.TrimSpace(segments[len(segments)-1])
}

func moduleName(file string) string {
	dir := filepath.Dir(file)
	if dir == "." {
		return "root"
	}
	parts := strings.Split(dir, string(filepath.Separator))
	return parts[0]
}

func mergeConfidence(current, next string) string {
	rank := map[string]int{
		"ambiguous": 1,
		"heuristic": 2,
		"resolved":  3,
	}
	if rank[next] >= rank[current] {
		return next
	}
	return current
}
