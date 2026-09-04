package github

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git runs a git command in dir, failing the test if it errors.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// originRepo builds a bare repo (the "remote") seeded with one commit on main
// and one extra branch, and returns its path.
func originRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, work, "init", "-b", "main")
	git(t, work, "config", "user.email", "test@example.com")
	git(t, work, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", "a.txt")
	git(t, work, "commit", "-m", "initial")
	git(t, work, "branch", "feature")

	bare := filepath.Join(root, "origin.git")
	git(t, root, "clone", "--bare", "--quiet", work, bare)
	return bare
}

func TestCreateAndCleanupTempDir(t *testing.T) {
	dir, err := CreateTempDir()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("CreateTempDir did not create a directory: %v", err)
	}
	if !strings.Contains(filepath.Base(dir), "claim-") {
		t.Errorf("temp dir %q should be recognisable as claim's", dir)
	}

	CleanupTempDir(dir)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("CleanupTempDir left %q behind", dir)
	}
}

func TestCleanupTempDirIsSafeOnMissingPath(t *testing.T) {
	CleanupTempDir(filepath.Join(t.TempDir(), "never-existed")) // must not panic
}

func TestClone(t *testing.T) {
	origin := originRepo(t)
	parent := t.TempDir()

	path, err := Clone(origin, parent)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if filepath.Base(path) != "origin" {
		t.Errorf("clone dir = %q, want the .git suffix stripped from the name", filepath.Base(path))
	}
	if _, err := os.Stat(filepath.Join(path, "a.txt")); err != nil {
		t.Errorf("clone has no working tree: %v", err)
	}
}

func TestCloneKeepsNameWithoutGitSuffix(t *testing.T) {
	origin := originRepo(t)
	plain := filepath.Join(t.TempDir(), "plainname")
	if err := os.Rename(origin, plain); err != nil {
		t.Fatal(err)
	}

	path, err := Clone(plain, t.TempDir())
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if filepath.Base(path) != "plainname" {
		t.Errorf("clone dir = %q, want %q", filepath.Base(path), "plainname")
	}
}

func TestCloneFails(t *testing.T) {
	_, err := Clone(filepath.Join(t.TempDir(), "no-such-repo.git"), t.TempDir())
	if err == nil {
		t.Fatal("Clone succeeded for a URL that does not exist")
	}
	if !strings.Contains(err.Error(), "git clone failed") {
		t.Errorf("error = %v, want it to say the clone failed", err)
	}
}

func TestCloneBare(t *testing.T) {
	origin := originRepo(t)
	path, err := CloneBare(origin, t.TempDir())
	if err != nil {
		t.Fatalf("CloneBare: %v", err)
	}
	if filepath.Base(path) != "origin.git" {
		t.Errorf("bare clone dir = %q, want it to end in .git", filepath.Base(path))
	}
	if _, err := os.Stat(filepath.Join(path, "a.txt")); !os.IsNotExist(err) {
		t.Error("a bare clone must not have a working tree")
	}
	if got := git(t, path, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Errorf("is-bare-repository = %q, want true", got)
	}
}

func TestCloneBareFails(t *testing.T) {
	if _, err := CloneBare(filepath.Join(t.TempDir(), "nope.git"), t.TempDir()); err == nil {
		t.Fatal("CloneBare succeeded for a URL that does not exist")
	}
}

func TestCheckout(t *testing.T) {
	origin := originRepo(t)
	path, err := Clone(origin, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := Checkout(path, "feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature" {
		t.Errorf("HEAD is on %q, want feature", got)
	}
}

func TestCheckoutMissingBranch(t *testing.T) {
	origin := originRepo(t)
	path, err := Clone(origin, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = Checkout(path, "no-such-branch")
	if err == nil {
		t.Fatal("Checkout succeeded for a branch that does not exist")
	}
	if !strings.Contains(err.Error(), "no-such-branch") {
		t.Errorf("error = %v, want it to name the branch", err)
	}
}

func TestPushForce(t *testing.T) {
	origin := originRepo(t)
	path, err := Clone(origin, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, path, "config", "user.email", "test@example.com")
	git(t, path, "config", "user.name", "Test")

	// Rewrite the branch tip so there is something to force-push.
	if err := os.WriteFile(filepath.Join(path, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, path, "commit", "-am", "amended history")
	local := git(t, path, "rev-parse", "HEAD")

	if err := PushForce(path, "main"); err != nil {
		t.Fatalf("PushForce: %v", err)
	}
	if remote := git(t, origin, "rev-parse", "main"); remote != local {
		t.Errorf("origin main = %s, want %s", remote, local)
	}
}

func TestPushForceFailsWithoutRemote(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	if err := PushForce(dir, "main"); err == nil {
		t.Fatal("PushForce succeeded with no origin configured")
	}
}

func TestAuthCloneURLEmbedsToken(t *testing.T) {
	got := NewClient("ghp_abc").AuthCloneURL("izam-mohammed", "claim-your-code")
	want := "https://x-access-token:ghp_abc@github.com/izam-mohammed/claim-your-code.git"
	if got != want {
		t.Errorf("AuthCloneURL() = %q, want %q", got, want)
	}
	if StripTokenFromURL(got) != "https://github.com/izam-mohammed/claim-your-code.git" {
		t.Errorf("StripTokenFromURL did not undo AuthCloneURL: %q", StripTokenFromURL(got))
	}
}
