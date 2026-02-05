package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/skelly-dev/skelly/internal/output"
	"github.com/skelly-dev/skelly/internal/state"
	"github.com/spf13/cobra"
)

func TestInitGenerateUpdateFlow(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {
	B()
}

func B() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}

		genCmd := newGenerateCmdForTest()
		if err := runGenerate(genCmd, []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		indexPath := filepath.Join(root, output.ContextDir, "index.txt")
		graphPath := filepath.Join(root, output.ContextDir, "graph.txt")
		statePath := filepath.Join(root, output.ContextDir, ".state.json")
		assertExists(t, indexPath)
		assertExists(t, graphPath)
		assertExists(t, statePath)

		firstGraph, err := os.ReadFile(graphPath)
		if err != nil {
			t.Fatalf("failed to read graph file: %v", err)
		}

		if err := runGenerate(genCmd, []string{"."}); err != nil {
			t.Fatalf("second runGenerate failed: %v", err)
		}

		secondGraph, err := os.ReadFile(graphPath)
		if err != nil {
			t.Fatalf("failed to read graph file (second run): %v", err)
		}
		if string(firstGraph) != string(secondGraph) {
			t.Fatalf("expected deterministic graph output between runs")
		}

		mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {
	B()
	C()
}

func B() {}
func C() {}
`)

		updateCmd := newUpdateCmdForTest()
		if err := runUpdate(updateCmd, nil); err != nil {
			t.Fatalf("runUpdate failed: %v", err)
		}

		updatedGraph, err := os.ReadFile(graphPath)
		if err != nil {
			t.Fatalf("failed to read graph file after update: %v", err)
		}
		if !strings.Contains(string(updatedGraph), "|C|") {
			t.Fatalf("expected updated graph output to contain symbol C")
		}
	})
}

func TestInitWithLLMGeneratesIntegrationFilesIdempotently(t *testing.T) {
	root := t.TempDir()

	withWorkingDir(t, root, func() {
		initCmd := newInitCmdForTest()
		mustSetFlag(t, initCmd, "llm", "codex,claude,cursor")
		if err := runInit(initCmd, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}

		expected := []string{
			filepath.Join(root, output.ContextDir, ".state.json"),
			filepath.Join(root, ".skelly", "skills", "skelly.md"),
			filepath.Join(root, "CONTEXT.md"),
			filepath.Join(root, "AGENTS.md"),
			filepath.Join(root, "CLAUDE.md"),
			filepath.Join(root, ".cursor", "rules", "skelly-context.mdc"),
		}
		for _, path := range expected {
			assertExists(t, path)
		}

		agentsBefore, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
		if err != nil {
			t.Fatalf("failed to read AGENTS.md: %v", err)
		}
		if !strings.Contains(string(agentsBefore), llmManagedBlockStart) {
			t.Fatalf("expected AGENTS.md to include managed block marker")
		}

		if err := runInit(initCmd, nil); err != nil {
			t.Fatalf("second runInit failed: %v", err)
		}

		agentsAfter, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
		if err != nil {
			t.Fatalf("failed to read AGENTS.md after rerun: %v", err)
		}
		if !bytes.Equal(agentsBefore, agentsAfter) {
			t.Fatalf("expected init --llm to be idempotent for AGENTS.md")
		}
	})
}

func TestDoctorReportsHealthyAndStaleStates(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {}
`)

	withWorkingDir(t, root, func() {
		initCmd := newInitCmdForTest()
		mustSetFlag(t, initCmd, "llm", "all")
		if err := runInit(initCmd, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}
		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		var healthy DoctorSummary
		doctorCmd := newDoctorCmdForTest()
		mustSetFlag(t, doctorCmd, "json", "true")
		stdout := captureStdout(t, func() {
			if err := runDoctor(doctorCmd, nil); err != nil {
				t.Fatalf("runDoctor failed: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &healthy); err != nil {
			t.Fatalf("failed to decode healthy doctor output: %v\noutput=%s", err, stdout)
		}
		if !healthy.Healthy {
			t.Fatalf("expected healthy doctor status, got %+v", healthy)
		}
		if !healthy.Clean || healthy.Changed != 0 || healthy.Deleted != 0 {
			t.Fatalf("expected clean doctor summary, got %+v", healthy)
		}

		mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() { B() }
func B() {}
`)

		var stale DoctorSummary
		stdout = captureStdout(t, func() {
			if err := runDoctor(doctorCmd, nil); err != nil {
				t.Fatalf("runDoctor after edits failed: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &stale); err != nil {
			t.Fatalf("failed to decode stale doctor output: %v\noutput=%s", err, stdout)
		}
		if stale.Clean {
			t.Fatalf("expected stale doctor summary to be dirty, got %+v", stale)
		}
		if stale.Changed == 0 {
			t.Fatalf("expected stale doctor summary to report changed files, got %+v", stale)
		}
		if !containsString(stale.Suggestions, "run skelly update") {
			t.Fatalf("expected stale doctor suggestions to include skelly update, got %#v", stale.Suggestions)
		}
	})
}

func TestGenerateWritesNavigationIndex(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() { B() }
func B() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(newInitCmdForTest(), nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}
		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		indexPath := filepath.Join(root, output.ContextDir, navigationIndexFile)
		assertExists(t, indexPath)

		data, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatalf("failed to read navigation index: %v", err)
		}
		var payload navigationIndex
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("failed to decode navigation index: %v", err)
		}
		if payload.Version == "" {
			t.Fatalf("expected navigation index version to be set")
		}
		if len(payload.Nodes) < 2 {
			t.Fatalf("expected navigation index to include at least 2 nodes, got %d", len(payload.Nodes))
		}
	})
}

func TestNavigationCommandsJSON(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func Start() { Mid() }
func Mid() { End() }
func End() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(newInitCmdForTest(), nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}
		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		symbolCmd := newSymbolCmdForTest()
		mustSetFlag(t, symbolCmd, "json", "true")
		var symbolPayload struct {
			Query   string                   `json:"query"`
			Matches []navigationSymbolRecord `json:"matches"`
		}
		stdout := captureStdout(t, func() {
			if err := runSymbol(symbolCmd, []string{"Mid"}); err != nil {
				t.Fatalf("runSymbol failed: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &symbolPayload); err != nil {
			t.Fatalf("failed to decode symbol output: %v\noutput=%s", err, stdout)
		}
		if len(symbolPayload.Matches) != 1 {
			t.Fatalf("expected one symbol match for Mid, got %#v", symbolPayload.Matches)
		}
		midID := symbolPayload.Matches[0].ID

		callersCmd := newCallersCmdForTest()
		mustSetFlag(t, callersCmd, "json", "true")
		var callersPayload struct {
			Callers []navigationEdgeRecord `json:"callers"`
		}
		stdout = captureStdout(t, func() {
			if err := runCallers(callersCmd, []string{"Mid"}); err != nil {
				t.Fatalf("runCallers failed: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &callersPayload); err != nil {
			t.Fatalf("failed to decode callers output: %v\noutput=%s", err, stdout)
		}
		if len(callersPayload.Callers) != 1 || callersPayload.Callers[0].Symbol.Name != "Start" {
			t.Fatalf("expected Start to call Mid, got %#v", callersPayload.Callers)
		}

		calleesCmd := newCalleesCmdForTest()
		mustSetFlag(t, calleesCmd, "json", "true")
		var calleesPayload struct {
			Callees []navigationEdgeRecord `json:"callees"`
		}
		stdout = captureStdout(t, func() {
			if err := runCallees(calleesCmd, []string{midID}); err != nil {
				t.Fatalf("runCallees failed: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &calleesPayload); err != nil {
			t.Fatalf("failed to decode callees output: %v\noutput=%s", err, stdout)
		}
		if len(calleesPayload.Callees) != 1 || calleesPayload.Callees[0].Symbol.Name != "End" {
			t.Fatalf("expected End as callee of Mid, got %#v", calleesPayload.Callees)
		}

		traceCmd := newTraceCmdForTest()
		mustSetFlag(t, traceCmd, "depth", "2")
		mustSetFlag(t, traceCmd, "json", "true")
		var tracePayload struct {
			Hops []navigationTraceHop `json:"hops"`
		}
		stdout = captureStdout(t, func() {
			if err := runTrace(traceCmd, []string{"Start"}); err != nil {
				t.Fatalf("runTrace failed: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &tracePayload); err != nil {
			t.Fatalf("failed to decode trace output: %v\noutput=%s", err, stdout)
		}
		if len(tracePayload.Hops) < 2 {
			t.Fatalf("expected at least two hops in trace, got %#v", tracePayload.Hops)
		}

		pathCmd := newPathCmdForTest()
		mustSetFlag(t, pathCmd, "json", "true")
		var pathPayload struct {
			Length int                      `json:"length"`
			Path   []navigationSymbolRecord `json:"path"`
		}
		stdout = captureStdout(t, func() {
			if err := runPath(pathCmd, []string{"Start", "End"}); err != nil {
				t.Fatalf("runPath failed: %v", err)
			}
		})
		if err := json.Unmarshal([]byte(stdout), &pathPayload); err != nil {
			t.Fatalf("failed to decode path output: %v\noutput=%s", err, stdout)
		}
		if pathPayload.Length != 2 {
			t.Fatalf("expected path length 2, got %#v", pathPayload)
		}
		if len(pathPayload.Path) != 3 {
			t.Fatalf("expected path with 3 symbols, got %#v", pathPayload.Path)
		}
	})
}

func TestGenerateParsesSupportedLanguageFixtures(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go", "main.go"), `package demo
func A() {}
`)
	mustWriteFile(t, filepath.Join(root, "python", "main.py"), `def run():
    return helper()

def helper():
    return 1
`)
	mustWriteFile(t, filepath.Join(root, "ruby", "main.rb"), `def run
  helper
end

def helper
  1
end
`)
	mustWriteFile(t, filepath.Join(root, "typescript", "main.ts"), `export function run() { return helper(); }
function helper() { return 1; }
`)
	mustWriteFile(t, filepath.Join(root, "javascript", "main.js"), `export function run() { return helper(); }
function helper() { return 1; }
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}

		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		indexData, err := os.ReadFile(filepath.Join(root, output.ContextDir, "index.txt"))
		if err != nil {
			t.Fatalf("failed to read index output: %v", err)
		}
		indexText := string(indexData)
		for _, expected := range []string{
			"go/main.go",
			"python/main.py",
			"ruby/main.rb",
			"typescript/main.ts",
			"javascript/main.js",
		} {
			if !strings.Contains(indexText, expected) {
				t.Fatalf("expected index to contain %s", expected)
			}
		}
	})
}

func TestGenerateJSONLDeterministic(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {
	B()
}

func B() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}

		genCmd := newGenerateCmdForTest()
		mustSetFlag(t, genCmd, "format", "jsonl")
		if err := runGenerate(genCmd, []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		symbolsPath := filepath.Join(root, output.ContextDir, "symbols.jsonl")
		edgesPath := filepath.Join(root, output.ContextDir, "edges.jsonl")
		manifestPath := filepath.Join(root, output.ContextDir, "manifest.json")

		assertExists(t, symbolsPath)
		assertExists(t, edgesPath)
		assertExists(t, manifestPath)
		assertNotExists(t, filepath.Join(root, output.ContextDir, "index.txt"))
		assertNotExists(t, filepath.Join(root, output.ContextDir, "graph.txt"))

		firstSymbols, err := os.ReadFile(symbolsPath)
		if err != nil {
			t.Fatalf("failed to read symbols jsonl: %v", err)
		}
		firstEdges, err := os.ReadFile(edgesPath)
		if err != nil {
			t.Fatalf("failed to read edges jsonl: %v", err)
		}
		firstManifest, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("failed to read manifest: %v", err)
		}

		assertSortedSymbolJSONL(t, firstSymbols)

		if err := runGenerate(genCmd, []string{"."}); err != nil {
			t.Fatalf("second runGenerate failed: %v", err)
		}

		secondSymbols, err := os.ReadFile(symbolsPath)
		if err != nil {
			t.Fatalf("failed to read symbols jsonl on second run: %v", err)
		}
		secondEdges, err := os.ReadFile(edgesPath)
		if err != nil {
			t.Fatalf("failed to read edges jsonl on second run: %v", err)
		}
		secondManifest, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("failed to read manifest on second run: %v", err)
		}

		if !bytes.Equal(firstSymbols, secondSymbols) {
			t.Fatalf("expected deterministic symbols jsonl output")
		}
		if !bytes.Equal(firstEdges, secondEdges) {
			t.Fatalf("expected deterministic edges jsonl output")
		}
		if !bytes.Equal(firstManifest, secondManifest) {
			t.Fatalf("expected deterministic manifest output")
		}
	})
}

func TestUpdateJSONLTracksArtifactHashesIncrementally(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {
	B()
}

func B() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}

		genCmd := newGenerateCmdForTest()
		mustSetFlag(t, genCmd, "format", "jsonl")
		if err := runGenerate(genCmd, []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		contextDir := filepath.Join(root, output.ContextDir)
		stBefore, err := state.Load(contextDir)
		if err != nil {
			t.Fatalf("failed to load state after generate: %v", err)
		}
		hashesBefore := cloneStringMap(stBefore.OutputHashes)

		updateCmd := newUpdateCmdForTest()
		mustSetFlag(t, updateCmd, "format", "jsonl")
		if err := runUpdate(updateCmd, nil); err != nil {
			t.Fatalf("runUpdate failed: %v", err)
		}

		stUnchanged, err := state.Load(contextDir)
		if err != nil {
			t.Fatalf("failed to load state after unchanged update: %v", err)
		}
		if !reflect.DeepEqual(hashesBefore, stUnchanged.OutputHashes) {
			t.Fatalf("expected output hashes to remain stable on unchanged update")
		}

		mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {
	B()
	C()
}

func B() {}
func C() {}
`)

		if err := runUpdate(updateCmd, nil); err != nil {
			t.Fatalf("runUpdate after changes failed: %v", err)
		}

		stAfter, err := state.Load(contextDir)
		if err != nil {
			t.Fatalf("failed to load state after changed update: %v", err)
		}

		if stAfter.OutputHashes["symbols.jsonl"] == hashesBefore["symbols.jsonl"] {
			t.Fatalf("expected symbols.jsonl hash to change after source updates")
		}
		if stAfter.OutputHashes["edges.jsonl"] == hashesBefore["edges.jsonl"] {
			t.Fatalf("expected edges.jsonl hash to change after source updates")
		}
		if stAfter.OutputHashes["manifest.json"] == hashesBefore["manifest.json"] {
			t.Fatalf("expected manifest.json hash to change after source updates")
		}
	})
}

func TestUpdateCanSwitchToJSONLWithoutSourceChanges(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}
		mustWriteAgentProfile(t, root)
		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		contextDir := filepath.Join(root, output.ContextDir)
		assertExists(t, filepath.Join(contextDir, "index.txt"))
		assertNotExists(t, filepath.Join(contextDir, "symbols.jsonl"))

		updateCmd := newUpdateCmdForTest()
		mustSetFlag(t, updateCmd, "format", "jsonl")
		if err := runUpdate(updateCmd, nil); err != nil {
			t.Fatalf("runUpdate failed: %v", err)
		}

		assertExists(t, filepath.Join(contextDir, "symbols.jsonl"))
		assertExists(t, filepath.Join(contextDir, "edges.jsonl"))
		assertExists(t, filepath.Join(contextDir, "manifest.json"))
		assertNotExists(t, filepath.Join(contextDir, "index.txt"))
		assertNotExists(t, filepath.Join(contextDir, "graph.txt"))
	})
}

func TestEnrichDryRunChangedScope(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}
		mustWriteAgentProfile(t, root)
		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		enrichCmd := newEnrichCmdForTest()
		mustSetFlag(t, enrichCmd, "agent", "local")
		mustSetFlag(t, enrichCmd, "scope", "changed")
		mustSetFlag(t, enrichCmd, "dry-run", "true")
		if err := runEnrich(enrichCmd, nil); err != nil {
			t.Fatalf("runEnrich dry-run failed: %v", err)
		}

		assertNotExists(t, filepath.Join(root, output.ContextDir, "enrich.jsonl"))
	})
}

func TestEnrichAllWritesJSONL(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() { B() }
func B() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}
		mustWriteAgentProfile(t, root)
		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		enrichCmd := newEnrichCmdForTest()
		mustSetFlag(t, enrichCmd, "agent", "local")
		mustSetFlag(t, enrichCmd, "scope", "all")
		mustSetFlag(t, enrichCmd, "max-symbols", "1")
		if err := runEnrich(enrichCmd, nil); err != nil {
			t.Fatalf("runEnrich failed: %v", err)
		}

		enrichPath := filepath.Join(root, output.ContextDir, "enrich.jsonl")
		assertExists(t, enrichPath)

		data, err := os.ReadFile(enrichPath)
		if err != nil {
			t.Fatalf("failed to read enrich output: %v", err)
		}
		lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
		if len(lines) != 1 {
			t.Fatalf("expected 1 enrich record, got %d", len(lines))
		}

		var row map[string]any
		if err := json.Unmarshal(lines[0], &row); err != nil {
			t.Fatalf("failed to decode enrich record: %v", err)
		}
		if row["symbol_id"] == "" {
			t.Fatalf("expected enrich record to include symbol_id")
		}
		outputValue, ok := row["output"].(map[string]any)
		if !ok {
			t.Fatalf("expected enrich output payload")
		}
		if outputValue["summary"] == "" {
			t.Fatalf("expected enrich output.summary to be present")
		}
	})
}

func TestEnrichAutoCreatesDefaultAgentProfile(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}
		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		enrichCmd := newEnrichCmdForTest()
		mustSetFlag(t, enrichCmd, "agent", "local")
		mustSetFlag(t, enrichCmd, "scope", "all")
		mustSetFlag(t, enrichCmd, "max-symbols", "1")
		if err := runEnrich(enrichCmd, nil); err != nil {
			t.Fatalf("runEnrich failed: %v", err)
		}

		assertExists(t, filepath.Join(root, ".skelly", "agents.yaml"))
		assertExists(t, filepath.Join(root, ".skelly", "default_agent.py"))
		assertExists(t, filepath.Join(root, ".skelly", "enrich-output-schema.json"))
		assertExists(t, filepath.Join(root, output.ContextDir, "enrich.jsonl"))
	})
}

func TestEnrichSupportsCommandPlaceholders(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {}
`)

	withWorkingDir(t, root, func() {
		if err := runInit(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runInit failed: %v", err)
		}
		mustWritePlaceholderAgentProfile(t, root)
		if err := runGenerate(newGenerateCmdForTest(), []string{"."}); err != nil {
			t.Fatalf("runGenerate failed: %v", err)
		}

		enrichCmd := newEnrichCmdForTest()
		mustSetFlag(t, enrichCmd, "agent", "codex")
		mustSetFlag(t, enrichCmd, "scope", "all")
		mustSetFlag(t, enrichCmd, "max-symbols", "1")
		if err := runEnrich(enrichCmd, nil); err != nil {
			t.Fatalf("runEnrich failed: %v", err)
		}

		enrichPath := filepath.Join(root, output.ContextDir, "enrich.jsonl")
		data, err := os.ReadFile(enrichPath)
		if err != nil {
			t.Fatalf("failed to read enrich output: %v", err)
		}
		var row map[string]any
		lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
		if len(lines) != 1 {
			t.Fatalf("expected 1 enrich record, got %d", len(lines))
		}
		if err := json.Unmarshal(lines[0], &row); err != nil {
			t.Fatalf("failed to decode enrich output: %v", err)
		}
		outputValue, ok := row["output"].(map[string]any)
		if !ok {
			t.Fatalf("expected output payload")
		}
		summary, _ := outputValue["summary"].(string)
		if !strings.Contains(summary, "schema-properties=") {
			t.Fatalf("expected summary to include schema marker, got: %q", summary)
		}
	})
}

func TestSetupRunsGenerateAndEnrich(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "demo.go"), `package demo

func A() {}
`)

	withWorkingDir(t, root, func() {
		mustWriteAgentProfile(t, root)

		setupCmd := newSetupCmdForTest()
		mustSetFlag(t, setupCmd, "agent", "local")
		mustSetFlag(t, setupCmd, "scope", "all")
		mustSetFlag(t, setupCmd, "max-symbols", "1")
		if err := runSetup(setupCmd, nil); err != nil {
			t.Fatalf("runSetup failed: %v", err)
		}

		assertExists(t, filepath.Join(root, output.ContextDir, "index.txt"))
		assertExists(t, filepath.Join(root, output.ContextDir, "graph.txt"))
		assertExists(t, filepath.Join(root, output.ContextDir, ".state.json"))
		assertExists(t, filepath.Join(root, output.ContextDir, "enrich.jsonl"))
	})
}

func newGenerateCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("lang", []string{}, "")
	cmd.Flags().String("format", "text", "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newInitCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("llm", "", "")
	return cmd
}

func newUpdateCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("explain", false, "")
	cmd.Flags().String("format", "text", "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newEnrichCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("agent", "", "")
	cmd.Flags().String("scope", "changed", "")
	cmd.Flags().Int("max-symbols", 200, "")
	cmd.Flags().Duration("timeout", 30*time.Second, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newSetupCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("agent", "", "")
	cmd.Flags().String("scope", "all", "")
	cmd.Flags().Int("max-symbols", 200, "")
	cmd.Flags().Duration("timeout", 30*time.Second, "")
	cmd.Flags().String("format", "text", "")
	return cmd
}

func newDoctorCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newSymbolCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newCallersCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newCalleesCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newTraceCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Int("depth", 2, "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newPathCmdForTest() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()

	fn()
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to not exist", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent: %v", path, err)
	}
}

func mustSetFlag(t *testing.T, cmd *cobra.Command, key, value string) {
	t.Helper()
	if err := cmd.Flags().Set(key, value); err != nil {
		t.Fatalf("failed to set --%s=%s: %v", key, value, err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = original
		_ = writer.Close()
		_ = reader.Close()
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close stdout writer: %v", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	return string(data)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func assertSortedSymbolJSONL(t *testing.T, data []byte) {
	t.Helper()

	type symbolLine struct {
		ID   string `json:"id"`
		File string `json:"file"`
		Line int    `json:"line"`
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	var prev symbolLine
	hasPrev := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var record symbolLine
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("failed to decode symbol jsonl line: %v", err)
		}
		if hasPrev {
			if record.File < prev.File {
				t.Fatalf("symbols.jsonl is not sorted by file: %s < %s", record.File, prev.File)
			}
			if record.File == prev.File && record.Line < prev.Line {
				t.Fatalf("symbols.jsonl is not sorted by line within file %s", record.File)
			}
			if record.File == prev.File && record.Line == prev.Line && record.ID < prev.ID {
				t.Fatalf("symbols.jsonl is not sorted by id for %s:%d", record.File, record.Line)
			}
		}
		prev = record
		hasPrev = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("failed reading symbols jsonl: %v", err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}

func mustWriteAgentProfile(t *testing.T, root string) {
	t.Helper()

	mustWriteFile(t, filepath.Join(root, ".skelly", "mock_agent.py"), `#!/usr/bin/env python3
import json
import sys

req = json.load(sys.stdin)
symbol = req.get("input", {}).get("symbol", {})
name = symbol.get("name", "unknown")
print(json.dumps({
    "summary": f"Summary for {name}",
    "purpose": f"Purpose for {name}",
    "side_effects": "No side effects",
    "confidence": "medium"
}))
`)
	mustWriteFile(t, filepath.Join(root, ".skelly", "agents.yaml"), `profiles:
  local:
    command: ["python3", ".skelly/mock_agent.py"]
    prompt_template: |
      Summarize {{ .Symbol.Name }}
`)
}

func mustWritePlaceholderAgentProfile(t *testing.T, root string) {
	t.Helper()

	mustWriteFile(t, filepath.Join(root, ".skelly", "placeholder_agent.py"), `#!/usr/bin/env python3
import json
import sys

request = json.loads(sys.argv[1])
with open(sys.argv[2], "r", encoding="utf-8") as f:
    schema = json.load(f)
prompt = sys.argv[3]

symbol = ((request.get("input") or {}).get("symbol") or {})
name = symbol.get("name", "symbol")

response = {
    "summary": f"schema-properties={len((schema.get('properties') or {}))} prompt={prompt[:30]}",
    "purpose": f"Purpose for {name}",
    "side_effects": "None",
    "confidence": "low",
}
print(json.dumps(response))
`)
	mustWriteFile(t, filepath.Join(root, ".skelly", "agents.yaml"), `profiles:
  codex:
    command: ["python3", ".skelly/placeholder_agent.py", REQUEST_JSON, JSON_SCHEMA, PROMPT]
    timeout: 15s
    prompt_template: |
      Summarize {{ .Symbol.Name }} now
`)
}
