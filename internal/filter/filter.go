package filter

import (
	"strings"

	"github.com/izam-mohammed/claim-your-code/internal/pattern"
)

// StripClaudeCoAuthor removes Claude co-author lines from a commit message
// and cleans up any resulting trailing blank lines.
func StripClaudeCoAuthor(message string) string {
	lines := strings.Split(message, "\n")
	var result []string

	for _, line := range lines {
		if !pattern.IsClaudeCoAuthorLine(line) {
			result = append(result, line)
		}
	}

	// Remove trailing blank lines
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n") + "\n"
}
