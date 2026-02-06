package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/morozRed/skelly/internal/fileutil"
	"github.com/morozRed/skelly/internal/output"
	"github.com/spf13/cobra"
)

const (
	HookStart = "# >>> skelly update hook >>>"
	HookEnd   = "# <<< skelly update hook <<<"
)

func RunInstallHook(cmd *cobra.Command, args []string) error {
	rootPath, err := resolveWorkingDirectory()
	if err != nil {
		return err
	}

	repoRoot, gitDir, err := ResolveGitPaths(rootPath)
	if err != nil {
		return err
	}

	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hookPath), 0755); err != nil {
		return fmt.Errorf("failed to create hook directory: %w", err)
	}

	existing := ""
	if data, err := os.ReadFile(hookPath); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing hook: %w", err)
	}

	updated := UpsertSkellyHook(existing, repoRoot)
	if err := os.WriteFile(hookPath, []byte(updated), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}

	fmt.Printf("Installed pre-commit hook at %s\n", hookPath)
	return nil
}

func ResolveGitPaths(workingDir string) (repoRoot string, gitDir string, err error) {
	repoRootOut, err := exec.Command("git", "-C", workingDir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", fmt.Errorf("not inside a git repository")
	}

	gitDirOut, err := exec.Command("git", "-C", workingDir, "rev-parse", "--git-dir").Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve git directory: %w", err)
	}

	repoRoot = strings.TrimSpace(string(repoRootOut))
	gitDir = strings.TrimSpace(string(gitDirOut))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoRoot, gitDir)
	}
	return repoRoot, gitDir, nil
}

func UpsertSkellyHook(existingHook, repoRoot string) string {
	block := BuildSkellyHookBlock(repoRoot)

	if existingHook == "" {
		return "#!/bin/sh\n\n" + block + "\n"
	}

	start := strings.Index(existingHook, HookStart)
	end := strings.Index(existingHook, HookEnd)
	if start >= 0 && end >= start {
		end += len(HookEnd)
		updated := existingHook[:start] + block + existingHook[end:]
		return fileutil.EnsureTrailingNewline(updated)
	}

	base := fileutil.EnsureTrailingNewline(existingHook)
	if !strings.HasPrefix(base, "#!") {
		base = "#!/bin/sh\n" + base
	}
	return base + "\n" + block + "\n"
}

func BuildSkellyHookBlock(repoRoot string) string {
	return fmt.Sprintf(
		"%s\nrepo_root=%q\ncontext_dir=\"$repo_root/%s\"\nif command -v skelly >/dev/null 2>&1; then\n  if [ -f \"$context_dir/manifest.json\" ] && [ -f \"$context_dir/symbols.jsonl\" ] && [ -f \"$context_dir/edges.jsonl\" ]; then\n    (cd \"$repo_root\" && skelly update --format jsonl) || exit 1\n  else\n    (cd \"$repo_root\" && skelly update) || exit 1\n  fi\nfi\n%s",
		HookStart,
		repoRoot,
		output.ContextDir,
		HookEnd,
	)
}
