package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/morozRed/skelly/internal/ignore"
)

// LanguageParser defines the interface each language must implement
type LanguageParser interface {
	// Language returns the language name (e.g., "go", "python")
	Language() string

	// Extensions returns file extensions this parser handles
	Extensions() []string

	// Parse extracts symbols from source code
	Parse(filename string, content []byte) (*FileSymbols, error)
}

// Registry holds all registered language parsers
type Registry struct {
	parsers   map[string]LanguageParser // language name -> parser
	extToLang map[string]string         // extension -> language name
}

// NewRegistry creates a new parser registry
func NewRegistry() *Registry {
	return &Registry{
		parsers:   make(map[string]LanguageParser),
		extToLang: make(map[string]string),
	}
}

// Register adds a language parser to the registry
func (r *Registry) Register(p LanguageParser) {
	lang := p.Language()
	r.parsers[lang] = p
	for _, ext := range p.Extensions() {
		r.extToLang[ext] = lang
	}
}

// GetParserForFile returns the appropriate parser for a file
func (r *Registry) GetParserForFile(filename string) (LanguageParser, bool) {
	ext := strings.ToLower(filepath.Ext(filename))
	lang, ok := r.extToLang[ext]
	if !ok {
		return nil, false
	}
	parser, ok := r.parsers[lang]
	return parser, ok
}

// SupportedExtensions returns all supported file extensions
func (r *Registry) SupportedExtensions() []string {
	exts := make([]string, 0, len(r.extToLang))
	for ext := range r.extToLang {
		exts = append(exts, ext)
	}
	return exts
}

// ParseFile parses a single file and returns its symbols
func (r *Registry) ParseFile(path string) (*FileSymbols, error) {
	parser, ok := r.GetParserForFile(path)
	if !ok {
		return nil, nil // unsupported file type, skip silently
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	symbols, err := parser.Parse(path, content)
	if err != nil {
		return nil, err
	}

	symbols.Imports = normalizeStrings(symbols.Imports)
	symbols.ImportAliases = normalizeImportAliases(symbols.ImportAliases)
	for i := range symbols.Symbols {
		symbols.Symbols[i].Calls = normalizeCallSites(symbols.Symbols[i].Calls)
	}

	// Compute file hash for incremental updates
	symbols.Hash = hashContent(content)

	return symbols, nil
}

// ParseDirectory recursively parses all supported files in a directory
func (r *Registry) ParseDirectory(root string, ignorePaths []string) (*ParseResult, error) {
	ignoreMatcher := ignore.NewMatcher(ignorePaths)

	result := &ParseResult{
		RootPath: root,
		Files:    make([]FileSymbols, 0),
		Issues:   make([]ParseIssue, 0),
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			relPath := path
			if rel, relErr := filepath.Rel(root, path); relErr == nil {
				relPath = rel
			}
			result.Issues = append(result.Issues, ParseIssue{
				File:     relPath,
				Severity: "warning",
				Message:  fmt.Sprintf("walk error: %v", err),
			})
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories and ignored paths
		relPath, _ := filepath.Rel(root, path)
		if ignoreMatcher.ShouldIgnore(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		symbols, err := r.ParseFile(path)
		if err != nil {
			lang := ""
			if langParser, ok := r.GetParserForFile(path); ok {
				lang = langParser.Language()
			}
			result.Issues = append(result.Issues, ParseIssue{
				File:     relPath,
				Language: lang,
				Severity: "error",
				Message:  err.Error(),
			})
			return nil
		}
		if symbols != nil {
			symbols.Path = relPath
			for i := range symbols.Symbols {
				symbols.Symbols[i].ID = StableSymbolID(relPath, symbols.Symbols[i])
			}
			result.Files = append(result.Files, *symbols)
		}

		return nil
	})

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Path < result.Files[j].Path
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		if result.Issues[i].File == result.Issues[j].File {
			return result.Issues[i].Message < result.Issues[j].Message
		}
		return result.Issues[i].File < result.Issues[j].File
	})

	return result, err
}

func hashContent(content []byte) string {
	h := sha256.New()
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))[:16] // short hash
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeCallSites(values []CallSite) []CallSite {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(values))
	out := make([]CallSite, 0, len(values))
	for _, value := range values {
		value.Name = strings.TrimSpace(value.Name)
		value.Qualifier = strings.TrimSpace(value.Qualifier)
		value.Receiver = strings.TrimSpace(value.Receiver)
		value.Raw = strings.TrimSpace(value.Raw)
		if value.Name == "" {
			continue
		}

		key := strings.Join([]string{
			value.Name,
			value.Qualifier,
			value.Receiver,
			fmt.Sprintf("%d", value.Arity),
			fmt.Sprintf("%d", value.Line),
		}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		if out[i].Qualifier != out[j].Qualifier {
			return out[i].Qualifier < out[j].Qualifier
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Receiver != out[j].Receiver {
			return out[i].Receiver < out[j].Receiver
		}
		return out[i].Raw < out[j].Raw
	})

	return out
}

func normalizeImportAliases(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	out := make(map[string]string, len(values))
	for alias, target := range values {
		alias = strings.TrimSpace(alias)
		target = strings.TrimSpace(target)
		if alias == "" || target == "" {
			continue
		}
		out[alias] = target
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
