package languages

import "testing"

func TestPythonFromImportCapturesAliasedMembers(t *testing.T) {
	parser := NewPythonParser()
	file, err := parser.Parse("main.py", []byte(`from util import foo as myfoo, bar

def run():
    myfoo()
    bar()
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(file.Imports) != 1 || file.Imports[0] != "util" {
		t.Fatalf("expected import list to include util, got %#v", file.Imports)
	}
	if got := file.ImportAliases["util"]; got != "util" {
		t.Fatalf("expected module alias util=>util, got %q", got)
	}
	if got := file.ImportAliases["myfoo"]; got != "util#foo" {
		t.Fatalf("expected aliased import myfoo=>util#foo, got %q", got)
	}
	if got := file.ImportAliases["bar"]; got != "util#bar" {
		t.Fatalf("expected named import bar=>util#bar, got %q", got)
	}
	if _, exists := file.ImportAliases["foo"]; exists {
		t.Fatalf("did not expect original name foo for aliased import")
	}
}
