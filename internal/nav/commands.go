package nav

import (
	"fmt"
	"os"
	"sort"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/search"
	"github.com/spf13/cobra"
)

func RunSymbol(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := OptionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}
	fuzzy, err := OptionalBoolFlag(cmd, "fuzzy", false)
	if err != nil {
		return err
	}
	limit, err := OptionalIntFlag(cmd, "limit", 10)
	if err != nil {
		return err
	}

	lookup, err := LoadLookup(rootPath)
	if err != nil {
		return err
	}
	var searchIndex *search.Index
	if fuzzy {
		searchIndex, err = search.Load(rootPath)
		if err != nil {
			return err
		}
	}
	matches := ResolveWithOptions(lookup, searchIndex, args[0], ResolveOptions{Fuzzy: fuzzy, Limit: limit})
	if len(matches) == 0 {
		return fmt.Errorf("symbol %q not found", args[0])
	}

	records := make([]SymbolRecord, 0, len(matches))
	for _, match := range matches {
		records = append(records, SymbolRecordFromNode(match))
	}

	if asJSON {
		return fileutil.PrintJSON(map[string]any{
			"query":   args[0],
			"matches": records,
		})
	}

	fmt.Printf("symbol matches for %q (%d)\n", args[0], len(records))
	for _, record := range records {
		fmt.Printf("- %s [%s] %s:%d\n", record.ID, record.Kind, record.File, record.Line)
		if record.Signature != "" {
			fmt.Printf("  sig: %s\n", record.Signature)
		}
	}
	return nil
}

func RunCallers(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := OptionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := LoadLookup(rootPath)
	if err != nil {
		return err
	}
	node, err := ResolveSingleSymbol(lookup, args[0])
	if err != nil {
		return err
	}

	callers := CollectCallers(lookup, node)
	if asJSON {
		return fileutil.PrintJSON(map[string]any{
			"query":   args[0],
			"symbol":  SymbolRecordFromNode(node),
			"callers": callers,
		})
	}

	fmt.Printf("callers for %s (%d)\n", node.ID, len(callers))
	if len(callers) == 0 {
		fmt.Println("no callers found")
		return nil
	}
	for _, caller := range callers {
		fmt.Printf("- %s [%s] %s:%d", caller.Symbol.ID, caller.Symbol.Kind, caller.Symbol.File, caller.Symbol.Line)
		if caller.Confidence != "" {
			fmt.Printf(" (%s)", caller.Confidence)
		}
		fmt.Println()
	}
	return nil
}

func RunCallees(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := OptionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := LoadLookup(rootPath)
	if err != nil {
		return err
	}
	node, err := ResolveSingleSymbol(lookup, args[0])
	if err != nil {
		return err
	}

	callees := CollectCallees(lookup, node)
	if asJSON {
		return fileutil.PrintJSON(map[string]any{
			"query":   args[0],
			"symbol":  SymbolRecordFromNode(node),
			"callees": callees,
		})
	}

	fmt.Printf("callees for %s (%d)\n", node.ID, len(callees))
	if len(callees) == 0 {
		fmt.Println("no callees found")
		return nil
	}
	for _, callee := range callees {
		fmt.Printf("- %s [%s] %s:%d", callee.Symbol.ID, callee.Symbol.Kind, callee.Symbol.File, callee.Symbol.Line)
		if callee.Confidence != "" {
			fmt.Printf(" (%s)", callee.Confidence)
		}
		fmt.Println()
	}
	return nil
}

func RunTrace(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	depth, err := cmd.Flags().GetInt("depth")
	if err != nil {
		return fmt.Errorf("failed to read --depth flag: %w", err)
	}
	if depth < 1 {
		return fmt.Errorf("--depth must be >= 1")
	}
	asJSON, err := OptionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := LoadLookup(rootPath)
	if err != nil {
		return err
	}
	startNode, err := ResolveSingleSymbol(lookup, args[0])
	if err != nil {
		return err
	}

	type queueItem struct {
		id    string
		depth int
	}
	queue := []queueItem{{id: startNode.ID, depth: 0}}
	seenDepth := map[string]int{startNode.ID: 0}
	hops := make([]TraceHop, 0)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= depth {
			continue
		}

		fromNode := lookup.ByID[current.id]
		if fromNode == nil {
			continue
		}

		for _, nextID := range fromNode.OutEdges {
			toNode := lookup.ByID[nextID]
			if toNode == nil {
				continue
			}
			nextDepth := current.depth + 1
			hops = append(hops, TraceHop{
				Depth:      nextDepth,
				From:       SymbolRecordFromNode(fromNode),
				To:         SymbolRecordFromNode(toNode),
				Confidence: lookup.EdgeConfidenceValue(fromNode.ID, toNode.ID),
			})
			if previousDepth, exists := seenDepth[nextID]; !exists || nextDepth < previousDepth {
				seenDepth[nextID] = nextDepth
				queue = append(queue, queueItem{id: nextID, depth: nextDepth})
			}
		}
	}

	sort.Slice(hops, func(i, j int) bool {
		if hops[i].Depth != hops[j].Depth {
			return hops[i].Depth < hops[j].Depth
		}
		if hops[i].From.ID != hops[j].From.ID {
			return hops[i].From.ID < hops[j].From.ID
		}
		return hops[i].To.ID < hops[j].To.ID
	})

	if asJSON {
		return fileutil.PrintJSON(map[string]any{
			"query": args[0],
			"start": SymbolRecordFromNode(startNode),
			"depth": depth,
			"hops":  hops,
		})
	}

	fmt.Printf("trace from %s depth=%d hops=%d\n", startNode.ID, depth, len(hops))
	if len(hops) == 0 {
		fmt.Println("no outgoing hops found")
		return nil
	}
	for _, hop := range hops {
		fmt.Printf("- d=%d %s -> %s", hop.Depth, hop.From.ID, hop.To.ID)
		if hop.Confidence != "" {
			fmt.Printf(" (%s)", hop.Confidence)
		}
		fmt.Println()
	}
	return nil
}

func RunPath(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := OptionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}

	lookup, err := LoadLookup(rootPath)
	if err != nil {
		return err
	}
	fromNode, err := ResolveSingleSymbol(lookup, args[0])
	if err != nil {
		return err
	}
	toNode, err := ResolveSingleSymbol(lookup, args[1])
	if err != nil {
		return err
	}

	pathIDs := ShortestPath(lookup, fromNode.ID, toNode.ID)
	if len(pathIDs) == 0 {
		return fmt.Errorf("no path found between %s and %s", fromNode.ID, toNode.ID)
	}

	pathNodes := make([]SymbolRecord, 0, len(pathIDs))
	edges := make([]map[string]string, 0, len(pathIDs)-1)
	for i, id := range pathIDs {
		node := lookup.ByID[id]
		if node == nil {
			continue
		}
		pathNodes = append(pathNodes, SymbolRecordFromNode(node))
		if i == 0 {
			continue
		}
		prevID := pathIDs[i-1]
		edges = append(edges, map[string]string{
			"from_id":    prevID,
			"to_id":      id,
			"confidence": lookup.EdgeConfidenceValue(prevID, id),
		})
	}

	if asJSON {
		return fileutil.PrintJSON(map[string]any{
			"from":   SymbolRecordFromNode(fromNode),
			"to":     SymbolRecordFromNode(toNode),
			"length": len(pathNodes) - 1,
			"path":   pathNodes,
			"edges":  edges,
		})
	}

	fmt.Printf("path %s -> %s length=%d\n", fromNode.ID, toNode.ID, len(pathNodes)-1)
	for i, node := range pathNodes {
		fmt.Printf("%d. %s [%s] %s:%d\n", i+1, node.ID, node.Kind, node.File, node.Line)
	}
	return nil
}

func OptionalBoolFlag(cmd *cobra.Command, name string, defaultValue bool) (bool, error) {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return defaultValue, nil
	}
	value, err := cmd.Flags().GetBool(name)
	if err != nil {
		return false, fmt.Errorf("failed to read --%s flag: %w", name, err)
	}
	return value, nil
}

func OptionalIntFlag(cmd *cobra.Command, name string, defaultValue int) (int, error) {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return defaultValue, nil
	}
	value, err := cmd.Flags().GetInt(name)
	if err != nil {
		return 0, fmt.Errorf("failed to read --%s flag: %w", name, err)
	}
	return value, nil
}
