package languages

import (
	"context"
	"strings"

	"github.com/morozRed/skelly/internal/parser"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

// PythonParser implements parsing for Python source files
type PythonParser struct {
	parser *sitter.Parser
}

// NewPythonParser creates a new Python parser
func NewPythonParser() *PythonParser {
	p := sitter.NewParser()
	p.SetLanguage(python.GetLanguage())
	return &PythonParser{parser: p}
}

func (p *PythonParser) Language() string {
	return "python"
}

func (p *PythonParser) Extensions() []string {
	return []string{".py", ".pyw"}
}

func (p *PythonParser) Parse(filename string, content []byte) (*parser.FileSymbols, error) {
	tree, err := p.parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	result := &parser.FileSymbols{
		Path:          filename,
		Language:      "python",
		Symbols:       make([]parser.Symbol, 0),
		Imports:       make([]string, 0),
		ImportAliases: make(map[string]string),
	}

	root := tree.RootNode()
	p.extractSymbols(root, content, result, "")

	return result, nil
}

func (p *PythonParser) extractSymbols(node *sitter.Node, content []byte, result *parser.FileSymbols, className string) {
	switch node.Type() {
	case "function_definition":
		sym := p.extractFunction(node, content, className)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}
		// Don't recurse into function body for nested functions (for now)
		return

	case "class_definition":
		sym := p.extractClass(node, content)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
			// Recurse into class body to get methods
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				for i := 0; i < int(bodyNode.ChildCount()); i++ {
					p.extractSymbols(bodyNode.Child(i), content, result, sym.Name)
				}
			}
		}
		return

	case "import_statement":
		imports, aliases := p.extractImport(node, content)
		result.Imports = append(result.Imports, imports...)
		result.ImportAliases = mergeImportAliases(result.ImportAliases, aliases)

	case "import_from_statement":
		imports, aliases := p.extractFromImport(node, content)
		result.Imports = append(result.Imports, imports...)
		result.ImportAliases = mergeImportAliases(result.ImportAliases, aliases)
	}

	// Recurse into children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		p.extractSymbols(child, content, result, className)
	}
}

func (p *PythonParser) extractFunction(node *sitter.Node, content []byte, className string) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	kind := parser.SymbolFunction
	if className != "" {
		kind = parser.SymbolMethod
	}

	sig := p.buildFunctionSignature(node, content)

	// Get docstring if present
	doc := ""
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil && bodyNode.ChildCount() > 0 {
		firstStmt := bodyNode.Child(0)
		if firstStmt.Type() == "expression_statement" && firstStmt.ChildCount() > 0 {
			expr := firstStmt.Child(0)
			if expr.Type() == "string" {
				doc = extractDocstring(expr.Content(content))
			}
		}
	}

	return &parser.Symbol{
		Name:      name,
		Kind:      kind,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
		Doc:       doc,
		Calls:     p.extractCalls(bodyNode, content),
	}
}

func (p *PythonParser) extractClass(node *sitter.Node, content []byte) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	sig := p.buildClassSignature(node, content)

	// Get docstring if present
	doc := ""
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil && bodyNode.ChildCount() > 0 {
		firstStmt := bodyNode.Child(0)
		if firstStmt.Type() == "expression_statement" && firstStmt.ChildCount() > 0 {
			expr := firstStmt.Child(0)
			if expr.Type() == "string" {
				doc = extractDocstring(expr.Content(content))
			}
		}
	}

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolClass,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
		Doc:       doc,
	}
}

func (p *PythonParser) extractImport(node *sitter.Node, content []byte) ([]string, map[string]string) {
	imports := make([]string, 0)
	aliases := make(map[string]string)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "dotted_name":
			module := strings.TrimSpace(child.Content(content))
			if module != "" {
				imports = append(imports, module)
				aliases[defaultImportAlias(module)] = module
			}
		case "aliased_import":
			module, alias := parsePythonAliasedImport(child.Content(content))
			if module != "" {
				imports = append(imports, module)
			}
			if alias != "" && module != "" {
				aliases[alias] = module
			}
		}
	}
	return imports, aliases
}

func (p *PythonParser) extractFromImport(node *sitter.Node, content []byte) ([]string, map[string]string) {
	aliases := make(map[string]string)
	moduleNode := node.ChildByFieldName("module_name")
	if moduleNode == nil {
		return nil, aliases
	}
	moduleName := strings.TrimSpace(moduleNode.Content(content))
	if moduleName == "" {
		return nil, aliases
	}

	alias := defaultImportAlias(moduleName)
	if alias != "" {
		aliases[alias] = moduleName
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		if node.FieldNameForChild(i) != "name" {
			continue
		}
		child := node.Child(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "aliased_import":
			importedName := ""
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				importedName = strings.TrimSpace(nameNode.Content(content))
			}
			aliasName := ""
			if aliasNode := child.ChildByFieldName("alias"); aliasNode != nil {
				aliasName = strings.TrimSpace(aliasNode.Content(content))
			}
			if aliasName != "" && importedName != "" {
				aliases[aliasName] = fromImportAliasTarget(moduleName, importedName)
			}
		case "dotted_name", "identifier":
			importedName := strings.TrimSpace(child.Content(content))
			if importedName != "" {
				aliases[importedName] = fromImportAliasTarget(moduleName, importedName)
			}
		}
	}

	return []string{moduleName}, aliases
}

func (p *PythonParser) buildFunctionSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	paramsNode := node.ChildByFieldName("parameters")
	returnNode := node.ChildByFieldName("return_type")

	sig := "def"
	if nameNode != nil {
		sig += " " + nameNode.Content(content)
	}
	if paramsNode != nil {
		sig += paramsNode.Content(content)
	}
	if returnNode != nil {
		sig += " -> " + returnNode.Content(content)
	}

	return sig
}

func (p *PythonParser) buildClassSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	superclassNode := node.ChildByFieldName("superclasses")

	sig := "class"
	if nameNode != nil {
		sig += " " + nameNode.Content(content)
	}
	if superclassNode != nil {
		sig += superclassNode.Content(content)
	}

	return sig
}

func (p *PythonParser) extractCalls(bodyNode *sitter.Node, content []byte) []parser.CallSite {
	if bodyNode == nil {
		return nil
	}

	calls := make([]parser.CallSite, 0)
	p.collectCalls(bodyNode, content, &calls)
	return dedupeCallSites(calls)
}

func (p *PythonParser) collectCalls(node *sitter.Node, content []byte, calls *[]parser.CallSite) {
	if node == nil {
		return
	}

	if node.Type() == "call" {
		callSite := p.extractCallSite(node, content)
		if callSite.Name != "" {
			*calls = append(*calls, callSite)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		p.collectCalls(node.Child(i), content, calls)
	}
}

func (p *PythonParser) extractCallSite(callNode *sitter.Node, content []byte) parser.CallSite {
	fnNode := callNode.ChildByFieldName("function")
	name, qualifier := p.extractCallName(fnNode, content)
	callSite := parser.CallSite{
		Name:      name,
		Qualifier: qualifier,
		Raw:       "",
		Line:      int(callNode.StartPoint().Row) + 1,
		Arity:     p.countCallArguments(callNode.ChildByFieldName("arguments")),
	}
	if fnNode != nil {
		callSite.Raw = strings.TrimSpace(fnNode.Content(content))
	}
	if qualifier == "self" || qualifier == "cls" {
		callSite.Receiver = qualifier
	}
	return callSite
}

func (p *PythonParser) extractCallName(node *sitter.Node, content []byte) (name, qualifier string) {
	if node == nil {
		return "", ""
	}

	switch node.Type() {
	case "identifier":
		return node.Content(content), ""
	case "attribute":
		object := node.ChildByFieldName("object")
		attr := node.ChildByFieldName("attribute")
		if attr != nil {
			qualifierValue := ""
			if object != nil {
				qualifierValue = strings.TrimSpace(object.Content(content))
			}
			return attr.Content(content), qualifierValue
		}
		qualifierValue, nameValue := splitQualifiedName(node.Content(content))
		return nameValue, qualifierValue
	case "parenthesized_expression":
		return p.extractCallName(node.ChildByFieldName("expression"), content)
	case "subscript":
		return p.extractCallName(node.ChildByFieldName("value"), content)
	}

	qualifierValue, nameValue := splitQualifiedName(node.Content(content))
	if nameValue != "" {
		return nameValue, qualifierValue
	}
	return strings.TrimSpace(node.Content(content)), ""
}

func (p *PythonParser) countCallArguments(argsNode *sitter.Node) int {
	if argsNode == nil {
		return 0
	}
	return int(argsNode.NamedChildCount())
}

func parsePythonAliasedImport(raw string) (module, alias string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	module, alias = splitAliasByAs(raw)
	if alias == "" {
		alias = defaultImportAlias(module)
	}
	return module, alias
}

func fromImportAliasTarget(moduleName, symbolName string) string {
	moduleName = strings.TrimSpace(moduleName)
	symbolName = strings.TrimSpace(symbolName)
	if moduleName == "" {
		return ""
	}
	if symbolName == "" {
		return moduleName
	}
	return moduleName + "#" + symbolName
}

func extractDocstring(s string) string {
	// Remove triple quotes and clean up
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"""`) && strings.HasSuffix(s, `"""`) {
		s = s[3 : len(s)-3]
	} else if strings.HasPrefix(s, `'''`) && strings.HasSuffix(s, `'''`) {
		s = s[3 : len(s)-3]
	}
	// Take first line only for brevity
	if idx := strings.Index(s, "\n"); idx != -1 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}
