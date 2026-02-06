package languages

import "github.com/morozRed/skelly/internal/parser"

// NewDefaultRegistry creates a registry with all supported language parsers
func NewDefaultRegistry() *parser.Registry {
	r := parser.NewRegistry()

	r.Register(NewGoParser())
	r.Register(NewPythonParser())
	r.Register(NewRubyParser())
	r.Register(NewTypeScriptParser())

	return r
}
