package languages

import (
	"context"
	"strings"

	"github.com/morozRed/skelly/internal/parser"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/ruby"
)

// RubyParser implements parsing for Ruby source files
type RubyParser struct {
	parser *sitter.Parser
}

// NewRubyParser creates a new Ruby parser
func NewRubyParser() *RubyParser {
	p := sitter.NewParser()
	p.SetLanguage(ruby.GetLanguage())
	return &RubyParser{parser: p}
}

func (r *RubyParser) Language() string {
	return "ruby"
}

func (r *RubyParser) Extensions() []string {
	return []string{".rb", ".rake", ".gemspec"}
}

func (r *RubyParser) Parse(filename string, content []byte) (*parser.FileSymbols, error) {
	tree, err := r.parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	result := &parser.FileSymbols{
		Path:          filename,
		Language:      "ruby",
		Symbols:       make([]parser.Symbol, 0),
		Imports:       make([]string, 0),
		ImportAliases: make(map[string]string),
	}

	root := tree.RootNode()
	r.extractSymbols(root, content, result, "", "")

	return result, nil
}

func (r *RubyParser) extractSymbols(node *sitter.Node, content []byte, result *parser.FileSymbols, modulePath string, className string) {
	switch node.Type() {
	case "method":
		sym := r.extractMethod(node, content, className)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}
		return

	case "singleton_method":
		sym := r.extractSingletonMethod(node, content, className)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
		}
		return

	case "class":
		sym := r.extractClass(node, content, modulePath)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
			// Recurse into class body
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				newClassName := sym.Name
				if modulePath != "" {
					newClassName = modulePath + "::" + sym.Name
				}
				for i := 0; i < int(bodyNode.ChildCount()); i++ {
					r.extractSymbols(bodyNode.Child(i), content, result, modulePath, newClassName)
				}
			}
		}
		return

	case "module":
		sym := r.extractModule(node, content, modulePath)
		if sym != nil {
			result.Symbols = append(result.Symbols, *sym)
			// Recurse into module body
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				newModulePath := sym.Name
				if modulePath != "" {
					newModulePath = modulePath + "::" + sym.Name
				}
				for i := 0; i < int(bodyNode.ChildCount()); i++ {
					r.extractSymbols(bodyNode.Child(i), content, result, newModulePath, "")
				}
			}
		}
		return

	case "call":
		// Check for require/require_relative
		methodNode := node.ChildByFieldName("method")
		if methodNode != nil {
			method := methodNode.Content(content)
			if method == "require" || method == "require_relative" {
				args := node.ChildByFieldName("arguments")
				if args != nil {
					for i := 0; i < int(args.ChildCount()); i++ {
						arg := args.Child(i)
						if arg.Type() == "string" {
							imp := extractRubyString(arg.Content(content))
							if imp != "" {
								result.Imports = append(result.Imports, imp)
								alias := defaultImportAlias(imp)
								if alias != "" {
									result.ImportAliases[alias] = imp
								}
							}
						}
					}
				}
			}
		}
	}

	// Recurse into children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		r.extractSymbols(child, content, result, modulePath, className)
	}
}

func (r *RubyParser) extractMethod(node *sitter.Node, content []byte, className string) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	kind := parser.SymbolFunction
	if className != "" {
		kind = parser.SymbolMethod
	}

	sig := r.buildMethodSignature(node, content)
	bodyNode := node.ChildByFieldName("body")

	return &parser.Symbol{
		Name:      name,
		Kind:      kind,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
		Calls:     r.extractCalls(bodyNode, content),
	}
}

func (r *RubyParser) extractSingletonMethod(node *sitter.Node, content []byte, className string) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	sig := "def self." + name + r.extractParams(node, content)
	bodyNode := node.ChildByFieldName("body")

	return &parser.Symbol{
		Name:      "self." + name,
		Kind:      parser.SymbolMethod,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
		Calls:     r.extractCalls(bodyNode, content),
	}
}

func (r *RubyParser) extractClass(node *sitter.Node, content []byte, modulePath string) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)
	sig := r.buildClassSignature(node, content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolClass,
		Signature: sig,
		Line:      int(node.StartPoint().Row) + 1,
	}
}

func (r *RubyParser) extractModule(node *sitter.Node, content []byte, modulePath string) *parser.Symbol {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nameNode.Content(content)

	return &parser.Symbol{
		Name:      name,
		Kind:      parser.SymbolModule,
		Signature: "module " + name,
		Line:      int(node.StartPoint().Row) + 1,
	}
}

func (r *RubyParser) buildMethodSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}

	return "def " + nameNode.Content(content) + r.extractParams(node, content)
}

func (r *RubyParser) extractParams(node *sitter.Node, content []byte) string {
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode == nil {
		return ""
	}
	return paramsNode.Content(content)
}

func (r *RubyParser) buildClassSignature(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	superclassNode := node.ChildByFieldName("superclass")

	sig := "class"
	if nameNode != nil {
		sig += " " + nameNode.Content(content)
	}
	if superclassNode != nil {
		sig += " < " + superclassNode.Content(content)
	}

	return sig
}

func extractRubyString(s string) string {
	s = strings.TrimSpace(s)
	// Handle single and double quoted strings
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		return s[1 : len(s)-1]
	}
	return s
}

func (r *RubyParser) extractCalls(bodyNode *sitter.Node, content []byte) []parser.CallSite {
	if bodyNode == nil {
		return nil
	}

	calls := make([]parser.CallSite, 0)
	r.collectCalls(bodyNode, content, &calls)
	return calls
}

func (r *RubyParser) collectCalls(node *sitter.Node, content []byte, calls *[]parser.CallSite) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "call", "command", "command_call":
		callSite := r.extractCallSite(node, content)
		if callSite.Name != "" {
			*calls = append(*calls, callSite)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		r.collectCalls(node.Child(i), content, calls)
	}
}

func (r *RubyParser) extractCallSite(node *sitter.Node, content []byte) parser.CallSite {
	methodNode := node.ChildByFieldName("method")
	if methodNode == nil {
		methodNode = node.ChildByFieldName("name")
	}

	name := ""
	if methodNode != nil {
		name = strings.TrimSpace(methodNode.Content(content))
	}
	receiverNode := node.ChildByFieldName("receiver")
	qualifier := ""
	if receiverNode != nil {
		qualifier = strings.TrimSpace(receiverNode.Content(content))
	}

	arity := 0
	argsNode := node.ChildByFieldName("arguments")
	if argsNode != nil {
		arity = int(argsNode.NamedChildCount())
	}

	callSite := parser.CallSite{
		Name:      name,
		Qualifier: qualifier,
		Raw:       strings.TrimSpace(node.Content(content)),
		Line:      int(node.StartPoint().Row) + 1,
		Arity:     arity,
	}
	if qualifier == "self" {
		callSite.Receiver = qualifier
	}
	return callSite
}
