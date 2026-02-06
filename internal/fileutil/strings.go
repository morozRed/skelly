package fileutil

import "sort"

func DedupeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func MapKeysSorted(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func ToSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, path := range paths {
		set[path] = true
	}
	return set
}

func ExistingFiles(paths []string, existing map[string]string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := existing[path]; ok {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

