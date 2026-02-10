package lsp

import (
	"errors"
	"testing"
)

func TestDetectLanguagePresence(t *testing.T) {
	presence := DetectLanguagePresence([]string{
		"internal/cli/root.go",
		"pkg/server/main.ts",
		"script/tool.py",
	})

	if !presence["go"] {
		t.Fatalf("expected go to be present")
	}
	if !presence["typescript"] {
		t.Fatalf("expected typescript to be present")
	}
	if !presence["python"] {
		t.Fatalf("expected python to be present")
	}
	if presence["ruby"] {
		t.Fatalf("expected ruby to be absent")
	}
}

func TestProbeCapabilitiesWithLookPath(t *testing.T) {
	presence := map[string]bool{
		"go":         true,
		"python":     true,
		"typescript": false,
		"ruby":       true,
	}

	capabilities := ProbeCapabilitiesWithLookPath(presence, func(file string) (string, error) {
		switch file {
		case "gopls", "pylsp":
			return "/mock/bin/" + file, nil
		default:
			return "", errors.New("not found")
		}
	})

	if !capabilities["go"].Available || capabilities["go"].Server != "gopls" {
		t.Fatalf("expected go capability to be available with gopls, got %#v", capabilities["go"])
	}
	if !capabilities["python"].Available || capabilities["python"].Server != "pylsp" {
		t.Fatalf("expected python fallback to pylsp, got %#v", capabilities["python"])
	}
	if capabilities["typescript"].Reason != "language_not_present" {
		t.Fatalf("expected typescript to be marked language_not_present, got %#v", capabilities["typescript"])
	}
	if capabilities["ruby"].Reason != "server_not_found" {
		t.Fatalf("expected ruby server_not_found, got %#v", capabilities["ruby"])
	}
}
