package lsp

import (
	"os/exec"
	"path/filepath"
	"strings"
)

type Capability struct {
	Present   bool   `json:"present"`
	Server    string `json:"server"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

var languageServers = map[string][]string{
	"go":         {"gopls"},
	"python":     {"pyright-langserver", "pylsp"},
	"typescript": {"typescript-language-server"},
	"ruby":       {"solargraph"},
}

var languageExtensions = map[string][]string{
	"go":         {".go"},
	"python":     {".py"},
	"typescript": {".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"},
	"ruby":       {".rb"},
}

func LanguageForPath(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	for language, exts := range languageExtensions {
		for _, candidate := range exts {
			if ext == candidate {
				return language, true
			}
		}
	}
	return "", false
}

func DetectLanguagePresence(paths []string) map[string]bool {
	presence := make(map[string]bool, len(languageServers))
	for language := range languageServers {
		presence[language] = false
	}

	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		for language, exts := range languageExtensions {
			for _, candidate := range exts {
				if ext == candidate {
					presence[language] = true
					break
				}
			}
		}
	}

	return presence
}

func ProbeCapabilities(presence map[string]bool) map[string]Capability {
	return ProbeCapabilitiesWithLookPath(presence, exec.LookPath)
}

func ProbeCapabilitiesWithLookPath(presence map[string]bool, lookPath func(file string) (string, error)) map[string]Capability {
	capabilities := make(map[string]Capability, len(languageServers))
	for language, preferredServers := range languageServers {
		capability := Capability{Present: presence[language]}
		if len(preferredServers) > 0 {
			capability.Server = preferredServers[0]
		}

		if !capability.Present {
			capability.Reason = "language_not_present"
			capabilities[language] = capability
			continue
		}

		for _, server := range preferredServers {
			if _, err := lookPath(server); err == nil {
				capability.Available = true
				capability.Server = server
				capability.Reason = ""
				break
			}
		}
		if !capability.Available {
			capability.Reason = "server_not_found"
		}
		capabilities[language] = capability
	}
	return capabilities
}
