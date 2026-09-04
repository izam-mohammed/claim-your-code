// The remote flows that actually clone: exercised against local bare
// repositories rather than GitHub.

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	githubpkg "github.com/izam-mohammed/claim-your-code/internal/github"
	"github.com/izam-mohammed/claim-your-code/internal/remote"
	"github.com/izam-mohammed/claim-your-code/internal/report"
)

// fakeGitHub creates <root>/<owner>/<repo>.git as a bare repo holding one
// Claude-co-authored commit, and points cloneBase at <root> so the remote
// flows clone it instead of reaching github.com. It returns the origin path.
func fakeGitHub(t *testing.T, owner, repo string, extraBranch string) string {
	t.Helper()
	root := t.TempDir()

	seed := newRepo(t, filepath.Join(root, "seed"))
	commit(t, seed, "clean.txt", "feat: a clean commit")
	commit(t, seed, "dirty.txt", "feat: a claimed commit\n\n"+claudeTrailer)
	if extraBranch != "" {
		git(t, seed, "branch", extraBranch)
	}

	origin := filepath.Join(root, owner, repo+".git")
	git(t, root, "clone", "--bare", "--quiet", seed, origin)
	// A bare repo refuses a push to its checked-out branch unless told otherwise.
	git(t, origin, "config", "receive.denyCurrentBranch", "ignore")

	prev := cloneBase
	cloneBase = root
	t.Cleanup(func() { cloneBase = prev })

	return origin
}

func TestCloneURLForBuildsAnAuthenticatedGitHubURL(t *testing.T) {
	got := cloneURLFor(githubpkg.NewClient("ghp_x"), "owner", "repo")
	want := "https://x-access-token:ghp_x@github.com/owner/repo.git"
	if got != want {
		t.Errorf("cloneURLFor() = %q, want %q", got, want)
	}
}

func TestCloneURLForPublicClient(t *testing.T) {
	got := cloneURLFor(githubpkg.NewPublicClient(), "owner", "repo")
	if got != "https://github.com/owner/repo.git" {
		t.Errorf("cloneURLFor() = %q", got)
	}
}

func TestCloneToTempClonesAndCleans(t *testing.T) {
	fakeGitHub(t, "owner", "repo", "")

	var path string
	var cleanup func()
	runMain(t, []string{"claim"}, func() {
		path, cleanup = cloneToTemp(githubpkg.NewPublicClient(), "owner", "repo")
	})
	if path == "" {
		t.Fatal("cloneToTemp returned no path")
	}
	if !strings.Contains(git(t, path, "log", "--format=%B"), "anthropic.com") {
		t.Error("the clone does not contain the seeded commit")
	}
	cleanup()
}

func TestRunRemoteRepoScanOnlyByDefault(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, err := remote.ParseRepo("owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo"}, func() {
		runRemoteRepoWithTarget(target)
	})

	for _, want := range []string{
		"1 commit(s) co-authored by Claude",
		"Report tracked",
		"scan-only run",
		"--apply",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q\n---\n%s", want, out)
		}
	}

	reports, _ := report.ListAll()
	if len(reports) != 1 || reports[0].Result.Status != "dry_run" {
		t.Errorf("reports = %+v, want one dry_run report", reports)
	}
	if reports[0].RemoteOwner != "owner" || reports[0].RemoteRepo != "repo" {
		t.Errorf("remote metadata = %+v", reports[0])
	}
}

func TestRunRemoteRepoDryRun(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, _ := remote.ParseRepo("owner/repo")
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo", "--dry-run"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !strings.Contains(out, "No changes made") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemoteRepoCleanRepository(t *testing.T) {
	isolateData(t)
	root := t.TempDir()
	seed := newRepo(t, filepath.Join(root, "seed"))
	commit(t, seed, "a.txt", "feat: nothing to claim")
	git(t, root, "clone", "--bare", "--quiet", seed, filepath.Join(root, "owner", "repo.git"))
	prev := cloneBase
	cloneBase = root
	t.Cleanup(func() { cloneBase = prev })

	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, _ := remote.ParseRepo("owner/repo")
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !strings.Contains(out, "No Claude co-authorship found") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemoteRepoApplyRewritesAndOffersAPush(t *testing.T) {
	isolateData(t)
	origin := fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})
	stubToken(t, "", nil)
	stubFilter(t)
	originBefore := git(t, origin, "rev-parse", "main")

	stubPrompts(t, prompts{
		confirm:   func(string) bool { return true },
		dangerous: func(string) bool { return true },
	})

	target, _ := remote.ParseRepo("owner/repo")
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo", "--apply"}, func() {
		runRemoteRepoWithTarget(target)
	})

	if !strings.Contains(out, "Cleaned") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "Pushed") {
		t.Errorf("the push was not reported:\n%s", out)
	}
	if git(t, origin, "rev-parse", "main") == originBefore {
		t.Error("origin was not updated by the push")
	}
	if strings.Contains(git(t, origin, "log", "main", "--format=%B"), "anthropic.com") {
		t.Error("the pushed history still contains a Claude trailer")
	}
}

func TestRunRemoteRepoApplyDeclinedRewrite(t *testing.T) {
	isolateData(t)
	origin := fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})
	stubToken(t, "", nil)
	before := git(t, origin, "rev-parse", "main")

	stubPrompts(t, prompts{confirm: func(string) bool { return false }})

	target, _ := remote.ParseRepo("owner/repo")
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo", "--apply"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !strings.Contains(out, "Aborted") {
		t.Errorf("output = %q", out)
	}
	if git(t, origin, "rev-parse", "main") != before {
		t.Error("origin changed even though the rewrite was declined")
	}
	reports, _ := report.ListAll()
	if len(reports) != 1 || reports[0].Result.Status != "aborted" {
		t.Errorf("reports = %+v, want one aborted report", reports)
	}
}

func TestRunRemoteRepoApplyPushDeclined(t *testing.T) {
	isolateData(t)
	origin := fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})
	stubToken(t, "", nil)
	stubFilter(t)
	before := git(t, origin, "rev-parse", "main")

	stubPrompts(t, prompts{dangerous: func(string) bool { return false }})

	target, _ := remote.ParseRepo("owner/repo")
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo", "--apply", "--force"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !strings.Contains(out, "Push skipped") {
		t.Errorf("output = %q", out)
	}
	if git(t, origin, "rev-parse", "main") != before {
		t.Error("origin was pushed even though the push was declined")
	}
}

func TestRunRemoteRepoChecksOutAnExplicitBranch(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "release")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, err := remote.ParseURL("https://github.com/owner/repo/tree/release")
	if err != nil {
		t.Fatal(err)
	}
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !strings.Contains(out, "Checking out branch") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemoteRepoMissingBranchFails(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, _ := remote.ParseURL("https://github.com/owner/repo/tree/no-such-branch")
	_, code, exited := runMain(t, []string{"claim", "repo", "owner/repo"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !exited || code != 1 {
		t.Errorf("checking out a missing branch should exit, got (%d, %v)", code, exited)
	}
}

func TestRunRemoteRepoPrivateRepoAuthenticates(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", true))) // private
	})
	stubToken(t, "", nil) // returns a public client, but the path is exercised

	target, _ := remote.ParseRepo("owner/repo")
	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo"}, func() {
		runRemoteRepoWithTarget(target)
	})
	if !strings.Contains(out, "Authenticating with GitHub") {
		t.Errorf("a private repo should trigger authentication:\n%s", out)
	}
}

func TestRunRemotePRScanOnly(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "feature")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":42,"title":"a change","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, err := remote.ParsePR("owner/repo#42")
	if err != nil {
		t.Fatal(err)
	}
	out, _, _ := runMain(t, []string{"claim", "pr", "owner/repo#42"}, func() {
		runRemotePRWithTarget(target)
	})

	for _, want := range []string{"a change", "feature", "Report tracked", "scan-only run"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q\n---\n%s", want, out)
		}
	}
	reports, _ := report.ListAll()
	if len(reports) != 1 || reports[0].PRNumber != 42 {
		t.Errorf("reports = %+v, want the PR number recorded", reports)
	}
}

func TestRunRemotePRDryRun(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "feature")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":42,"title":"t","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})

	stubToken(t, "", nil)

	target, _ := remote.ParsePR("owner/repo#42")
	out, _, _ := runMain(t, []string{"claim", "pr", "owner/repo#42", "--dry-run", "--apply"}, func() {
		runRemotePRWithTarget(target)
	})
	if !strings.Contains(out, "No changes made") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemotePRCleanBranch(t *testing.T) {
	isolateData(t)
	root := t.TempDir()
	seed := newRepo(t, filepath.Join(root, "seed"))
	commit(t, seed, "a.txt", "feat: clean")
	git(t, seed, "branch", "feature")
	git(t, root, "clone", "--bare", "--quiet", seed, filepath.Join(root, "owner", "repo.git"))
	prev := cloneBase
	cloneBase = root
	t.Cleanup(func() { cloneBase = prev })

	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":7,"title":"t","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, _ := remote.ParsePR("owner/repo#7")
	out, _, _ := runMain(t, []string{"claim", "pr", "owner/repo#7"}, func() {
		runRemotePRWithTarget(target)
	})
	if !strings.Contains(out, "No Claude co-authorship found in PR #7") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemotePRApplyRewritesAndPushes(t *testing.T) {
	isolateData(t)
	origin := fakeGitHub(t, "owner", "repo", "feature")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":42,"title":"t","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})
	stubToken(t, "", nil)
	stubFilter(t)
	before := git(t, origin, "rev-parse", "feature")

	stubPrompts(t, prompts{
		confirm:   func(string) bool { return true },
		dangerous: func(string) bool { return true },
	})

	target, _ := remote.ParsePR("owner/repo#42")
	out, _, _ := runMain(t, []string{"claim", "pr", "owner/repo#42", "--apply"}, func() {
		runRemotePRWithTarget(target)
	})

	if !strings.Contains(out, "Cleaned") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "Pushed branch feature") {
		t.Errorf("the branch push was not reported:\n%s", out)
	}
	if git(t, origin, "rev-parse", "feature") == before {
		t.Error("the PR branch was not updated on origin")
	}
}

func TestRunRemotePRApplyDeclinedRewrite(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "feature")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":42,"title":"t","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})
	stubToken(t, "", nil)
	stubPrompts(t, prompts{confirm: func(string) bool { return false }})

	target, _ := remote.ParsePR("owner/repo#42")
	out, _, _ := runMain(t, []string{"claim", "pr", "owner/repo#42", "--apply"}, func() {
		runRemotePRWithTarget(target)
	})
	if !strings.Contains(out, "Aborted") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRemotePRApplyPushDeclined(t *testing.T) {
	isolateData(t)
	origin := fakeGitHub(t, "owner", "repo", "feature")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":42,"title":"t","head":{"ref":"feature","sha":"abc"},"base":{"ref":"main"}}`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})
	stubToken(t, "", nil)
	stubFilter(t)
	before := git(t, origin, "rev-parse", "feature")

	stubPrompts(t, prompts{dangerous: func(string) bool { return false }})

	target, _ := remote.ParsePR("owner/repo#42")
	out, _, _ := runMain(t, []string{"claim", "pr", "owner/repo#42", "--apply", "--force"}, func() {
		runRemotePRWithTarget(target)
	})
	if !strings.Contains(out, "Push skipped") {
		t.Errorf("output = %q", out)
	}
	if git(t, origin, "rev-parse", "feature") != before {
		t.Error("the branch was pushed even though the push was declined")
	}
}

func TestRunRemotePRMissingHeadBranchFails(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":42,"title":"t","head":{"ref":"ghost","sha":"abc"},"base":{"ref":"main"}}`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})

	target, _ := remote.ParsePR("owner/repo#42")
	_, code, exited := runMain(t, []string{"claim", "pr", "owner/repo#42"}, func() {
		runRemotePRWithTarget(target)
	})
	if !exited || code != 1 {
		t.Errorf("a missing head branch should exit, got (%d, %v)", code, exited)
	}
}

func TestCloneAndScanConcurrentRecordsAReportPerRepo(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "acme", "dirty", "")

	// A second, clean repository under the same fake host.
	root := cloneBase
	seed := newRepo(t, filepath.Join(root, "seed-clean"))
	commit(t, seed, "a.txt", "feat: clean")
	git(t, root, "clone", "--bare", "--quiet", seed, filepath.Join(root, "acme", "clean.git"))

	out, _, _ := runMain(t, []string{"claim"}, func() {
		cloneAndScanConcurrent(githubpkg.NewPublicClient(), []githubpkg.RepoInfo{
			{Owner: "acme", Name: "dirty", DefaultBranch: "main"},
			{Owner: "acme", Name: "clean", DefaultBranch: "main"},
		})
	})

	if !strings.Contains(out, "acme/dirty") {
		t.Errorf("the affected repo is missing:\n%s", out)
	}
	if !strings.Contains(out, "1 repo(s) clean") {
		t.Errorf("the clean count is missing:\n%s", out)
	}

	reports, _ := report.ListAll()
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want one for the affected repo only", len(reports))
	}
	r := reports[0]
	if r.RemoteOwner != "acme" || r.RemoteRepo != "dirty" {
		t.Errorf("report metadata = %+v", r)
	}
	if r.Result.Status != "dry_run" {
		t.Errorf("multi-repo scans should record dry_run, got %q", r.Result.Status)
	}
}

func TestRunRemoteMultiRepoCloneScan(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "acme", "one", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"one","owner":{"login":"acme"},"default_branch":"main"}]`))
	})

	answers := []string{"all", "clone"}
	i := 0
	stubPrompts(t, prompts{selectOne: func(string, []huh.Option[string], int) (string, error) {
		a := answers[i]
		i++
		return a, nil
	}})

	out, _, _ := runMain(t, []string{"claim", "org", "acme"}, func() {
		runRemoteMultiRepo(githubpkg.NewClient("tok"), "acme", true)
	})
	if !strings.Contains(out, "acme/one") {
		t.Errorf("the cloned scan did not report the repo:\n%s", out)
	}
}

func TestCloneAndScanConcurrentCleansUpAfterAFailedClone(t *testing.T) {
	isolateData(t)
	// A clone base with nothing behind it, so every clone fails.
	prev := cloneBase
	cloneBase = filepath.Join(t.TempDir(), "nothing-here")
	t.Cleanup(func() { cloneBase = prev })

	before := countClaimTempDirs(t)

	runMain(t, []string{"claim"}, func() {
		cloneAndScanConcurrent(githubpkg.NewPublicClient(), []githubpkg.RepoInfo{
			{Owner: "acme", Name: "one", DefaultBranch: "main"},
			{Owner: "acme", Name: "two", DefaultBranch: "main"},
		})
	})

	if after := countClaimTempDirs(t); after > before {
		t.Errorf("failed clones leaked %d temp directories", after-before)
	}
}

// countClaimTempDirs counts the claim-* directories left in the temp dir.
func countClaimTempDirs(t *testing.T) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "claim-*"))
	if err != nil {
		t.Fatal(err)
	}
	return len(matches)
}
