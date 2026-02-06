package nav

import "sort"

func CollectCallers(l *Lookup, node *IndexNode) []EdgeRecord {
	out := make([]EdgeRecord, 0, len(node.InEdges))
	for _, callerID := range node.InEdges {
		caller := l.ByID[callerID]
		if caller == nil {
			continue
		}
		out = append(out, EdgeRecord{
			Symbol:     SymbolRecordFromNode(caller),
			Confidence: l.EdgeConfidenceValue(caller.ID, node.ID),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Symbol.ID < out[j].Symbol.ID
	})
	return out
}

func CollectCallees(l *Lookup, node *IndexNode) []EdgeRecord {
	out := make([]EdgeRecord, 0, len(node.OutEdges))
	for _, calleeID := range node.OutEdges {
		callee := l.ByID[calleeID]
		if callee == nil {
			continue
		}
		out = append(out, EdgeRecord{
			Symbol:     SymbolRecordFromNode(callee),
			Confidence: l.EdgeConfidenceValue(node.ID, callee.ID),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Symbol.ID < out[j].Symbol.ID
	})
	return out
}

func ShortestPath(lookup *Lookup, fromID, toID string) []string {
	if fromID == toID {
		return []string{fromID}
	}

	queue := []string{fromID}
	visited := map[string]bool{fromID: true}
	parent := map[string]string{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		node := lookup.ByID[current]
		if node == nil {
			continue
		}
		for _, nextID := range node.OutEdges {
			if visited[nextID] {
				continue
			}
			visited[nextID] = true
			parent[nextID] = current
			if nextID == toID {
				return ReconstructPath(parent, fromID, toID)
			}
			queue = append(queue, nextID)
		}
	}

	return nil
}

func ReconstructPath(parent map[string]string, fromID, toID string) []string {
	out := []string{toID}
	for current := toID; current != fromID; {
		prev, ok := parent[current]
		if !ok {
			return nil
		}
		out = append(out, prev)
		current = prev
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
