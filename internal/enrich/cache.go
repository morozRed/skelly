package enrich

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
)

func CacheKey(symbolID, fileHash, promptVersion, agentProfile, model string) string {
	seed := strings.Join([]string{
		strings.TrimSpace(symbolID),
		strings.TrimSpace(fileHash),
		strings.TrimSpace(promptVersion),
		strings.TrimSpace(agentProfile),
		strings.TrimSpace(model),
	}, "|")
	sum := sha256.Sum256([]byte(seed))
	return "cache-v1-" + hex.EncodeToString(sum[:8])
}

func LoadCache(path string) (map[string]Record, error) {
	cache := make(map[string]Record)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return nil, fmt.Errorf("failed to read enrich cache: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.AgentProfile == "" {
			record.AgentProfile = record.Agent
		}
		if record.PromptVersion == "" {
			record.PromptVersion = "prompt-v1-legacy"
		}
		if record.Model == "" {
			record.Model = "unknown"
		}
		if record.CacheKey == "" {
			record.CacheKey = CacheKey(record.SymbolID, record.FileHash, record.PromptVersion, record.AgentProfile, record.Model)
		}
		if record.CacheKey == "" {
			continue
		}

		existing, exists := cache[record.CacheKey]
		if !exists {
			cache[record.CacheKey] = record
			continue
		}
		existingTS := existing.UpdatedAt
		if existingTS == "" {
			existingTS = existing.GeneratedAt
		}
		recordTS := record.UpdatedAt
		if recordTS == "" {
			recordTS = record.GeneratedAt
		}
		if recordTS >= existingTS {
			cache[record.CacheKey] = record
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse enrich cache: %w", err)
	}
	return cache, nil
}

func WriteCache(path string, cache map[string]Record) error {
	records := make([]Record, 0, len(cache))
	for _, record := range cache {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].SymbolID == records[j].SymbolID {
			if records[i].AgentProfile == records[j].AgentProfile {
				return records[i].CacheKey < records[j].CacheKey
			}
			return records[i].AgentProfile < records[j].AgentProfile
		}
		return records[i].SymbolID < records[j].SymbolID
	})

	data, err := fileutil.EncodeJSONL(records)
	if err != nil {
		return fmt.Errorf("failed to encode enrich output: %w", err)
	}
	if err := fileutil.WriteIfChanged(path, data); err != nil {
		return fmt.Errorf("failed to write enrich output: %w", err)
	}
	return nil
}

func PruneCacheForSymbol(cache map[string]Record, keepKey, symbolID, agentProfile string) {
	for key, record := range cache {
		if key == keepKey {
			continue
		}
		if record.SymbolID != symbolID {
			continue
		}
		profile := record.AgentProfile
		if profile == "" {
			profile = record.Agent
		}
		if profile != agentProfile {
			continue
		}
		delete(cache, key)
	}
}
