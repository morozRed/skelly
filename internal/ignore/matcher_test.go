package ignore

import "testing"

func TestMatcher_DefaultAndUserOverrides(t *testing.T) {
	m := NewMatcher([]string{
		"vendor/**",
		"!vendor/keep/file.go",
		"*.tmp",
	})

	cases := []struct {
		path    string
		isDir   bool
		ignored bool
	}{
		{path: ".git/config", isDir: false, ignored: true},
		{path: ".skelly/.context/index.txt", isDir: false, ignored: true},
		{path: "node_modules/pkg/index.js", isDir: false, ignored: true},
		{path: "vendor/lib/a.go", isDir: false, ignored: true},
		{path: "vendor/keep/file.go", isDir: false, ignored: false},
		{path: "nested/cache.tmp", isDir: false, ignored: true},
		{path: "src/main.go", isDir: false, ignored: false},
	}

	for _, tc := range cases {
		got := m.ShouldIgnore(tc.path, tc.isDir)
		if got != tc.ignored {
			t.Fatalf("path %s: expected ignored=%v, got %v", tc.path, tc.ignored, got)
		}
	}
}

func TestMatcher_NegatedDirectoryRule(t *testing.T) {
	m := NewMatcher([]string{
		"build/",
		"!build/include/",
	})

	if !m.ShouldIgnore("build/out/file.go", false) {
		t.Fatalf("expected build/out/file.go to be ignored")
	}
	if m.ShouldIgnore("build/include/file.go", false) {
		t.Fatalf("expected build/include/file.go to be included")
	}
}
