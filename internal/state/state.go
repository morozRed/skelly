package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/skelly-dev/skelly/internal/parser"
)

const (
	StateFile            = ".state.json"
	CurrentStateVersion  = "2"
	CurrentParserVersion = "tree-sitter-v1"
)

// FileState tracks the state of a single file
type FileState struct {
	Hash          string            `json:"hash"`
	Language      string            `json:"language,omitempty"`
	Symbols       []parser.Symbol   `json:"symbols,omitempty"`
	Imports       []string          `json:"imports,omitempty"`
	ImportAliases map[string]string `json:"import_aliases,omitempty"`
	Dependencies  []string          `json:"dependencies,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// State tracks the state of all files for incremental updates
type State struct {
	Version       string               `json:"version"`
	ParserVersion string               `json:"parser_version,omitempty"`
	UpdatedAt     time.Time            `json:"updated_at"`
	Files         map[string]FileState `json:"files"`
	OutputHashes  map[string]string    `json:"output_hashes,omitempty"`
}

// NewState creates a new empty state
func NewState() *State {
	return &State{
		Version:       CurrentStateVersion,
		ParserVersion: CurrentParserVersion,
		Files:         make(map[string]FileState),
		OutputHashes:  make(map[string]string),
	}
}

// Load reads state from the context state file.
func Load(contextDir string) (*State, error) {
	path := filepath.Join(contextDir, StateFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	migrateState(&state)

	return &state, nil
}

// Save writes state to the context state file.
func (s *State) Save(contextDir string) error {
	if s.Version == "" {
		s.Version = CurrentStateVersion
	}
	if s.ParserVersion == "" {
		s.ParserVersion = CurrentParserVersion
	}
	if s.Files == nil {
		s.Files = make(map[string]FileState)
	}
	if s.OutputHashes == nil {
		s.OutputHashes = make(map[string]string)
	}

	s.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(contextDir, StateFile)
	return os.WriteFile(path, data, 0644)
}

// SetFileHash updates the hash for a file
func (s *State) SetFileHash(file, hash string) {
	s.Files[file] = FileState{
		Hash:      hash,
		UpdatedAt: time.Now(),
	}
}

// SetFileData stores parsed file metadata for incremental updates.
func (s *State) SetFileData(file parser.FileSymbols) {
	s.Files[file.Path] = FileState{
		Hash:          file.Hash,
		Language:      file.Language,
		Symbols:       file.Symbols,
		Imports:       file.Imports,
		ImportAliases: file.ImportAliases,
		UpdatedAt:     time.Now(),
	}
}

// GetFileHash returns the stored hash for a file
func (s *State) GetFileHash(file string) (string, bool) {
	fs, ok := s.Files[file]
	if !ok {
		return "", false
	}
	return fs.Hash, true
}

// HasChanged returns true if the file hash differs from stored
func (s *State) HasChanged(file, currentHash string) bool {
	storedHash, ok := s.GetFileHash(file)
	if !ok {
		return true // New file
	}
	return storedHash != currentHash
}

// RemoveFile removes a file from state tracking
func (s *State) RemoveFile(file string) {
	delete(s.Files, file)
}

// ChangedFiles returns files that have changed based on provided hashes
func (s *State) ChangedFiles(currentHashes map[string]string) []string {
	changed := make([]string, 0)

	// Check for new or modified files
	for file, hash := range currentHashes {
		if s.HasChanged(file, hash) {
			changed = append(changed, file)
		}
	}

	return changed
}

// DeletedFiles returns files that no longer exist
func (s *State) DeletedFiles(currentFiles map[string]bool) []string {
	deleted := make([]string, 0)

	for file := range s.Files {
		if !currentFiles[file] {
			deleted = append(deleted, file)
		}
	}

	return deleted
}

// ImpactedFiles returns changed/deleted files plus reverse dependency closure.
func (s *State) ImpactedFiles(changedFiles, deletedFiles []string) []string {
	reverse := make(map[string][]string)
	for file, fileState := range s.Files {
		for _, dep := range fileState.Dependencies {
			reverse[dep] = append(reverse[dep], file)
		}
	}

	impacted := make(map[string]bool)
	queue := make([]string, 0, len(changedFiles)+len(deletedFiles))
	for _, file := range changedFiles {
		if !impacted[file] {
			impacted[file] = true
			queue = append(queue, file)
		}
	}
	for _, file := range deletedFiles {
		if !impacted[file] {
			impacted[file] = true
			queue = append(queue, file)
		}
	}

	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		for _, depender := range reverse[file] {
			if impacted[depender] {
				continue
			}
			impacted[depender] = true
			queue = append(queue, depender)
		}
	}

	out := make([]string, 0, len(impacted))
	for file := range impacted {
		out = append(out, file)
	}
	sort.Strings(out)
	return out
}

// SetOutputHash records the content hash for a generated output file.
func (s *State) SetOutputHash(path, hash string) {
	if s.OutputHashes == nil {
		s.OutputHashes = make(map[string]string)
	}
	s.OutputHashes[path] = hash
}

// GetOutputHash returns the previously stored hash for a generated output file.
func (s *State) GetOutputHash(path string) (string, bool) {
	hash, ok := s.OutputHashes[path]
	return hash, ok
}

func migrateState(s *State) {
	if s.Files == nil {
		s.Files = make(map[string]FileState)
	}
	if s.OutputHashes == nil {
		s.OutputHashes = make(map[string]string)
	}
	if s.ParserVersion == "" {
		s.ParserVersion = CurrentParserVersion
	}

	switch s.Version {
	case "", "1":
		s.Version = CurrentStateVersion
	case CurrentStateVersion:
		// no-op
	default:
		// Keep unknown versions untouched but ensure required maps are initialized.
	}
}
