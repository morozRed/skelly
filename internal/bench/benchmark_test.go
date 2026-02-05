package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/skelly-dev/skelly/internal/graph"
	"github.com/skelly-dev/skelly/pkg/languages"
)

func BenchmarkParseAndGraph_MediumRepo(b *testing.B) {
	root := b.TempDir()
	createSyntheticGoRepo(b, root, 250)

	registry := languages.NewDefaultRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := registry.ParseDirectory(root, nil)
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
		g := graph.BuildFromParseResult(result)
		if len(g.Nodes) == 0 {
			b.Fatalf("expected graph nodes")
		}
	}
}

func createSyntheticGoRepo(tb testing.TB, root string, files int) {
	tb.Helper()

	for i := 0; i < files; i++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%d", i%10))
		if err := os.MkdirAll(dir, 0755); err != nil {
			tb.Fatalf("mkdir failed: %v", err)
		}

		filePath := filepath.Join(dir, fmt.Sprintf("file_%03d.go", i))
		src := fmt.Sprintf(`package pkg%d

func Func%d() int {
	return helper%d()
}

func helper%d() int {
	return %d
}
`, i%10, i, i, i, i)

		if err := os.WriteFile(filePath, []byte(src), 0644); err != nil {
			tb.Fatalf("write failed: %v", err)
		}
	}
}
