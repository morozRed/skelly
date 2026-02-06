package languages

import (
	"path/filepath"
	"strings"
)

func splitQualifiedName(raw string) (qualifier, name string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if idx := strings.LastIndex(raw, "."); idx != -1 {
		qualifier = strings.TrimSpace(raw[:idx])
		name = strings.TrimSpace(raw[idx+1:])
		return qualifier, name
	}
	return "", raw
}

func defaultImportAlias(path string) string {
	base := filepath.Base(strings.TrimSpace(path))
	if base == "." || base == "/" {
		return ""
	}
	return strings.TrimSpace(base)
}

func mergeImportAliases(dst map[string]string, src map[string]string) map[string]string {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]string, len(src))
	}
	for alias, target := range src {
		alias = strings.TrimSpace(alias)
		target = strings.TrimSpace(target)
		if alias == "" || target == "" {
			continue
		}
		dst[alias] = target
	}
	return dst
}

func splitAliasByAs(raw string) (base string, alias string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	parts := strings.Split(raw, " as ")
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), ""
	}
	base = strings.TrimSpace(strings.Join(parts[:len(parts)-1], " as "))
	alias = strings.TrimSpace(parts[len(parts)-1])
	return base, alias
}
