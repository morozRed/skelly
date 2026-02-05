package ignore

import (
	"path/filepath"
	"regexp"
	"strings"
)

type rule struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
}

// Matcher applies gitignore-like rules with "last rule wins" behavior.
type Matcher struct {
	rules []rule
}

// NewMatcher builds a matcher from user-provided .skellyignore lines.
// Default excludes are prepended and can be overridden by user negation rules.
func NewMatcher(userRules []string) *Matcher {
	defaultRules := []string{
		".git/",
		".skelly/",
		".context/",
		"node_modules/",
		"vendor/",
		"dist/",
		"build/",
		"target/",
		"__pycache__/",
	}

	all := make([]string, 0, len(defaultRules)+len(userRules))
	all = append(all, defaultRules...)
	all = append(all, userRules...)

	rules := make([]rule, 0, len(all))
	for _, line := range all {
		if parsed, ok := parseRule(line); ok {
			rules = append(rules, parsed)
		}
	}

	return &Matcher{rules: rules}
}

// ShouldIgnore returns true when relPath should be excluded.
func (m *Matcher) ShouldIgnore(relPath string, isDir bool) bool {
	relPath = normalizePath(relPath)
	ignored := false
	for _, rule := range m.rules {
		if ruleMatches(rule, relPath, isDir) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func parseRule(line string) (rule, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return rule{}, false
	}

	parsed := rule{}
	if strings.HasPrefix(line, "!") {
		parsed.negated = true
		line = strings.TrimPrefix(line, "!")
	}
	if strings.HasPrefix(line, "/") {
		parsed.anchored = true
		line = strings.TrimPrefix(line, "/")
	}
	if strings.HasSuffix(line, "/") {
		parsed.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	line = normalizePath(line)
	if line == "" {
		return rule{}, false
	}
	parsed.pattern = line
	return parsed, true
}

func ruleMatches(rule rule, relPath string, isDir bool) bool {
	relPath = normalizePath(relPath)

	if rule.dirOnly {
		if matchDirectoryPattern(rule, relPath) {
			return true
		}
		if isDir && matchPathPattern(rule.pattern, filepath.Base(relPath)) {
			return true
		}
		return false
	}

	if rule.anchored {
		return matchPathPattern(rule.pattern, relPath)
	}

	if strings.Contains(rule.pattern, "/") {
		if matchPathPattern(rule.pattern, relPath) {
			return true
		}
		parts := strings.Split(relPath, "/")
		for i := 1; i < len(parts); i++ {
			if matchPathPattern(rule.pattern, strings.Join(parts[i:], "/")) {
				return true
			}
		}
		return false
	}

	base := filepath.Base(relPath)
	if matchPathPattern(rule.pattern, base) {
		return true
	}

	for _, segment := range strings.Split(relPath, "/") {
		if matchPathPattern(rule.pattern, segment) {
			return true
		}
	}
	return false
}

func matchDirectoryPattern(rule rule, relPath string) bool {
	if rule.anchored {
		return relPath == rule.pattern || strings.HasPrefix(relPath, rule.pattern+"/")
	}

	if relPath == rule.pattern || strings.HasPrefix(relPath, rule.pattern+"/") {
		return true
	}

	parts := strings.Split(relPath, "/")
	for i := range parts {
		candidate := strings.Join(parts[:i+1], "/")
		if candidate == rule.pattern {
			return true
		}
	}
	return false
}

func matchPathPattern(pattern, value string) bool {
	re := globToRegex(pattern)
	ok, err := regexp.MatchString("^"+re+"$", value)
	return err == nil && ok
}

func globToRegex(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]

		if ch == '*' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString("[^/]*")
			continue
		}

		if ch == '?' {
			b.WriteString("[^/]")
			continue
		}

		if strings.ContainsRune(`.+()|[]{}^$\\`, rune(ch)) {
			b.WriteByte('\\')
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func normalizePath(path string) string {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	return path
}
