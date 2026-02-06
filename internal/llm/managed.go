package llm

import (
	"fmt"
	"os"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
)

const (
	ManagedBlockStart = "<!-- skelly:managed:start -->"
	ManagedBlockEnd   = "<!-- skelly:managed:end -->"
)

func UpsertManagedMarkdownFile(path, body string) (bool, error) {
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}

	managed := fmt.Sprintf("%s\n%s\n%s", ManagedBlockStart, strings.TrimSpace(body), ManagedBlockEnd)
	updated := UpsertManagedBlock(existing, ManagedBlockStart, ManagedBlockEnd, managed)
	return fileutil.WriteIfChangedTracked(path, []byte(updated))
}

func UpsertManagedBlock(existing, startMarker, endMarker, managedContent string) string {
	if existing == "" {
		return managedContent + "\n"
	}

	start := strings.Index(existing, startMarker)
	end := strings.Index(existing, endMarker)
	if start >= 0 && end >= start {
		end += len(endMarker)
		updated := existing[:start] + managedContent + existing[end:]
		return fileutil.EnsureTrailingNewline(updated)
	}

	base := fileutil.EnsureTrailingNewline(existing)
	return base + "\n" + managedContent + "\n"
}

func ContainsManagedBlock(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, ManagedBlockStart) && strings.Contains(text, ManagedBlockEnd)
}
