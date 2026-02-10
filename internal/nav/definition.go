package nav

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
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

	record := SymbolRecordFromNode(node)
	if asJSON {
		payload := map[string]any{
			"query":      args[0],
			"definition": record,
		}
		if lspStatus != nil {
			payload["lsp"] = lspStatus
		}
		return fileutil.PrintJSON(payload)
	}

	fmt.Printf("definition for %q\n", args[0])
	fmt.Printf("- %s [%s] %s:%d\n", record.ID, record.Kind, record.File, record.Line)
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
	if useLSP {
		for i := range references {
			references[i].Source = "parser"
		}
	}

	if asJSON {
		payload := map[string]any{
			"query":      args[0],
			"symbol":     SymbolRecordFromNode(node),
			"references": references,
		}
		if lspStatus != nil {
			payload["lsp"] = lspStatus
		}
		return fileutil.PrintJSON(payload)
	}

	fmt.Printf("references for %s (%d)\n", node.ID, len(references))
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

	file, line, ok := parseLocationQuery(query)
	if !ok {
		return nil, err
	}

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
		if len(exact) == 1 {
			return exact[0], nil
		}
		options := make([]string, 0, len(exact))
		for _, candidate := range exact {
			options = append(options, candidate.ID)
		}
		return nil, fmt.Errorf("location %q is ambiguous; use one of: %s", query, strings.Join(options, ", "))
	}

	if len(nearest) > 0 {
		sort.Slice(nearest, func(i, j int) bool {
			return nearest[i].ID < nearest[j].ID
		})
		return nearest[0], nil
	}

	return nil, fmt.Errorf("symbol %q not found", query)
}

func parseLocationQuery(query string) (file string, line int, ok bool) {
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
