package parser

import (
	"encoding/json"
	"strings"
)

// SymbolKind represents the type of code symbol
type SymbolKind int

const (
	SymbolFunction SymbolKind = iota
	SymbolMethod
	SymbolClass
	SymbolStruct
	SymbolInterface
	SymbolModule
	SymbolConstant
	SymbolVariable
)

func (k SymbolKind) String() string {
	switch k {
	case SymbolFunction:
		return "func"
	case SymbolMethod:
		return "method"
	case SymbolClass:
		return "class"
	case SymbolStruct:
		return "struct"
	case SymbolInterface:
		return "interface"
	case SymbolModule:
		return "module"
	case SymbolConstant:
		return "const"
	case SymbolVariable:
		return "var"
	default:
		return "unknown"
	}
}

// CallSite captures a function/method invocation discovered inside a symbol body.
type CallSite struct {
	Name      string `json:"name"`
	Qualifier string `json:"qualifier,omitempty"`
	Receiver  string `json:"receiver,omitempty"`
	Arity     int    `json:"arity,omitempty"`
	Line      int    `json:"line,omitempty"`
	Raw       string `json:"raw,omitempty"`
}

// Symbol represents a code symbol (function, class, etc.)
type Symbol struct {
	ID        string
	Name      string
	Kind      SymbolKind
	Signature string // e.g., "func(ctx context.Context, id string) (*User, error)"
	File      string // relative file path
	Line      int    // line number
	Doc       string // docstring/comment if available
	Calls     []CallSite
	CalledBy  []string // symbols that call this one
}

// UnmarshalJSON supports both legacy []string call payloads and the newer []CallSite shape.
func (s *Symbol) UnmarshalJSON(data []byte) error {
	type wireSymbol struct {
		ID        string
		Name      string
		Kind      SymbolKind
		Signature string
		File      string
		Line      int
		Doc       string
		Calls     json.RawMessage
		CalledBy  []string
	}

	var wire wireSymbol
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	s.ID = wire.ID
	s.Name = wire.Name
	s.Kind = wire.Kind
	s.Signature = wire.Signature
	s.File = wire.File
	s.Line = wire.Line
	s.Doc = wire.Doc
	s.CalledBy = wire.CalledBy

	rawCalls := strings.TrimSpace(string(wire.Calls))
	if rawCalls == "" || rawCalls == "null" {
		s.Calls = nil
		return nil
	}

	var typedCalls []CallSite
	if err := json.Unmarshal(wire.Calls, &typedCalls); err == nil {
		s.Calls = typedCalls
		return nil
	}

	var legacyCalls []string
	if err := json.Unmarshal(wire.Calls, &legacyCalls); err != nil {
		return err
	}
	s.Calls = make([]CallSite, 0, len(legacyCalls))
	for _, call := range legacyCalls {
		call = strings.TrimSpace(call)
		if call == "" {
			continue
		}
		s.Calls = append(s.Calls, CallSite{Name: call, Raw: call})
	}
	return nil
}

// FileSymbols holds all symbols extracted from a single file
type FileSymbols struct {
	Path          string
	Language      string
	Symbols       []Symbol
	Imports       []string          // imported modules/packages
	ImportAliases map[string]string // alias -> import target (module/package path, optionally module#symbol)
	Hash          string            // file content hash for incremental updates
}

// ParseIssue captures non-fatal parser warnings/errors encountered while scanning files.
type ParseIssue struct {
	File     string `json:"file"`
	Language string `json:"language,omitempty"`
	Severity string `json:"severity"` // warning | error
	Message  string `json:"message"`
}

// ParseResult holds the complete parse result for a codebase
type ParseResult struct {
	Files    []FileSymbols
	RootPath string
	Issues   []ParseIssue
}
