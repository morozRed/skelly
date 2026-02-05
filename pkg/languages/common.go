package languages

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/skelly-dev/skelly/internal/parser"
)

func dedupeCallSites(calls []parser.CallSite) []parser.CallSite {
	if len(calls) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(calls))
	out := make([]parser.CallSite, 0, len(calls))
	for _, call := range calls {
		call.Name = strings.TrimSpace(call.Name)
		call.Qualifier = strings.TrimSpace(call.Qualifier)
		call.Receiver = strings.TrimSpace(call.Receiver)
		call.Raw = strings.TrimSpace(call.Raw)
		if call.Name == "" {
			continue
		}

		key := strings.Join([]string{
			call.Name,
			call.Qualifier,
			call.Receiver,
			strconvInt(call.Line),
			strconvInt(call.Arity),
		}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, call)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		if out[i].Qualifier != out[j].Qualifier {
			return out[i].Qualifier < out[j].Qualifier
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Raw < out[j].Raw
	})
	return out
}

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

func strconvInt(v int) string {
	return strconv.Itoa(v)
}
