package parser

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type mockParser struct {
	lang string
	exts []string
}

func (m mockParser) Language() string {
	return m.lang
}

func (m mockParser) Extensions() []string {
	return m.exts
}

func (m mockParser) Parse(filename string, content []byte) (*FileSymbols, error) {
	return &FileSymbols{
		Path:     filename,
		Language: m.lang,
		Symbols: []Symbol{
			{
				Name:      "mock",
				Kind:      SymbolFunction,
				Signature: "func mock()",
				Line:      1,
			},
		},
	}, nil
}

func TestRegistryGetParserForFile(t *testing.T) {
	r := NewRegistry()
	r.Register(mockParser{lang: "mock", exts: []string{".mock"}})

	p, ok := r.GetParserForFile("demo.MOCK")
	if !ok {
		t.Fatalf("expected parser for .MOCK extension")
	}
	if p.Language() != "mock" {
		t.Fatalf("expected language mock, got %s", p.Language())
	}
}

func TestParseDirectoryRespectsIgnoreRules(t *testing.T) {
	root := t.TempDir()
	r := NewRegistry()
	r.Register(mockParser{lang: "mock", exts: []string{".mock"}})

	mustWriteFile(t, filepath.Join(root, "keep.mock"), "ok")
	mustWriteFile(t, filepath.Join(root, "skip", "ignored.mock"), "x")
	mustWriteFile(t, filepath.Join(root, "skip", "include.mock"), "y")
	mustWriteFile(t, filepath.Join(root, ".skelly", ".context", "hidden.mock"), "z")

	result, err := r.ParseDirectory(root, []string{
		"skip/*",
		"!skip/include.mock",
	})
	if err != nil {
		t.Fatalf("ParseDirectory failed: %v", err)
	}

	got := make([]string, 0, len(result.Files))
	for _, file := range result.Files {
		got = append(got, file.Path)
	}
	sort.Strings(got)

	want := []string{"keep.mock", "skip/include.mock"}
	if len(got) != len(want) {
		t.Fatalf("expected %d parsed files, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
