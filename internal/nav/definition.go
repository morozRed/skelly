package nav

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/lsp"
	"github.com/spf13/cobra"
)

func RunDefinition(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := OptionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}
	useLSP, err := OptionalBoolFlag(cmd, "lsp", false)
	if err != nil {
		return err
	}

	lookup, err := LoadLookup(rootPath)
	if err != nil {
		return err
	}
	node, err := ResolveSymbolOrLocation(lookup, args[0])
	if err != nil {
		return err
	}
	lspStatus, err := ResolveLSPStatus(node, useLSP)
	if err != nil {
		return err
	}
	source := "parser"
	if useLSP && lspStatus != nil && lspStatus.Available {
		if lspNode, lspErr := ResolveDefinitionViaLSP(rootPath, lookup, node, lspStatus); lspErr == nil && lspNode != nil {
			node = lspNode
			source = "lsp"
		}
	}

	record := SymbolRecordFromNode(node)
	if asJSON {
		payload := map[string]any{
			"query":      args[0],
			"definition": record,
			"source":     source,
		}
		if lspStatus != nil {
			payload["lsp"] = lspStatus
		}
		return fileutil.PrintJSON(payload)
	}

	fmt.Printf("definition for %q\n", args[0])
	fmt.Printf("- %s [%s] %s:%d\n", record.ID, record.Kind, record.File, record.Line)
	fmt.Printf("  source: %s\n", source)
	if record.Signature != "" {
		fmt.Printf("  sig: %s\n", record.Signature)
	}
	if useLSP && lspStatus != nil && !lspStatus.Available {
		fmt.Printf("note: lsp unavailable (%s); run skelly doctor for details\n", lspStatus.Reason)
	}
	return nil
}

func RunReferences(cmd *cobra.Command, args []string) error {
	rootPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	asJSON, err := OptionalBoolFlag(cmd, "json", false)
	if err != nil {
		return err
	}
	useLSP, err := OptionalBoolFlag(cmd, "lsp", false)
	if err != nil {
		return err
	}

	lookup, err := LoadLookup(rootPath)
	if err != nil {
		return err
	}
	node, err := ResolveSymbolOrLocation(lookup, args[0])
	if err != nil {
		return err
	}
	lspStatus, err := ResolveLSPStatus(node, useLSP)
	if err != nil {
		return err
	}

	references := CollectCallers(lookup, node)
	source := "parser"
	if useLSP {
		for i := range references {
			references[i].Source = "parser"
		}
		if lspStatus != nil && lspStatus.Available {
			if lspRefs, lspErr := ResolveReferencesViaLSP(rootPath, lookup, node, lspStatus); lspErr == nil && len(lspRefs) > 0 {
				references = lspRefs
				source = "lsp"
			}
		}
	}

	if asJSON {
		payload := map[string]any{
			"query":      args[0],
			"symbol":     SymbolRecordFromNode(node),
			"references": references,
			"source":     source,
		}
		if lspStatus != nil {
			payload["lsp"] = lspStatus
		}
		return fileutil.PrintJSON(payload)
	}

	fmt.Printf("references for %s (%d)\n", node.ID, len(references))
	fmt.Printf("source: %s\n", source)
	if len(references) == 0 {
		fmt.Println("no references found")
		return nil
	}
	for _, ref := range references {
		fmt.Printf("- %s [%s] %s:%d", ref.Symbol.ID, ref.Symbol.Kind, ref.Symbol.File, ref.Symbol.Line)
		if ref.Confidence != "" {
			fmt.Printf(" (%s)", ref.Confidence)
		}
		if ref.Source != "" {
			fmt.Printf(" source=%s", ref.Source)
		}
		fmt.Println()
	}
	if useLSP && lspStatus != nil && !lspStatus.Available {
		fmt.Printf("note: lsp unavailable (%s); run skelly doctor for details\n", lspStatus.Reason)
	}
	return nil
}

func ResolveSymbolOrLocation(lookup *Lookup, query string) (*IndexNode, error) {
	node, err := ResolveSingleSymbol(lookup, query)
	if err == nil {
		return node, nil
	}
	if !strings.Contains(query, ":") {
		return nil, err
	}

	file, line, ok := ParseLocationQuery(query)
	if !ok {
		return nil, err
	}

	candidateNode := ResolveNodeAtLocation(lookup, file, line)
	if candidateNode != nil {
		return candidateNode, nil
	}

	return nil, fmt.Errorf("symbol %q not found", query)
}

func ResolveNodeAtLocation(lookup *Lookup, file string, line int) *IndexNode {
	exact := make([]*IndexNode, 0)
	nearest := make([]*IndexNode, 0)
	bestLine := -1
	for _, candidate := range lookup.ByID {
		if candidate.File != file {
			continue
		}
		if candidate.Line == line {
			exact = append(exact, candidate)
			continue
		}
		if candidate.Line <= line {
			if candidate.Line > bestLine {
				nearest = nearest[:0]
				bestLine = candidate.Line
			}
			if candidate.Line == bestLine {
				nearest = append(nearest, candidate)
			}
		}
	}

	if len(exact) > 0 {
		sort.Slice(exact, func(i, j int) bool {
			return exact[i].ID < exact[j].ID
		})
		return exact[0]
	}

	if len(nearest) > 0 {
		sort.Slice(nearest, func(i, j int) bool {
			return nearest[i].ID < nearest[j].ID
		})
		return nearest[0]
	}

	return nil
}

func ParseLocationQuery(query string) (file string, line int, ok bool) {
	idx := strings.LastIndex(query, ":")
	if idx <= 0 || idx >= len(query)-1 {
		return "", 0, false
	}
	file = strings.TrimSpace(query[:idx])
	lineRaw := strings.TrimSpace(query[idx+1:])
	if file == "" || lineRaw == "" {
		return "", 0, false
	}
	parsedLine, err := strconv.Atoi(lineRaw)
	if err != nil || parsedLine <= 0 {
		return "", 0, false
	}
	return file, parsedLine, true
}

func ResolveDefinitionViaLSP(rootPath string, lookup *Lookup, node *IndexNode, status *LSPStatus) (*IndexNode, error) {
	if status == nil || !status.Available {
		return nil, nil
	}
	locations, err := lsp.QueryDefinition(rootPath, node.File, node.Line, 1, status.Server)
	if err != nil {
		return nil, err
	}
	for _, location := range locations {
		candidate := ResolveNodeAtLocation(lookup, location.File, location.Line)
		if candidate != nil {
			return candidate, nil
		}
	}
	return nil, nil
}

func ResolveReferencesViaLSP(rootPath string, lookup *Lookup, node *IndexNode, status *LSPStatus) ([]EdgeRecord, error) {
	if status == nil || !status.Available {
		return nil, nil
	}
	locations, err := lsp.QueryReferences(rootPath, node.File, node.Line, 1, status.Server)
	if err != nil {
		return nil, err
	}
	unique := make(map[string]*IndexNode)
	for _, location := range locations {
		candidate := ResolveNodeAtLocation(lookup, location.File, location.Line)
		if candidate == nil || candidate.ID == node.ID {
			continue
		}
		unique[candidate.ID] = candidate
	}
	out := make([]EdgeRecord, 0, len(unique))
	for _, candidate := range unique {
		out = append(out, EdgeRecord{
			Symbol:     SymbolRecordFromNode(candidate),
			Confidence: lookup.EdgeConfidenceValue(candidate.ID, node.ID),
			Source:     "lsp",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Symbol.ID < out[j].Symbol.ID
	})
	return out, nil
}
