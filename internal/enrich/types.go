package enrich

import (
	"github.com/morozRed/skelly/internal/graph"
	"github.com/morozRed/skelly/internal/parser"
	"github.com/morozRed/skelly/internal/state"
)

type Scope string

const (
	ScopeTarget Scope = "target"
	OutputFile        = "enrich.jsonl"
)

type Record struct {
	SymbolID      string       `json:"symbol_id"`
	Agent         string       `json:"agent"`
	AgentProfile  string       `json:"agent_profile,omitempty"`
	Model         string       `json:"model,omitempty"`
	PromptVersion string       `json:"prompt_version,omitempty"`
	CacheKey      string       `json:"cache_key,omitempty"`
	Scope         string       `json:"scope"`
	FileHash      string       `json:"file_hash"`
	Input         InputPayload `json:"input"`
	Output        Output       `json:"output,omitempty"`
	Status        string       `json:"status,omitempty"`
	Error         string       `json:"error,omitempty"`
	GeneratedAt   string       `json:"generated_at,omitempty"`
	UpdatedAt     string       `json:"updated_at,omitempty"`
}

type InputPayload struct {
	Symbol   SymbolMetadata `json:"symbol"`
	Source   SourceSpan     `json:"source"`
	Imports  []string       `json:"imports,omitempty"`
	Calls    []string       `json:"calls,omitempty"`
	CalledBy []string       `json:"called_by,omitempty"`
}

type SymbolMetadata struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Path      string `json:"path"`
	Language  string `json:"language"`
	Line      int    `json:"line"`
}

type SourceSpan struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Body      string `json:"body,omitempty"`
}

type Output struct {
	Summary     string `json:"summary"`
	Purpose     string `json:"purpose"`
	SideEffects string `json:"side_effects"`
	Confidence  string `json:"confidence"`
}

type WorkItem struct {
	File      string
	FileState state.FileState
	Symbol    parser.Symbol
	Node      *graph.Node
}
