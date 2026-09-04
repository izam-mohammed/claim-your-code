package rewriter

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubFilter installs a message filter that strips Claude co-author trailers,
// standing in for the real claim binary that RewriteBranches shells out to.
func stubFilter(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub filter is a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "claim-stub")
	body := "#!/bin/sh\nsed '/[Cc]o-[Aa]uthored-[Bb]y: Claude/d'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := executable
	executable = func() (string, error) { return script, nil }
	t.Cleanup(func() { executable = prev })
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

const claudeTrailer = "Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"

func TestRewriteStripsTrailersOnAllBranches(t *testing.T) {
	stubFilter(t)
	repo := initTestRepo(t)
	addCommit(t, repo, "a.txt", "feat: one\n\n"+claudeTrailer)
	addCommit(t, repo, "b.txt", "feat: clean")

	gitOut(t, repo, "checkout", "-b", "feature")
	addCommit(t, repo, "c.txt", "feat: on branch\n\n"+claudeTrailer)

	before, err := GetBranchRefs(repo)
	if err != nil {
		t.Fatal(err)
	}

	if err := Rewrite(repo); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}

	log := gitOut(t, repo, "log", "--all", "--format=%B")
	if strings.Contains(log, "anthropic.com") {
		t.Errorf("Claude trailers survived the rewrite:\n%s", log)
	}
	for _, subject := range []string{"feat: one", "feat: clean", "feat: on branch"} {
		if !strings.Contains(log, subject) {
			t.Errorf("commit subject %q was lost in the rewrite", subject)
		}
	}

	after, err := GetBranchRefs(repo)
	if err != nil {
		t.Fatal(err)
	}
	changed := ChangedBranches(before, after)
	if len(changed) != 2 {
		t.Errorf("ChangedBranches = %v, want both branches rewritten", changed)
	}
}

func TestRewriteRemovesFilterBranchBackupRefs(t *testing.T) {
	stubFilter(t)
	repo := initTestRepo(t)
	addCommit(t, repo, "a.txt", "feat: one\n\n"+claudeTrailer)

	if err := Rewrite(repo); err != nil {
		t.Fatal(err)
	}
	if refs := gitOut(t, repo, "for-each-ref", "--format=%(refname)", "refs/original/"); refs != "" {
		t.Errorf("refs/original/ still holds %q — backup refs should be cleaned up", refs)
	}
}

func TestRewriteBranchesOnlyTouchesNamedBranch(t *testing.T) {
	stubFilter(t)
	repo := initTestRepo(t)
	addCommit(t, repo, "base.txt", "feat: base")
	mainName := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")

	gitOut(t, repo, "checkout", "-b", "other")
	addCommit(t, repo, "other.txt", "feat: other\n\n"+claudeTrailer)
	gitOut(t, repo, "checkout", mainName)
	addCommit(t, repo, "main.txt", "feat: main\n\n"+claudeTrailer)

	before, _ := GetBranchRefs(repo)
	if err := RewriteBranches(repo, []string{mainName}); err != nil {
		t.Fatalf("RewriteBranches: %v", err)
	}
	after, _ := GetBranchRefs(repo)

	if after[mainName] == before[mainName] {
		t.Error("the named branch was not rewritten")
	}
	if after["other"] != before["other"] {
		t.Error("branch \"other\" changed, but only the named branch should have been rewritten")
	}
	if !strings.Contains(gitOut(t, repo, "log", "other", "--format=%B"), "anthropic.com") {
		t.Error("the untouched branch lost its trailer")
	}
}

func TestRewriteBranchesFailsWhenBinaryUnavailable(t *testing.T) {
	repo := initTestRepo(t)
	addCommit(t, repo, "a.txt", "feat: one")

	prev := executable
	executable = func() (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { executable = prev })

	err := RewriteBranches(repo, nil)
	if err == nil {
		t.Fatal("RewriteBranches succeeded without a filter binary")
	}
	if !strings.Contains(err.Error(), "failed to find claim binary") {
		t.Errorf("error = %v, want it to name the missing binary", err)
	}
}

func TestRewriteBranchesFailsOnBadRepo(t *testing.T) {
	stubFilter(t)
	if err := RewriteBranches(t.TempDir(), nil); err == nil {
		t.Fatal("RewriteBranches succeeded outside a git repository")
	}
}

func TestResolveRefPrefersLocalBranch(t *testing.T) {
	repo := initTestRepo(t)
	addCommit(t, repo, "a.txt", "feat: one")
	branch := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")

	if got := resolveRef(repo, branch); got != "refs/heads/"+branch {
		t.Errorf("resolveRef(%q) = %q, want the local ref", branch, got)
	}
}

func TestResolveRefFallsBackToRemoteTracking(t *testing.T) {
	repo := initTestRepo(t)
	addCommit(t, repo, "a.txt", "feat: one")
	head := gitOut(t, repo, "rev-parse", "HEAD")
	gitOut(t, repo, "update-ref", "refs/remotes/origin/only-remote", head)

	if got := resolveRef(repo, "only-remote"); got != "refs/remotes/origin/only-remote" {
		t.Errorf("resolveRef = %q, want the remote-tracking ref", got)
	}
}

func TestResolveRefUnknownBranchFallsBackToLocalPath(t *testing.T) {
	repo := initTestRepo(t)
	addCommit(t, repo, "a.txt", "feat: one")
	if got := resolveRef(repo, "ghost"); got != "refs/heads/ghost" {
		t.Errorf("resolveRef(ghost) = %q, want the local ref path as a fallback", got)
	}
}

func TestGetRemoteURL(t *testing.T) {
	repo := initTestRepo(t)
	if got := GetRemoteURL(repo); got != "" {
		t.Errorf("GetRemoteURL = %q for a repo with no remote, want empty", got)
	}

	gitOut(t, repo, "remote", "add", "origin", "https://github.com/o/r.git")
	if got := GetRemoteURL(repo); got != "https://github.com/o/r.git" {
		t.Errorf("GetRemoteURL = %q, want the origin URL", got)
	}
}

func TestGetRemoteURLOutsideRepo(t *testing.T) {
	if got := GetRemoteURL(t.TempDir()); got != "" {
		t.Errorf("GetRemoteURL = %q outside a repo, want empty", got)
	}
}

func TestChangedBranches(t *testing.T) {
	before := map[string]string{"main": "aaa", "stable": "bbb", "gone": "ccc"}
	after := map[string]string{"main": "zzz", "stable": "bbb", "added": "ddd"}

	changed := ChangedBranches(before, after)
	if len(changed) != 1 || changed[0] != "main" {
		t.Errorf("ChangedBranches = %v, want only [main] — unchanged, removed and new branches do not count", changed)
	}
}

func TestChangedBranchesEmpty(t *testing.T) {
	if got := ChangedBranches(nil, nil); len(got) != 0 {
		t.Errorf("ChangedBranches(nil, nil) = %v, want empty", got)
	}
}

func TestPushBranchesPushesOnlyRewrittenBranches(t *testing.T) {
	stubFilter(t)

	// A bare repo to act as origin, and a clone to rewrite in.
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	gitOut(t, seed, "init", "-b", "main")
	gitOut(t, seed, "config", "user.email", "test@example.com")
	gitOut(t, seed, "config", "user.name", "Test")
	addCommit(t, seed, "a.txt", "feat: one\n\n"+claudeTrailer)
	gitOut(t, seed, "branch", "untouched")

	origin := filepath.Join(root, "origin.git")
	gitOut(t, root, "clone", "--bare", "--quiet", seed, origin)

	work := filepath.Join(root, "work")
	gitOut(t, root, "clone", "--quiet", origin, work)
	gitOut(t, work, "config", "user.email", "test@example.com")
	gitOut(t, work, "config", "user.name", "Test")

	before, _ := GetBranchRefs(work)
	if err := RewriteBranches(work, []string{"main"}); err != nil {
		t.Fatal(err)
	}
	after, _ := GetBranchRefs(work)

	originMainBefore := gitOut(t, origin, "rev-parse", "main")
	originUntouchedBefore := gitOut(t, origin, "rev-parse", "untouched")

	if err := PushBranches(work, before, after); err != nil {
		t.Fatalf("PushBranches: %v", err)
	}

	if got := gitOut(t, origin, "rev-parse", "main"); got != after["main"] {
		t.Errorf("origin main = %s, want the rewritten tip %s", got, after["main"])
	}
	if originMainBefore == after["main"] {
		t.Error("the rewrite did not actually change main, so this test proves nothing")
	}
	if got := gitOut(t, origin, "rev-parse", "untouched"); got != originUntouchedBefore {
		t.Error("an unchanged branch was pushed")
	}
}

func TestPushBranchesSkipsUnchangedAndMissing(t *testing.T) {
	repo := initTestRepo(t)
	// No remote configured — if it tried to push, this would fail.
	before := map[string]string{"main": "aaa", "dropped": "bbb"}
	after := map[string]string{"main": "aaa"}
	if err := PushBranches(repo, before, after); err != nil {
		t.Errorf("PushBranches = %v, want nil — nothing changed, so nothing should be pushed", err)
	}
}

func TestPushBranchesReportsFailure(t *testing.T) {
	repo := initTestRepo(t)
	addCommit(t, repo, "a.txt", "feat: one")
	head := gitOut(t, repo, "rev-parse", "HEAD")

	// origin points nowhere, so the push must fail.
	gitOut(t, repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git"))

	err := PushBranches(repo, map[string]string{"main": "0000000000000000000000000000000000000000"}, map[string]string{"main": head})
	if err == nil {
		t.Fatal("PushBranches succeeded against a missing remote")
	}
	if !strings.Contains(err.Error(), "failed to push branch main") {
		t.Errorf("error = %v, want it to name the branch", err)
	}
}

func TestSplitLinesNoTrailingNewline(t *testing.T) {
	got := splitLines("a\nb\nc")
	if len(got) != 3 || got[2] != "c" {
		t.Errorf("splitLines = %q, want the final unterminated line included", got)
	}
}

func TestSplitLinesEmpty(t *testing.T) {
	if got := splitLines(""); len(got) != 0 {
		t.Errorf("splitLines(\"\") = %q, want empty", got)
	}
}

func TestSetExecutableOverridesAndRestores(t *testing.T) {
	original, err := executable()
	if err != nil {
		t.Fatal(err)
	}

	restore := SetExecutable("/tmp/stub-binary")
	got, err := executable()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/stub-binary" {
		t.Errorf("executable() = %q, want the override", got)
	}

	restore()
	got, err = executable()
	if err != nil {
		t.Fatal(err)
	}
	if got != original {
		t.Errorf("executable() = %q after restore, want %q", got, original)
	}
}
