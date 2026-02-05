package parser

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

// StableSymbolID returns a deterministic ID for a symbol.
// Format: file|line|kind|name|signature-hash.
func StableSymbolID(file string, symbol Symbol) string {
	base := fmt.Sprintf("%s|%d|%s|%s", file, symbol.Line, symbol.Kind.String(), symbol.Name)

	if symbol.Signature == "" {
		return base
	}

	sigHash := sha1.Sum([]byte(symbol.Signature))
	return fmt.Sprintf("%s|%s", base, hex.EncodeToString(sigHash[:4]))
}
