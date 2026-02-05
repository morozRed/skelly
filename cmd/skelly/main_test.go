package main

import (
	"strings"
	"testing"

	"github.com/skelly-dev/skelly/internal/output"
)

func TestBuildSkellyHookBlockPreservesFormat(t *testing.T) {
	block := buildSkellyHookBlock("/repo/path")

	if !strings.Contains(block, "context_dir=\"$repo_root/"+output.ContextDir+"\"") {
		t.Fatalf("expected hook block to include context dir detection, got:\n%s", block)
	}
	for _, expected := range []string{
		"$context_dir/manifest.json",
		"$context_dir/symbols.jsonl",
		"$context_dir/edges.jsonl",
		"skelly update --format jsonl",
		"skelly update) || exit 1",
	} {
		if !strings.Contains(block, expected) {
			t.Fatalf("expected hook block to contain %q, got:\n%s", expected, block)
		}
	}
}

func TestUpsertSkellyHookReplacesExistingBlock(t *testing.T) {
	existing := "#!/bin/sh\n\necho before\n" + hookStart + "\nold block\n" + hookEnd + "\n\necho after\n"
	updated := upsertSkellyHook(existing, "/repo/path")

	if strings.Contains(updated, "old block") {
		t.Fatalf("expected old hook block to be replaced, got:\n%s", updated)
	}
	if strings.Count(updated, hookStart) != 1 || strings.Count(updated, hookEnd) != 1 {
		t.Fatalf("expected exactly one hook block after update, got:\n%s", updated)
	}
	if !strings.Contains(updated, "echo before") || !strings.Contains(updated, "echo after") {
		t.Fatalf("expected non-skelly hook content to be preserved, got:\n%s", updated)
	}
}
