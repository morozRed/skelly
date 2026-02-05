package languages

import "github.com/skelly-dev/skelly/internal/parser"

// NewDefaultRegistry creates a registry with all supported language parsers
func NewDefaultRegistry() *parser.Registry {
	r := parser.NewRegistry()

	r.Register(NewGoParser())
	r.Register(NewPythonParser())
	r.Register(NewRubyParser())
	r.Register(NewTypeScriptParser())

	return r
}
