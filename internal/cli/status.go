package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/languages"
	"github.com/morozRed/skelly/internal/output"
	"github.com/morozRed/skelly/internal/state"
	"github.com/spf13/cobra"
)

func RunStatus(cmd *cobra.Command, args []string) error {
	start := time.Now()
	rootPath, err := resolveWorkingDirectory()
	if err != nil {
		return err
	}
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("failed to read --json flag: %w", err)
	}

	registry := languages.NewDefaultRegistry()
	ignoreRules, err := LoadIgnoreRules(rootPath)
	if err != nil {
		return err
	}

	contextDir := filepath.Join(rootPath, output.ContextDir)
	st, err := state.Load(contextDir)
	if err != nil {
		if IsCorruptStateError(err) {
			fmt.Fprintf(os.Stderr, "warning: corrupt state file detected (%v); treating all files as changed\n", err)
			st = state.NewState()
		} else {
			return fmt.Errorf("failed to load state: %w", err)
		}
	}

	currentHashes, err := fileutil.ScanFileHashes(rootPath, registry, ignoreRules)
	if err != nil {
		return fmt.Errorf("failed to scan files: %w", err)
	}

	currentFiles := make(map[string]bool, len(currentHashes))
	for file := range currentHashes {
		currentFiles[file] = true
	}

	changed := st.ChangedFiles(currentHashes)
	deleted := st.DeletedFiles(currentFiles)
	for file, fileState := range st.Files {
		if currentFiles[file] && fileState.Language == "" && len(fileState.Symbols) == 0 {
			changed = append(changed, file)
		}
	}
	changed = fileutil.DedupeStrings(changed)
	sort.Strings(changed)
	sort.Strings(deleted)
	impacted, reasons := fileutil.ImpactedWithReasons(st, changed, deleted)

	summary := RunSummary{
		Mode:          "status",
		RootPath:      rootPath,
		Scanned:       len(currentHashes),
		Parsed:        len(changed),
		Reused:        MaxInt(len(currentHashes)-len(changed), 0),
		Rewritten:     0,
		Changed:       len(changed),
		Deleted:       len(deleted),
		Impacted:      len(impacted),
		DurationMS:    time.Since(start).Milliseconds(),
		ChangedFiles:  changed,
		DeletedFiles:  deleted,
		ImpactedFiles: impacted,
		Reasons:       reasons,
	}

	return PrintRunSummary(summary, asJSON)
}
