package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScan(t *testing.T) {
	// Create a temporary git repo with test commits
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

	// Commit 1: clean
	if err := os.WriteFile(filepath.Join(repoPath, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "a.txt")
	run("git", "commit", "-m", "Clean commit")

	// Commit 2: has Claude co-author
	if err := os.WriteFile(filepath.Join(repoPath, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "b.txt")
	run("git", "commit", "-m", "Fix bug\n\nCo-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>")

	// Commit 3: has Claude co-author with context
	if err := os.WriteFile(filepath.Join(repoPath, "c.txt"), []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "c.txt")
	run("git", "commit", "-m", "Add feature\n\nCo-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>")

	results, branchSummaries, err := Scan(repoPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Check results have model and branch info
	models := map[string]bool{}
	for _, r := range results {
		if len(r.Hash) != 40 {
			t.Errorf("expected 40-char hash, got %q", r.Hash)
		}
		if r.Model == "" {
			t.Errorf("expected model name for commit %q", r.Subject)
		}
		models[r.Model] = true
		if len(r.Branches) == 0 {
			t.Errorf("expected at least one branch for commit %q", r.Subject)
		}
	}
	if !models["Claude Opus 4.6"] {
		t.Error("expected to find model 'Claude Opus 4.6'")
	}
	if !models["Claude Sonnet 4.5 (1M context)"] {
		t.Error("expected to find model 'Claude Sonnet 4.5 (1M context)'")
	}

	// Check branch summaries
	if len(branchSummaries) == 0 {
		t.Fatal("expected at least one branch summary")
	}
	found := false
	for _, bs := range branchSummaries {
		if bs.Count == 2 {
			found = true
			if len(bs.Models) != 2 {
				t.Errorf("expected 2 models in branch summary, got %d", len(bs.Models))
			}
		}
	}
	if !found {
		t.Error("expected a branch with 2 co-authored commits")
	}
}

func TestScanNotARepo(t *testing.T) {
	dir := t.TempDir()
	_, _, err := Scan(dir)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}
