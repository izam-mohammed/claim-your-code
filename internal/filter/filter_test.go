package filter

import "testing"

func TestStripClaudeCoAuthor(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "full commit message with co-author",
			input: `Format outbound call metadata JSON as structured user_details prompt
When metadata is a JSON dict, format it as a <user_details> XML block
with <important> and <details> tags so the agent treats caller information
as critical context. Fall back to raw string if metadata is not a dict.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
`,
			want: `Format outbound call metadata JSON as structured user_details prompt
When metadata is a JSON dict, format it as a <user_details> XML block
with <important> and <details> tags so the agent treats caller information
as critical context. Fall back to raw string if metadata is not a dict.
`,
		},
		{
			name: "message with context note variant",
			input: `Fix bug

Co-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>
`,
			want: "Fix bug\n",
		},
		{
			name:  "message without co-author",
			input: "Simple commit message\n",
			want:  "Simple commit message\n",
		},
		{
			name: "message with non-claude co-author preserved",
			input: `Fix bug

Co-Authored-By: John Doe <john@example.com>
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
`,
			want: `Fix bug

Co-Authored-By: John Doe <john@example.com>
`,
		},
		{
			name: "multiple trailing blank lines cleaned",
			input: `Fix bug


Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>

`,
			want: "Fix bug\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripClaudeCoAuthor(tt.input)
			if got != tt.want {
				t.Errorf("StripClaudeCoAuthor():\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}
