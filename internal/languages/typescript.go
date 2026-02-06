package languages

import (
	"context"
	"strings"

	"github.com/morozRed/skelly/internal/parser"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// TypeScriptParser implements parsing for TypeScript/JavaScript source files
type TypeScriptParser struct {
	tsParser *sitter.Parser
	jsParser *sitter.Parser
}

// NewTypeScriptParser creates a new TypeScript/JavaScript parser
func NewTypeScriptParser() *TypeScriptParser {
	ts := sitter.NewParser()
	ts.SetLanguage(typescript.GetLanguage())

	js := sitter.NewParser()
	js.SetLanguage(javascript.GetLanguage())

	return &TypeScriptParser{
		tsParser: ts,
		jsParser: js,
	}
}

func (t *TypeScriptParser) Language() string {
	return "typescript"
}

func (t *TypeScriptParser) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}
}

func (t *TypeScriptParser) Parse(filename string, content []byte) (*parser.FileSymbols, error) {
	// Choose parser based on extension
	var p *sitter.Parser
	lang := "typescript"
	if strings.HasSuffix(filename, ".js") || strings.HasSuffix(filename, ".jsx") ||
		strings.HasSuffix(filename, ".mjs") || strings.HasSuffix(filename, ".cjs") {
		p = t.jsParser
		lang = "javascript"
	} else {
		p = t.tsParser
	}

	tree, err := p.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	result := &parser.FileSymbols{
		Path:          filename,
		Language:      lang,
		Symbols:       make([]parser.Symbol, 0),
		Imports:       make([]string, 0),
		ImportAliases: make(map[string]string),
	}

	root := tree.RootNode()
	t.extractSymbols(root, content, result, "")

	return result, nil
}

func (t *TypeScriptParser) extractSymbols(node *sitter.Node, content []byte, result *parser.FileSymbols, className string) {
	switch node.Type() {
	case "function_declaration":
		sym := t.extractFunction(node, content)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}
		return

	case "method_definition":
		sym := t.extractMethod(node, content, className)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}
		return

	case "class_declaration":
		sym := t.extractClass(node, content)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
			// Recurse into class body
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				for i := 0; i < int(bodyNode.ChildCount()); i++ {
					t.extractSymbols(bodyNode.Child(i), content, result, sym.Name)
				}
			}
		}
		return

	case "interface_declaration":
		sym := t.extractInterface(node, content)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}
		return

	case "type_alias_declaration":
		sym := t.extractTypeAlias(node, content)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}
		return

	case "lexical_declaration", "variable_declaration":
		// Check for arrow functions or function expressions
		syms := t.extractVariableDeclarations(node, content)
		result.Symbols = append(result.Symbols, syms...)
		return

	case "export_statement":
		// Recurse into exported declarations
		for i := 0; i < int(node.ChildCount()); i++ {
			t.extractSymbols(node.Child(i), content, result, className)
		}
		return

	case "import_statement":
		imports, aliases := t.extractImports(node, content)
		result.Imports = append(result.Imports, imports...)
		result.ImportAliases = mergeImportAliases(result.ImportAliases, aliases)
	}

	// Recurse into children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		t.extractSymbols(child, content, result, className)
	}
}

func (t *TypeScriptParser) extractFunction(node *sitter.Node, content []byte) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	sig := t.buildFunctionSignature(node, content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolFunction,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
		Calls:     t.extractCalls(node.ChildByFieldName("body"), content),
	}
}

func (t *TypeScriptParser) extractMethod(node *sitter.Node, content []byte, className string) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	sig := t.buildMethodSignature(node, content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolMethod,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
		Calls:     t.extractCalls(node.ChildByFieldName("body"), content),
	}
}

func (t *TypeScriptParser) extractClass(node *sitter.Node, content []byte) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	sig := t.buildClassSignature(node, content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolClass,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
	}
}

func (t *TypeScriptParser) extractInterface(node *sitter.Node, content []byte) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolInterface,
		Signature: "interface " + name,
		Line:      int(node.StartPoint().Row) + 1,
	}
}

func (t *TypeScriptParser) extractTypeAlias(node *sitter.Node, content []byte) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolStruct, // Using struct for type aliases
		Signature: "type " + name,
		Line:      int(node.StartPoint().Row) + 1,
	}
}

func (t *TypeScriptParser) extractVariableDeclarations(node *sitter.Node, content []byte) []parser.Symbol {
	symbols := make([]parser.Symbol, 0)

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")

			if nameNode == nil || valueNode == nil {
				continue
			}

			// Check if it's a function
			if valueNode.Type() == "arrow_function" || valueNode.Type() == "function" {
				name := nameNode.Content(content)
				sig := t.buildArrowFunctionSignature(nameNode, valueNode, content)
				symbols = append(symbols, parser.Symbol{
					Name:      name,
					Kind:      parser.SymbolFunction,
					Signature: sig,
					Line:      int(child.StartPoint().Row) + 1,
					Calls:     t.extractCalls(valueNode, content),
				})
			}
		}
	}

	return symbols
}

func (t *TypeScriptParser) extractImports(node *sitter.Node, content []byte) ([]string, map[string]string) {
	imports := make([]string, 0)
	aliases := make(map[string]string)

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "string" {
			imp := child.Content(content)
			// Remove quotes
			if len(imp) >= 2 {
				imp = imp[1 : len(imp)-1]
			}
			imports = append(imports, imp)

			for _, alias := range parseJSImportAliases(node.Content(content)) {
				aliases[alias] = imp
			}
			defaultAlias := defaultImportAlias(imp)
			if defaultAlias != "" {
				if _, ok := aliases[defaultAlias]; !ok {
					aliases[defaultAlias] = imp
				}
			}
		}
	}

	return imports, aliases
}

func (t *TypeScriptParser) buildFunctionSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	paramsNode := node.ChildByFieldName("parameters")
	returnNode := node.ChildByFieldName("return_type")

	sig := "function"
	if nameNode != nil {
		sig += " " + nameNode.Content(content)
	}
	if paramsNode != nil {
		sig += paramsNode.Content(content)
	}
	if returnNode != nil {
		sig += formatTypeScriptReturnType(returnNode.Content(content))
	}

	return sig
}

func (t *TypeScriptParser) buildMethodSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	paramsNode := node.ChildByFieldName("parameters")
	returnNode := node.ChildByFieldName("return_type")

	sig := ""
	if nameNode != nil {
		sig = nameNode.Content(content)
	}
	if paramsNode != nil {
		sig += paramsNode.Content(content)
	}
	if returnNode != nil {
		sig += formatTypeScriptReturnType(returnNode.Content(content))
	}

	return sig
}

func (t *TypeScriptParser) buildClassSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")

	sig := "class"
	if nameNode != nil {
		sig += " " + nameNode.Content(content)
	}

	// Look for extends/implements
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "class_heritage" {
			sig += " " + child.Content(content)
			break
		}
	}

	return sig
}

func (t *TypeScriptParser) buildArrowFunctionSignature(nameNode, valueNode *sitter.Node, content []byte) string {
	name := nameNode.Content(content)
	paramsNode := valueNode.ChildByFieldName("parameters")
	returnNode := valueNode.ChildByFieldName("return_type")

	sig := "const " + name + " = "
	if paramsNode != nil {
		sig += paramsNode.Content(content)
	}
	sig += " =>"
	if returnNode != nil {
		sig += " " + returnNode.Content(content)
	}

	return sig
}

func (t *TypeScriptParser) extractCalls(node *sitter.Node, content []byte) []parser.CallSite {
	if node == nil {
		return nil
	}

	calls := make([]parser.CallSite, 0)
	t.collectCalls(node, content, &calls)
	return calls
}

func (t *TypeScriptParser) collectCalls(node *sitter.Node, content []byte, calls *[]parser.CallSite) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		callSite := t.extractCallSite(node, content)
		if callSite.Name != "" {
			*calls = append(*calls, callSite)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		t.collectCalls(node.Child(i), content, calls)
	}
}

func (t *TypeScriptParser) extractCallSite(callNode *sitter.Node, content []byte) parser.CallSite {
	fnNode := callNode.ChildByFieldName("function")
	name, qualifier := t.extractCallName(fnNode, content)
	callSite := parser.CallSite{
		Name:      name,
		Qualifier: qualifier,
		Raw:       "",
		Line:      int(callNode.StartPoint().Row) + 1,
		Arity:     t.countCallArguments(callNode.ChildByFieldName("arguments")),
	}
	if fnNode != nil {
		callSite.Raw = strings.TrimSpace(fnNode.Content(content))
	}
	if qualifier == "this" {
		callSite.Receiver = qualifier
	}
	return callSite
}

func (t *TypeScriptParser) extractCallName(node *sitter.Node, content []byte) (name, qualifier string) {
	if node == nil {
		return "", ""
	}

	switch node.Type() {
	case "identifier":
		return node.Content(content), ""
	case "member_expression":
		objectNode := node.ChildByFieldName("object")
		property := node.ChildByFieldName("property")
		if property != nil {
			qualifierValue := ""
			if objectNode != nil {
				qualifierValue = strings.TrimSpace(objectNode.Content(content))
			}
			return property.Content(content), qualifierValue
		}
	case "subscript_expression":
		return t.extractCallName(node.ChildByFieldName("object"), content)
	case "parenthesized_expression":
		return t.extractCallName(node.ChildByFieldName("expression"), content)
	}

	qualifierValue, nameValue := splitQualifiedName(node.Content(content))
	if nameValue != "" {
		return nameValue, qualifierValue
	}
	return strings.TrimSpace(node.Content(content)), ""
}

func (t *TypeScriptParser) countCallArguments(argsNode *sitter.Node) int {
	if argsNode == nil {
		return 0
	}
	return int(argsNode.NamedChildCount())
}

func parseJSImportAliases(raw string) []string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "import ") {
		return nil
	}

	fromIdx := strings.Index(raw, " from ")
	if fromIdx == -1 {
		return nil
	}
	spec := strings.TrimSpace(strings.TrimPrefix(raw[:fromIdx], "import "))
	if spec == "" {
		return nil
	}

	aliases := make([]string, 0)
	parts := splitTopLevelCSV(spec)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			members := splitTopLevelCSV(strings.Trim(part, "{} "))
			for _, member := range members {
				member = strings.TrimSpace(member)
				if member == "" {
					continue
				}
				member = strings.TrimSpace(strings.TrimPrefix(member, "type "))
				base, alias := splitAliasByAs(member)
				if alias != "" {
					member = alias
				} else {
					member = base
				}
				if member != "" {
					aliases = append(aliases, member)
				}
			}
			continue
		}

		part = strings.TrimSpace(strings.TrimPrefix(part, "type "))
		if strings.HasPrefix(part, "* as ") {
			alias := strings.TrimSpace(strings.TrimPrefix(part, "* as "))
			if alias != "" {
				aliases = append(aliases, alias)
			}
			continue
		}

		aliases = append(aliases, part)
	}
	return aliases
}

func formatTypeScriptReturnType(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.TrimSpace(strings.TrimPrefix(value, ":"))
	if value == "" {
		return ""
	}
	return ": " + value
}

func splitTopLevelCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := make([]string, 0)
	depth := 0
	start := 0
	for i, ch := range raw {
		switch ch {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(raw[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(raw[start:]))
	return parts
}
