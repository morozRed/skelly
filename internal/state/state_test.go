package state

import (
	"reflect"
	"testing"
)

func TestChangedAndDeletedFiles(t *testing.T) {
	s := NewState()
	s.SetFileHash("a.go", "a1")
	s.SetFileHash("b.go", "b1")
	s.SetFileHash("c.go", "c1")

	changed := s.ChangedFiles(map[string]string{
		"a.go": "a1",
		"b.go": "b2",
		"d.go": "d1",
	})
	expectSet(t, changed, []string{"b.go", "d.go"})

	deleted := s.DeletedFiles(map[string]bool{
		"a.go": true,
		"b.go": true,
		"d.go": true,
	})
	expectSet(t, deleted, []string{"c.go"})
}

func TestImpactedFilesClosure(t *testing.T) {
	s := NewState()
	s.Files["a.go"] = FileState{Dependencies: []string{"b.go"}}
	s.Files["c.go"] = FileState{Dependencies: []string{"a.go"}}
	s.Files["d.go"] = FileState{Dependencies: []string{"x.go"}}

	impacted := s.ImpactedFiles([]string{"b.go"}, nil)
	want := []string{"a.go", "b.go", "c.go"}
	if !reflect.DeepEqual(impacted, want) {
		t.Fatalf("expected impacted %v, got %v", want, impacted)
	}
}

func TestMigrateStateSetsOutputVersion(t *testing.T) {
	s := &State{
		Version:       "1",
		ParserVersion: "",
		OutputVersion: "",
		Files:         map[string]FileState{},
	}

	migrateState(s)

	if s.OutputVersion != CurrentOutputVersion {
		t.Fatalf("expected output version %q, got %q", CurrentOutputVersion, s.OutputVersion)
	}
	if s.ParserVersion != CurrentParserVersion {
		t.Fatalf("expected parser version %q, got %q", CurrentParserVersion, s.ParserVersion)
	}
}

func expectSet(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d (%v)", len(want), len(got), got)
	}

	index := make(map[string]bool, len(got))
	for _, item := range got {
		index[item] = true
	}

	for _, item := range want {
		if !index[item] {
			t.Fatalf("expected item %q in %v", item, got)
		}
	}
}
