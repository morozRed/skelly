package languages

import (
	"testing"

	"github.com/morozRed/skelly/internal/parser"
)

func TestParseJSImportAliasesHandlesNamedAliases(t *testing.T) {
	raw := `import core, { foo, bar as baz } from "./util"`
	aliases := parseJSImportAliases(raw)

	expected := []string{"core", "foo", "baz"}
	if len(aliases) != len(expected) {
		t.Fatalf("expected %d aliases, got %d (%#v)", len(expected), len(aliases), aliases)
	}
	for i := range expected {
		if aliases[i] != expected[i] {
			t.Fatalf("expected alias[%d]=%q, got %q", i, expected[i], aliases[i])
		}
	}
}

func TestTypeScriptSignaturesNormalizeReturnType(t *testing.T) {
	parser := NewTypeScriptParser()
	file, err := parser.Parse("main.ts", []byte(`function f(a:number): string { return ""; }
class Box {
  value(): Promise<string> { return Promise.resolve(""); }
}
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	functionSig := findSignatureByName(file.Symbols, "f")
	if functionSig != "function f(a:number): string" {
		t.Fatalf("unexpected function signature: %q", functionSig)
	}

	methodSig := findSignatureByName(file.Symbols, "value")
	if methodSig != "value(): Promise<string>" {
		t.Fatalf("unexpected method signature: %q", methodSig)
	}
}

func findSignatureByName(symbols []parser.Symbol, name string) string {
	for _, sym := range symbols {
		if sym.Name == name {
			return sym.Signature
		}
	}
	return ""
}
