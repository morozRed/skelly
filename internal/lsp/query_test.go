package lsp

import (
	"errors"
	"strings"
	"testing"
)

func TestParseLocationOutput(t *testing.T) {
	output := strings.Join([]string{
		"/repo/internal/cli/root.go:42:7",
		"/repo/internal/cli/root.go:42:7",
		"/repo/internal/nav/commands.go:11:3-extra",
		"garbage",
	}, "\n")

	locations := ParseLocationOutput("/repo", output)
	locations = DeduplicateLocations(locations)

	if len(locations) != 2 {
		t.Fatalf("expected 2 locations, got %#v", locations)
	}
	if locations[0].File != "internal/cli/root.go" || locations[0].Line != 42 {
		t.Fatalf("unexpected first location: %#v", locations[0])
	}
	if locations[1].File != "internal/nav/commands.go" || locations[1].Line != 11 {
		t.Fatalf("unexpected second location: %#v", locations[1])
	}
}

func TestQueryDefinitionWithRunner(t *testing.T) {
	runner := func(dir string, name string, args ...string) (string, error) {
		if dir != "/repo" || name != "gopls" {
			t.Fatalf("unexpected runner invocation dir=%q name=%q", dir, name)
		}
		if len(args) != 2 || args[0] != "definition" || !strings.Contains(args[1], "/repo/internal/cli/root.go:9:1") {
			t.Fatalf("unexpected args: %#v", args)
		}
		return "/repo/internal/nav/index.go:99:2\n", nil
	}

	locations, err := QueryDefinitionWithRunner("/repo", "internal/cli/root.go", 9, 1, "gopls", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("expected one definition location, got %#v", locations)
	}
	if locations[0].File != "internal/nav/index.go" || locations[0].Line != 99 {
		t.Fatalf("unexpected definition location: %#v", locations[0])
	}
}

func TestQueryReferencesUnsupportedServer(t *testing.T) {
	_, err := QueryReferencesWithRunner("/repo", "internal/cli/root.go", 9, 1, "pylsp", func(dir string, name string, args ...string) (string, error) {
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("expected unsupported server error, got %v", err)
	}
}

func TestQueryDefinitionRunnerError(t *testing.T) {
	_, err := QueryDefinitionWithRunner("/repo", "internal/cli/root.go", 9, 1, "gopls", func(dir string, name string, args ...string) (string, error) {
		return "", errors.New("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "lsp definition query failed") {
		t.Fatalf("expected wrapped query error, got %v", err)
	}
}
