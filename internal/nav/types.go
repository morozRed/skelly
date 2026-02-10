package nav

type Index struct {
	Version string      `json:"version"`
	Nodes   []IndexNode `json:"nodes"`
}

type IndexNode struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Kind          string           `json:"kind"`
	Signature     string           `json:"signature,omitempty"`
	File          string           `json:"file"`
	Line          int              `json:"line"`
	OutEdges      []string         `json:"out_edges,omitempty"`
	InEdges       []string         `json:"in_edges,omitempty"`
	OutConfidence []EdgeConfidence `json:"out_confidence,omitempty"`
}

type EdgeConfidence struct {
	TargetID   string `json:"target_id"`
	Confidence string `json:"confidence,omitempty"`
}

type Lookup struct {
	ByID   map[string]*IndexNode
	ByName map[string][]string
}

type ResolveOptions struct {
	Fuzzy bool
	Limit int
}

type SymbolRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature,omitempty"`
	File      string `json:"file"`
	Line      int    `json:"line"`
}

type EdgeRecord struct {
	Symbol     SymbolRecord `json:"symbol"`
	Confidence string       `json:"confidence,omitempty"`
}

type TraceHop struct {
	Depth      int          `json:"depth"`
	From       SymbolRecord `json:"from"`
	To         SymbolRecord `json:"to"`
	Confidence string       `json:"confidence,omitempty"`
}
