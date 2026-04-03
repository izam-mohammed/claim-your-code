package rewriter

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepo creates a temporary git repo with test commits and returns the path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "test-repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")

	return repoPath
}

func addCommit(t *testing.T, repoPath, filename, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, filename), []byte(filename), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", filename)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func TestGetBranchRefs(t *testing.T) {
	repoPath := initTestRepo(t)
	addCommit(t, repoPath, "a.txt", "Initial commit")

	refs, err := GetBranchRefs(repoPath)
	if err != nil {
		t.Fatalf("GetBranchRefs() failed: %v", err)
	}

	if len(refs) == 0 {
		t.Fatal("expected at least one branch ref")
	}

	// Should have a branch (master or main)
	found := false
	for name, hash := range refs {
		if name != "" && len(hash) == 40 {
			found = true
		}
	}
	if !found {
		t.Error("expected a branch with a valid 40-char hash")
	}
}

func TestGetBranchRefsMultipleBranches(t *testing.T) {
	repoPath := initTestRepo(t)
	addCommit(t, repoPath, "a.txt", "Initial commit")

	// Create a second branch
	cmd := exec.Command("git", "branch", "feature")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch failed: %v\n%s", err, out)
	}

	refs, err := GetBranchRefs(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) < 2 {
		t.Errorf("expected at least 2 branches, got %d", len(refs))
	}
	if _, ok := refs["feature"]; !ok {
		t.Error("expected 'feature' branch in refs")
	}
}

func TestGetBranchRefsNotARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := GetBranchRefs(dir)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestRevert(t *testing.T) {
	repoPath := initTestRepo(t)
	addCommit(t, repoPath, "a.txt", "First commit")

	// Get current refs
	refs, err := GetBranchRefs(repoPath)
	if err != nil {
		t.Fatal(err)
	}

	// Add another commit to move the branch forward
	addCommit(t, repoPath, "b.txt", "Second commit")

	// Revert should restore to original refs
	if err := Revert(repoPath, refs); err != nil {
		t.Fatalf("Revert() failed: %v", err)
	}

	// Verify refs are restored
	newRefs, err := GetBranchRefs(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	for branch, hash := range refs {
		if newRefs[branch] != hash {
			t.Errorf("branch %q: expected %s, got %s", branch, hash[:8], newRefs[branch][:8])
		}
	}
}

func TestRevertInvalidRef(t *testing.T) {
	dir := t.TempDir()
	// Not a git repo at all — should fail
	err := Revert(dir, map[string]string{"main": "abc123"})
	if err == nil {
		t.Error("expected error reverting in non-git directory")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "single line no newline",
			input: "hello",
			want:  []string{"hello"},
		},
		{
			name:  "single line with newline",
			input: "hello\n",
			want:  []string{"hello"},
		},
		{
			name:  "multiple lines",
			input: "a\nb\nc\n",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "multiple lines no trailing newline",
			input: "a\nb\nc",
			want:  []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitLines(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// Note: Rewrite() is not unit-testable because it invokes os.Executable()
// (the claim binary) via git filter-branch. It is covered by integration tests
// through the full CLI workflow.
