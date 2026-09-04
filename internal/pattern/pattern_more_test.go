package pattern

import "testing"

func TestIsClaudeCoAuthorLineVariants(t *testing.T) {
	match := []string{
		"Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>",
		"Co-authored-by: Claude Opus 4.6 <noreply@anthropic.com>",
		"CO-AUTHORED-BY: CLAUDE OPUS 4.6 <NOREPLY@ANTHROPIC.COM>",
		"  Co-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>  ",
		"Co-Authored-By:Claude Haiku 4.5 <noreply@anthropic.com>",
		"Co-Authored-By: Claude <noreply@anthropic.com>",
		"Co-Authored-By: Claude Opus 4.6 <claude@anthropic.com>",
	}
	for _, line := range match {
		if !IsClaudeCoAuthorLine(line) {
			t.Errorf("IsClaudeCoAuthorLine(%q) = false, want true", line)
		}
	}

	noMatch := []string{
		"",
		"Co-Authored-By: A Human <human@example.com>",
		"Co-Authored-By: Claudia Smith <claudia@example.com>",
		"Co-Authored-By: Claude Opus 4.6 <noreply@example.com>",
		"Signed-off-by: Claude Opus 4.6 <noreply@anthropic.com>",
		"This mentions Co-Authored-By: Claude <noreply@anthropic.com> mid-sentence",
		"Co-Authored-By: Claude Opus 4.6 noreply@anthropic.com",
	}
	for _, line := range noMatch {
		if IsClaudeCoAuthorLine(line) {
			t.Errorf("IsClaudeCoAuthorLine(%q) = true, want false", line)
		}
	}
}

func TestContainsClaudeCoAuthorNeedsOwnLine(t *testing.T) {
	msg := "fix: thing\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>\n"
	if !ContainsClaudeCoAuthor(msg) {
		t.Error("ContainsClaudeCoAuthor = false for a message with a trailer line")
	}
	inline := "fix: mention Co-Authored-By: Claude <noreply@anthropic.com> inline\n"
	if ContainsClaudeCoAuthor(inline) {
		t.Error("ContainsClaudeCoAuthor matched text embedded mid-line")
	}
}

func TestExtractModelNameVariants(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{"opus", "x\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>", "Claude Opus 4.6"},
		{"context suffix", "x\n\nCo-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>", "Claude Sonnet 4.5 (1M context)"},
		{"lowercase trailer", "x\n\nco-authored-by: Claude Haiku 4.5 <noreply@anthropic.com>", "Claude Haiku 4.5"},
		{"first of several wins", "x\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>\nCo-Authored-By: Claude Haiku 4.5 <noreply@anthropic.com>", "Claude Opus 4.6"},
		{"no claude trailer", "just a commit\n", ""},
		{"empty message", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractModelName(tt.msg); got != tt.want {
				t.Errorf("ExtractModelName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubjectEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{"multi line", "feat: add thing\n\nbody text\n", "feat: add thing"},
		{"single line no newline", "fix: one liner", "fix: one liner"},
		{"empty", "", ""},
		{"leading newline", "\nbody", ""},
		{"unicode subject", "feat: café ☕ support\nbody", "feat: café ☕ support"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Subject(tt.msg); got != tt.want {
				t.Errorf("Subject(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}
