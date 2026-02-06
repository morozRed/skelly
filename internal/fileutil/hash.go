package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/morozRed/skelly/internal/ignore"
	"github.com/morozRed/skelly/internal/parser"
)

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

func ScanFileHashes(rootPath string, registry *parser.Registry, ignoreRules []string) (map[string]string, error) {
	hashes := make(map[string]string)
	ignoreMatcher := ignore.NewMatcher(ignoreRules)

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		if ignoreMatcher.ShouldIgnore(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if _, ok := registry.GetParserForFile(path); !ok {
			return nil
		}

		hash, err := HashFile(path)
		if err != nil {
			return err
		}
		hashes[relPath] = hash

		return nil
	})

	return hashes, err
}
