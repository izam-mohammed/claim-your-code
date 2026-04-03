package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func initGitDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestIsGitRepo(t *testing.T) {
	dir := t.TempDir()

	if IsGitRepo(dir) {
		t.Error("empty dir should not be a git repo")
	}

	initGitDir(t, dir)

	if !IsGitRepo(dir) {
		t.Error("dir with .git should be a git repo")
	}
}

func TestIsGitRepoFile(t *testing.T) {
	dir := t.TempDir()
	// .git as a file (not directory) should not count
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsGitRepo(dir) {
		t.Error(".git as a file should not be detected as git repo")
	}
}

func TestFindReposSimple(t *testing.T) {
	root := t.TempDir()

	// Create two repos at depth 1
	initGitDir(t, filepath.Join(root, "repo-a"))
	initGitDir(t, filepath.Join(root, "repo-b"))

	// Create a non-repo dir
	os.MkdirAll(filepath.Join(root, "not-a-repo"), 0o755)

	repos := FindRepos(root, 2)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	names := map[string]bool{}
	for _, r := range repos {
		names[r.Name] = true
	}
	if !names["repo-a"] || !names["repo-b"] {
		t.Errorf("expected repo-a and repo-b, got %v", names)
	}
}

func TestFindReposDepthLimit(t *testing.T) {
	root := t.TempDir()

	// Create a repo at depth 3
	deepPath := filepath.Join(root, "a", "b", "c")
	initGitDir(t, deepPath)

	// maxDepth=2 should NOT find it
	repos := FindRepos(root, 2)
	if len(repos) != 0 {
		t.Errorf("expected 0 repos at maxDepth=2, got %d", len(repos))
	}

	// maxDepth=3 should find it
	repos = FindRepos(root, 3)
	if len(repos) != 1 {
		t.Errorf("expected 1 repo at maxDepth=3, got %d", len(repos))
	}
}

func TestFindReposStopsAtGit(t *testing.T) {
	root := t.TempDir()

	// Create a repo with a nested repo inside
	initGitDir(t, filepath.Join(root, "outer"))
	initGitDir(t, filepath.Join(root, "outer", "inner"))

	repos := FindRepos(root, 4)
	if len(repos) != 1 {
		t.Errorf("should stop at outer repo, expected 1, got %d", len(repos))
	}
	if repos[0].Name != "outer" {
		t.Errorf("expected 'outer', got %q", repos[0].Name)
	}
}

func TestFindReposSkipsDirs(t *testing.T) {
	root := t.TempDir()

	// Create repos inside skip dirs — should be ignored
	initGitDir(t, filepath.Join(root, "node_modules", "some-pkg"))
	initGitDir(t, filepath.Join(root, "vendor", "dep"))
	initGitDir(t, filepath.Join(root, ".cache", "thing"))

	// Create a valid repo
	initGitDir(t, filepath.Join(root, "my-project"))

	repos := FindRepos(root, 3)
	if len(repos) != 1 {
		t.Errorf("expected 1 repo (skip dirs ignored), got %d", len(repos))
	}
	if repos[0].Name != "my-project" {
		t.Errorf("expected 'my-project', got %q", repos[0].Name)
	}
}

func TestFindReposSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()

	// Hidden dir (not .git) should be skipped
	initGitDir(t, filepath.Join(root, ".hidden", "repo"))

	// Visible repo should be found
	initGitDir(t, filepath.Join(root, "visible"))

	repos := FindRepos(root, 3)
	if len(repos) != 1 {
		t.Errorf("expected 1 repo (hidden dir skipped), got %d", len(repos))
	}
}

func TestFindReposRootIsRepo(t *testing.T) {
	root := t.TempDir()
	initGitDir(t, root)

	repos := FindRepos(root, 2)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo when root is a repo, got %d", len(repos))
	}
	if repos[0].Path != root {
		t.Errorf("expected root path, got %q", repos[0].Path)
	}
}

func TestFindReposEmpty(t *testing.T) {
	root := t.TempDir()

	repos := FindRepos(root, 4)
	if len(repos) != 0 {
		t.Errorf("expected 0 repos in empty dir, got %d", len(repos))
	}
}

func TestFindNestedRepos(t *testing.T) {
	root := t.TempDir()
	initGitDir(t, root) // root is itself a repo

	// Create nested repos
	initGitDir(t, filepath.Join(root, "packages", "lib-a"))
	initGitDir(t, filepath.Join(root, "packages", "lib-b"))

	nested := FindNestedRepos(root)
	if len(nested) != 2 {
		t.Fatalf("expected 2 nested repos, got %d", len(nested))
	}

	names := map[string]bool{}
	for _, r := range nested {
		names[r.Name] = true
	}
	if !names[filepath.Join("packages", "lib-a")] || !names[filepath.Join("packages", "lib-b")] {
		t.Errorf("unexpected nested repo names: %v", names)
	}
}

func TestFindNestedReposNone(t *testing.T) {
	root := t.TempDir()
	initGitDir(t, root)

	nested := FindNestedRepos(root)
	if len(nested) != 0 {
		t.Errorf("expected 0 nested repos, got %d", len(nested))
	}
}

func TestFindNestedReposSkipsDirs(t *testing.T) {
	root := t.TempDir()
	initGitDir(t, root)

	// Repo inside node_modules should be skipped
	initGitDir(t, filepath.Join(root, "node_modules", "pkg"))

	// Valid nested repo
	initGitDir(t, filepath.Join(root, "sub-project"))

	nested := FindNestedRepos(root)
	if len(nested) != 1 {
		t.Errorf("expected 1 nested repo, got %d", len(nested))
	}
}

func TestFindNestedReposDepthLimit(t *testing.T) {
	root := t.TempDir()
	initGitDir(t, root)

	// Repo at depth 5 — beyond the 3-level limit
	deepPath := filepath.Join(root, "a", "b", "c", "d", "e")
	initGitDir(t, deepPath)

	nested := FindNestedRepos(root)
	if len(nested) != 0 {
		t.Errorf("expected 0 nested repos beyond depth limit, got %d", len(nested))
	}

	// Repo at depth 2 — well within limit
	initGitDir(t, filepath.Join(root, "x", "y"))
	nested = FindNestedRepos(root)
	if len(nested) != 1 {
		t.Errorf("expected 1 nested repo within depth limit, got %d", len(nested))
	}
}

func TestFindNestedReposStopsAtGit(t *testing.T) {
	root := t.TempDir()
	initGitDir(t, root)

	// Nested repo with its own nested repo
	initGitDir(t, filepath.Join(root, "outer-sub"))
	initGitDir(t, filepath.Join(root, "outer-sub", "inner"))

	nested := FindNestedRepos(root)
	// Should find outer-sub but not inner (stops at first .git)
	if len(nested) != 1 {
		t.Errorf("expected 1 nested repo (stop at .git), got %d", len(nested))
	}
	if nested[0].Name != "outer-sub" {
		t.Errorf("expected 'outer-sub', got %q", nested[0].Name)
	}
}
