package filter

import "testing"

func TestStripClaudeCoAuthorKeepsHumanTrailers(t *testing.T) {
	in := "fix: thing\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>\nCo-Authored-By: A Human <human@example.com>\n"
	want := "fix: thing\n\nCo-Authored-By: A Human <human@example.com>\n"
	if got := StripClaudeCoAuthor(in); got != want {
		t.Errorf("StripClaudeCoAuthor() = %q, want %q", got, want)
	}
}

func TestStripClaudeCoAuthorTrimsTrailingBlankLines(t *testing.T) {
	in := "fix: thing\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>\n\n\n"
	want := "fix: thing\n"
	if got := StripClaudeCoAuthor(in); got != want {
		t.Errorf("StripClaudeCoAuthor() = %q, want %q — blank lines left behind must be trimmed", got, want)
	}
}

func TestStripClaudeCoAuthorRemovesEveryClaudeTrailer(t *testing.T) {
	in := "feat: x\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>\nCo-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>\n"
	want := "feat: x\n"
	if got := StripClaudeCoAuthor(in); got != want {
		t.Errorf("StripClaudeCoAuthor() = %q, want %q", got, want)
	}
}

func TestStripClaudeCoAuthorLeavesCleanMessageIntact(t *testing.T) {
	in := "fix: nothing to strip\n\nWith a body.\n"
	if got := StripClaudeCoAuthor(in); got != in {
		t.Errorf("StripClaudeCoAuthor() = %q, want the message unchanged (%q)", got, in)
	}
}

func TestStripClaudeCoAuthorEmptyInput(t *testing.T) {
	if got := StripClaudeCoAuthor(""); got != "\n" {
		t.Errorf("StripClaudeCoAuthor(\"\") = %q, want %q — git expects a trailing newline", got, "\n")
	}
}

func TestStripClaudeCoAuthorMessageThatIsOnlyATrailer(t *testing.T) {
	in := "Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>\n"
	if got := StripClaudeCoAuthor(in); got != "\n" {
		t.Errorf("StripClaudeCoAuthor() = %q, want %q", got, "\n")
	}
}

func TestStripClaudeCoAuthorPreservesBodyBlankLines(t *testing.T) {
	in := "feat: x\n\npara one\n\npara two\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>\n"
	want := "feat: x\n\npara one\n\npara two\n"
	if got := StripClaudeCoAuthor(in); got != want {
		t.Errorf("StripClaudeCoAuthor() = %q, want %q — internal blank lines must survive", got, want)
	}
}
