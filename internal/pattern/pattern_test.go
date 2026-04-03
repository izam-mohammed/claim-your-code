package pattern

import "testing"

func TestIsClaudeCoAuthorLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		match bool
	}{
		{"opus 4.6", "Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>", true},
		{"sonnet 4.5", "Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>", true},
		{"haiku 3.5", "Co-Authored-By: Claude Haiku 3.5 <noreply@anthropic.com>", true},
		{"with context note", "Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>", true},
		{"lowercase", "co-authored-by: Claude Opus 4.6 <noreply@anthropic.com>", true},
		{"leading spaces", "   Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>", true},
		{"not claude", "Co-Authored-By: John Doe <john@example.com>", false},
		{"random text", "This is a commit message", false},
		{"partial match", "Co-Authored-By: Claude", false},
		{"different domain", "Co-Authored-By: Claude Opus 4.6 <noreply@example.com>", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsClaudeCoAuthorLine(tt.line); got != tt.match {
				t.Errorf("IsClaudeCoAuthorLine(%q) = %v, want %v", tt.line, got, tt.match)
			}
		})
	}
}

func TestContainsClaudeCoAuthor(t *testing.T) {
	msg := `Fix authentication bug

Refactored the auth middleware to properly validate tokens.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>`

	if !ContainsClaudeCoAuthor(msg) {
		t.Error("expected to find Claude co-author in multi-line message")
	}

	clean := `Fix authentication bug

Refactored the auth middleware to properly validate tokens.`

	if ContainsClaudeCoAuthor(clean) {
		t.Error("did not expect to find Claude co-author in clean message")
	}
}

func TestExtractModelName(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			"opus 4.6",
			"Fix bug\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>",
			"Claude Opus 4.6",
		},
		{
			"sonnet with context",
			"Fix bug\n\nCo-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>",
			"Claude Sonnet 4.5 (1M context)",
		},
		{
			"haiku",
			"Co-Authored-By: Claude Haiku 3.5 <noreply@anthropic.com>",
			"Claude Haiku 3.5",
		},
		{
			"no match",
			"Just a regular commit message",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractModelName(tt.message)
			if got != tt.want {
				t.Errorf("ExtractModelName() = %q, want %q", got, tt.want)
			}
		})
	}
}
