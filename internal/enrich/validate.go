package enrich

import (
	"fmt"
	"strings"
)

func ValidateOutput(output Output) error {
	if strings.TrimSpace(output.Summary) == "" {
		return fmt.Errorf("missing output.summary")
	}
	if strings.TrimSpace(output.Purpose) == "" {
		return fmt.Errorf("missing output.purpose")
	}
	if strings.TrimSpace(output.SideEffects) == "" {
		return fmt.Errorf("missing output.side_effects")
	}
	confidence := strings.ToLower(strings.TrimSpace(output.Confidence))
	switch confidence {
	case "low", "medium", "high":
		return nil
	default:
		return fmt.Errorf("output.confidence must be one of low|medium|high")
	}
}
