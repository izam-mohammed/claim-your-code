package pattern

import (
	"regexp"
	"strings"
)

// ClaudeCoAuthorRe matches Co-Authored-By lines for any Claude model variant.
// Examples:
//   - Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
//   - Co-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>
//   - Co-Authored-By: Claude Haiku 3.5 <noreply@anthropic.com>
var ClaudeCoAuthorRe = regexp.MustCompile(`(?im)^\s*Co-Authored-By:\s*Claude\b[^<]*<[^>]*@anthropic\.com>\s*$`)

// modelNameRe extracts the model name from a Co-Authored-By line.
// Captures the part between "Claude " and " <" (e.g. "Opus 4.6", "Sonnet 4.5 (1M context)").
var modelNameRe = regexp.MustCompile(`(?i)Co-Authored-By:\s*Claude\s+(.+?)\s*<`)

// IsClaudeCoAuthorLine checks if a single line matches the Claude co-author pattern.
func IsClaudeCoAuthorLine(line string) bool {
	return ClaudeCoAuthorRe.MatchString(line)
}

// ContainsClaudeCoAuthor checks if a commit message contains a Claude co-author line.
func ContainsClaudeCoAuthor(message string) bool {
	return ClaudeCoAuthorRe.MatchString(message)
}

// ExtractModelName returns the Claude model name from a commit message.
// e.g. "Claude Opus 4.6" from a message containing "Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>".
// Returns empty string if no match found.
func ExtractModelName(message string) string {
	for _, line := range strings.Split(message, "\n") {
		m := modelNameRe.FindStringSubmatch(line)
		if m != nil {
			return "Claude " + strings.TrimSpace(m[1])
		}
	}
	return ""
}
