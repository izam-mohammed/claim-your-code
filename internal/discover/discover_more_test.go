package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// mkRepo creates a directory containing a .git dir, i.e. something IsGitRepo accepts.
func mkRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestIsGitRepoMissingPath(t *testing.T) {
	if IsGitRepo(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Error("IsGitRepo returned true for a path that does not exist")
	}
}

func TestFindReposIgnoresFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mkRepo(t, filepath.Join(root, "a"))

	repos := FindRepos(root, 2)
	if len(repos) != 1 || repos[0].Name != "a" {
		t.Errorf("FindRepos = %+v, want just the repo \"a\" (plain files must be ignored)", repos)
	}
}

func TestFindReposUnreadableDirIsSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions behave differently on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root — permission bits are not enforced")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	mkRepo(t, filepath.Join(root, "visible"))
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	repos := FindRepos(root, 3)
	if len(repos) != 1 || repos[0].Name != "visible" {
		t.Errorf("FindRepos = %+v, want only \"visible\" — an unreadable dir must be skipped, not fatal", repos)
	}
}

func TestFindReposDepthZeroOnlyChecksRoot(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, filepath.Join(root, "child"))

	if repos := FindRepos(root, 0); len(repos) != 0 {
		t.Errorf("FindRepos(depth 0) = %+v, want none — the root itself is not a repo", repos)
	}

	mkRepo(t, root)
	repos := FindRepos(root, 0)
	if len(repos) != 1 || repos[0].Path != root {
		t.Errorf("FindRepos(depth 0) = %+v, want the root repo itself", repos)
	}
}

func TestFindReposEachSkipDirIsSkipped(t *testing.T) {
	for name := range skipDirs {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			mkRepo(t, filepath.Join(root, name, "inner"))
			if repos := FindRepos(root, 4); len(repos) != 0 {
				t.Errorf("FindRepos descended into %q and found %+v", name, repos)
			}
		})
	}
}

func TestFindNestedReposIgnoresFiles(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if nested := FindNestedRepos(root); len(nested) != 0 {
		t.Errorf("FindNestedRepos = %+v, want none", nested)
	}
}

func TestFindNestedReposNamesAreRelative(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root)
	mkRepo(t, filepath.Join(root, "packages", "api"))

	nested := FindNestedRepos(root)
	if len(nested) != 1 {
		t.Fatalf("FindNestedRepos = %+v, want exactly one", nested)
	}
	want := filepath.Join("packages", "api")
	if nested[0].Name != want {
		t.Errorf("Name = %q, want the path relative to the outer repo (%q)", nested[0].Name, want)
	}
}

func TestFindNestedReposUnreadableDirIsSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions behave differently on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root — permission bits are not enforced")
	}
	root := t.TempDir()
	mkRepo(t, root)
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(filepath.Join(locked, "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	mkRepo(t, filepath.Join(root, "sub"))
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	nested := FindNestedRepos(root)
	if len(nested) != 1 || nested[0].Name != "sub" {
		t.Errorf("FindNestedRepos = %+v, want only \"sub\"", nested)
	}
}

func TestFindNestedReposSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root)
	mkRepo(t, filepath.Join(root, ".hidden", "inner"))
	if nested := FindNestedRepos(root); len(nested) != 0 {
		t.Errorf("FindNestedRepos = %+v, want none — hidden dirs must be skipped", nested)
	}
}
