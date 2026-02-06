package languages

import (
	"context"
	"strings"

	"github.com/morozRed/skelly/internal/parser"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
)

// GoParser implements parsing for Go source files
type GoParser struct {
	parser *sitter.Parser
}

// NewGoParser creates a new Go parser
func NewGoParser() *GoParser {
	p := sitter.NewParser()
	p.SetLanguage(golang.GetLanguage())
	return &GoParser{parser: p}
}

func (g *GoParser) Language() string {
	return "go"
}

func (g *GoParser) Extensions() []string {
	return []string{".go"}
}

func (g *GoParser) Parse(filename string, content []byte) (*parser.FileSymbols, error) {
	tree, err := g.parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	result := &parser.FileSymbols{
		Path:          filename,
		Language:      "go",
		Symbols:       make([]parser.Symbol, 0),
		Imports:       make([]string, 0),
		ImportAliases: make(map[string]string),
	}

	root := tree.RootNode()
	g.extractSymbols(root, content, result)

	return result, nil
}

func (g *GoParser) extractSymbols(node *sitter.Node, content []byte, result *parser.FileSymbols) {
	switch node.Type() {
	case "function_declaration":
		sym := g.extractFunction(node, content)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}

	case "method_declaration":
		sym := g.extractMethod(node, content)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}

	case "type_declaration":
		syms := g.extractTypeDecl(node, content)
		result.Symbols = append(result.Symbols, syms...)

	case "import_declaration":
		imports, aliases := g.extractImports(node, content)
		result.Imports = append(result.Imports, imports...)
		result.ImportAliases = mergeImportAliases(result.ImportAliases, aliases)
	}

	// Recurse into children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		g.extractSymbols(child, content, result)
	}
}

func (g *GoParser) extractFunction(node *sitter.Node, content []byte) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	sig := g.buildFunctionSignature(node, content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolFunction,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
		Calls:     g.extractCalls(node.ChildByFieldName("body"), content),
	}
}

func (g *GoParser) extractMethod(node *sitter.Node, content []byte) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)

	// Get receiver type
	receiver := ""
	receiverNode := node.ChildByFieldName("receiver")
	if receiverNode != nil {
		receiver = receiverNode.Content(content)
	}

	sig := g.buildFunctionSignature(node, content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolMethod,
		Signature: receiver + " " + sig,
		Line:      int(node.StartPoint().Row) + 1,
		Calls:     g.extractCalls(node.ChildByFieldName("body"), content),
	}
}

func (g *GoParser) extractTypeDecl(node *sitter.Node, content []byte) []parser.Symbol {
	symbols := make([]parser.Symbol, 0)

	// Iterate through type specs
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_spec" {
			nameNode := child.ChildByFieldName("name")
			typeNode := child.ChildByFieldName("type")

			if nameNode == nil {
				continue
			}

			name := nameNode.Content(content)
			kind := parser.SymbolStruct

			if typeNode != nil {
				switch typeNode.Type() {
				case "struct_type":
					kind = parser.SymbolStruct
				case "interface_type":
					kind = parser.SymbolInterface
				}
			}

			symbols = append(symbols, parser.Symbol{
				Name:      name,
				Kind:      kind,
				Signature: g.buildTypeSignature(child, content),
				Line:      int(child.StartPoint().Row) + 1,
			})
		}
	}

	return symbols
}

func (g *GoParser) extractImports(node *sitter.Node, content []byte) ([]string, map[string]string) {
	imports := make([]string, 0)
	aliases := make(map[string]string)

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "import_spec" {
			importPath, alias := g.readImportSpec(child, content)
			if importPath == "" {
				continue
			}
			imports = append(imports, importPath)
			if alias != "" {
				aliases[alias] = importPath
			}
		} else if child.Type() == "import_spec_list" {
			// Handle grouped imports
			for j := 0; j < int(child.ChildCount()); j++ {
				spec := child.Child(j)
				if spec.Type() == "import_spec" {
					importPath, alias := g.readImportSpec(spec, content)
					if importPath == "" {
						continue
					}
					imports = append(imports, importPath)
					if alias != "" {
						aliases[alias] = importPath
					}
				}
			}
		}
	}

	return imports, aliases
}

func (g *GoParser) buildFunctionSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	paramsNode := node.ChildByFieldName("parameters")
	resultNode := node.ChildByFieldName("result")

	sig := "func"
	if nameNode != nil {
		sig += " " + nameNode.Content(content)
	}
	if paramsNode != nil {
		sig += paramsNode.Content(content)
	}
	if resultNode != nil {
		sig += " " + resultNode.Content(content)
	}

	return sig
}

func (g *GoParser) buildTypeSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	typeNode := node.ChildByFieldName("type")

	if nameNode == nil {
		return ""
	}

	sig := "type " + nameNode.Content(content)
	if typeNode != nil {
		switch typeNode.Type() {
		case "struct_type":
			sig += " struct"
		case "interface_type":
			sig += " interface"
		default:
			sig += " " + typeNode.Content(content)
		}
	}

	return sig
}

func (g *GoParser) extractCalls(bodyNode *sitter.Node, content []byte) []parser.CallSite {
	if bodyNode == nil {
		return nil
	}

	calls := make([]parser.CallSite, 0)
	g.collectCalls(bodyNode, content, &calls)
	return calls
}

func (g *GoParser) collectCalls(node *sitter.Node, content []byte, calls *[]parser.CallSite) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		callSite := g.extractCallSite(node, content)
		if callSite.Name != "" {
			*calls = append(*calls, callSite)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		g.collectCalls(node.Child(i), content, calls)
	}
}

func (g *GoParser) extractCallSite(callNode *sitter.Node, content []byte) parser.CallSite {
	fnNode := callNode.ChildByFieldName("function")
	name, qualifier := g.extractCallName(fnNode, content)
	callSite := parser.CallSite{
		Name:      name,
		Qualifier: qualifier,
		Raw:       "",
		Line:      int(callNode.StartPoint().Row) + 1,
		Arity:     g.countCallArguments(callNode.ChildByFieldName("arguments")),
	}
	if fnNode != nil {
		callSite.Raw = strings.TrimSpace(fnNode.Content(content))
	}
	if qualifier != "" {
		callSite.Receiver = qualifier
	}
	return callSite
}

func (g *GoParser) extractCallName(node *sitter.Node, content []byte) (name, qualifier string) {
	if node == nil {
		return "", ""
	}

	switch node.Type() {
	case "identifier":
		return node.Content(content), ""
	case "selector_expression":
		operandNode := node.ChildByFieldName("operand")
		fieldNode := node.ChildByFieldName("field")
		if fieldNode != nil {
			qualifierValue := ""
			if operandNode != nil {
				qualifierValue = strings.TrimSpace(operandNode.Content(content))
			}
			return fieldNode.Content(content), qualifierValue
		}
		qualifierValue, nameValue := splitQualifiedName(node.Content(content))
		return nameValue, qualifierValue
	case "parenthesized_expression":
		return g.extractCallName(node.ChildByFieldName("expression"), content)
	case "index_expression", "type_instantiation_expression":
		nameValue, qualifierValue := g.extractCallName(node.ChildByFieldName("operand"), content)
		return nameValue, qualifierValue
	}

	qualifierValue, nameValue := splitQualifiedName(node.Content(content))
	if nameValue != "" {
		return nameValue, qualifierValue
	}
	return strings.TrimSpace(node.Content(content)), ""
}

func (g *GoParser) countCallArguments(argsNode *sitter.Node) int {
	if argsNode == nil {
		return 0
	}

	count := 0
	for i := 0; i < int(argsNode.NamedChildCount()); i++ {
		child := argsNode.NamedChild(i)
		if child != nil {
			count++
		}
	}
	return count
}

func (g *GoParser) readImportSpec(spec *sitter.Node, content []byte) (importPath, alias string) {
	pathNode := spec.ChildByFieldName("path")
	if pathNode == nil {
		return "", ""
	}

	importPath = strings.TrimSpace(pathNode.Content(content))
	if len(importPath) >= 2 && strings.HasPrefix(importPath, `"`) && strings.HasSuffix(importPath, `"`) {
		importPath = importPath[1 : len(importPath)-1]
	}

	aliasNode := spec.ChildByFieldName("name")
	if aliasNode != nil {
		alias = strings.TrimSpace(aliasNode.Content(content))
	}
	if alias == "_" || alias == "." {
		alias = ""
	}
	if alias == "" {
		alias = defaultImportAlias(importPath)
	}
	return importPath, alias
}
