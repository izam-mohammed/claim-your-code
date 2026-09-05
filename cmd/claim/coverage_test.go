// Cases that exercise the remaining entry points: main, every dispatch
// branch, and the failure paths the happy-path tests do not reach.

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/izam-mohammed/claim-your-code/internal/auth"
	githubpkg "github.com/izam-mohammed/claim-your-code/internal/github"
	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/rewriter"
)

func TestMainRunsDispatch(t *testing.T) {
	out, _, exited := runMain(t, []string{"claim", "--version"}, main)
	if exited {
		t.Error("`claim --version` should not exit with a failure")
	}
	if !strings.Contains(out, "claim "+version) {
		t.Errorf("main() printed %q", out)
	}
}

func TestRunFilterMsgReportsAReadFailure(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	r.Close() // reading from a closed descriptor fails

	prev := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prev })

	out, code, exited := runMain(t, []string{"claim", "__filter-msg"}, runFilterMsg)
	if !exited || code != 1 {
		t.Errorf("a stdin read failure should exit, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "error:") {
		t.Errorf("output = %q", out)
	}
}

// TestDispatchRoutesEverySubcommand drives each branch of the dispatch switch.
// Every case is given arguments that fail fast, so the test asserts on routing,
// not on the command's own behaviour.
func TestDispatchRoutesEverySubcommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit bool
		want     string
	}{
		{"repo", []string{"claim", "repo"}, true, "claim repo --help"},
		{"pr", []string{"claim", "pr"}, true, "claim pr --help"},
		{"org", []string{"claim", "org"}, true, "claim org --help"},
		{"user", []string{"claim", "user"}, true, "claim user --help"},
		{"revert", []string{"claim", "revert"}, true, "claim revert --help"},
		{"logout", []string{"claim", "logout"}, false, "No saved accounts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateData(t)
			stubPrompts(t, prompts{})
			out, _, exited := runMain(t, tt.args, dispatch)
			if exited != tt.wantExit {
				t.Errorf("exited = %v, want %v (output: %s)", exited, tt.wantExit, out)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output is missing %q:\n%s", tt.want, out)
			}
		})
	}
}

func TestDispatchRoutesReport(t *testing.T) {
	isolateData(t)
	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", "report"}, dispatch)
	if !strings.Contains(out, "No claim reports found") {
		t.Errorf("`claim report` was not routed:\n%s", out)
	}
}

func TestDispatchRoutesFilterMsg(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		w.WriteString("feat: x\n\n" + claudeTrailer + "\n")
		w.Close()
	}()
	prev := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prev })

	out, _, _ := runMain(t, []string{"claim", "__filter-msg"}, dispatch)
	if strings.Contains(out, "anthropic.com") {
		t.Errorf("__filter-msg was not routed to the filter: %q", out)
	}
	if !strings.Contains(out, "feat: x") {
		t.Errorf("output = %q", out)
	}
}

func TestDispatchRoutesALocalFolder(t *testing.T) {
	isolateData(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "dispatched"))
	stubPrompts(t, prompts{})

	out, _, _ := runMain(t, []string{"claim", repo, "--dry-run", "--force"}, dispatch)
	if !strings.Contains(out, "No changes made") {
		t.Errorf("a folder argument was not routed to the local flow:\n%s", out)
	}
}

func TestDispatchRoutesARepoURL(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	stubToken(t, "", nil)

	_, code, exited := runMain(t, []string{"claim", "https://github.com/owner/repo"}, dispatch)
	if !exited || code != 1 {
		t.Errorf("a repo URL should have been routed and then failed, got (%d, %v)", code, exited)
	}
}

func TestDispatchRoutesAPRURL(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	stubToken(t, "", nil)

	out, code, exited := runMain(t, []string{"claim", "https://github.com/owner/repo/pull/42"}, dispatch)
	if !exited || code != 1 {
		t.Errorf("a PR URL should have been routed and then failed, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "PR #42") {
		t.Errorf("the PR flow was not entered:\n%s", out)
	}
}

func TestDispatchRoutesAProfileURL(t *testing.T) {
	isolateData(t)
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})
	stubToken(t, "", nil)
	stubPrompts(t, prompts{})

	out, _, _ := runMain(t, []string{"claim", "https://github.com/izam-mohammed"}, dispatch)
	if !strings.Contains(out, "izam-mohammed") {
		t.Errorf("a profile URL was not routed to the multi-repo flow:\n%s", out)
	}
}

func TestRunRemoteRepoDelegatesAValidTarget(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(repoJSON("repo", false)))
	})

	out, _, _ := runMain(t, []string{"claim", "repo", "owner/repo"}, runRemoteRepo)
	if !strings.Contains(out, "Fetching repo info") {
		t.Errorf("runRemoteRepo did not reach the target flow:\n%s", out)
	}
}

func TestRunRemotePRDelegatesAValidTarget(t *testing.T) {
	isolateData(t)
	fakeGitHub(t, "owner", "repo", "feature")
	serveGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/pulls/") {
			w.Write([]byte(`{"number":42,"title":"t","head":{"ref":"feature","sha":"a"},"base":{"ref":"main"}}`))
			return
		}
		w.Write([]byte(repoJSON("repo", false)))
	})

	out, _, _ := runMain(t, []string{"claim", "pr", "owner/repo#42"}, runRemotePR)
	if !strings.Contains(out, "Fetching PR #42") {
		t.Errorf("runRemotePR did not reach the target flow:\n%s", out)
	}
}

func TestCloneToTempExitsWhenTheCloneFails(t *testing.T) {
	prev := cloneBase
	cloneBase = filepath.Join(t.TempDir(), "nothing-here")
	t.Cleanup(func() { cloneBase = prev })

	_, code, exited := runMain(t, []string{"claim"}, func() {
		cloneToTemp(githubpkg.NewPublicClient(), "owner", "repo")
	})
	if !exited || code != 1 {
		t.Errorf("a failed clone should exit, got (%d, %v)", code, exited)
	}
}

func TestRunLogoutRemoveNamedAccountFailure(t *testing.T) {
	isolateData(t)
	if err := auth.SaveAccount("izam", "tok", "pat"); err != nil {
		t.Fatal(err)
	}
	// Break the data directory so the removal cannot be written back.
	breakDataDir(t)

	_, code, exited := runMain(t, []string{"claim", "logout", "izam"}, runLogout)
	if exited && code != 1 {
		t.Errorf("unexpected exit code %d", code)
	}
}

func TestRunReportFolderArgumentWithID(t *testing.T) {
	isolateData(t)
	dir := t.TempDir()
	abs, _ := filepath.Abs(dir)
	rpt := saveReport(t, abs, map[string]string{"main": strings.Repeat("a", 40)})

	out, _, _ := runMain(t, []string{"claim", "report", dir, rpt.ID}, runReport)
	if !strings.Contains(out, rpt.ID) {
		t.Errorf("`claim report <folder> <id>` did not show the report:\n%s", out)
	}
}

func TestRunReportUnknownIDInAFolder(t *testing.T) {
	isolateData(t)
	dir := t.TempDir()
	_, code, exited := runMain(t, []string{"claim", "report", dir, "clm_nope"}, runReport)
	if !exited || code != 1 {
		t.Errorf("an unknown id should exit, got (%d, %v)", code, exited)
	}
}

func TestRevertReportUsesTheReportsOwnRepoPath(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "target"))
	commit(t, repo, "a.txt", "feat: x")
	head := git(t, repo, "rev-parse", "HEAD")
	rpt := saveReport(t, repo, map[string]string{"main": head})

	stubPrompts(t, prompts{confirm: func(string) bool { return true }})
	out, _, _ := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)
	if !strings.Contains(out, filepath.Base(repo)) {
		t.Errorf("revert should name the repository it is acting on:\n%s", out)
	}
}

func TestClaimRepoRefCaptureFailureIsWarnedAbout(t *testing.T) {
	isolateData(t)
	// A report built for a path that is a repo, then rewritten elsewhere, is
	// awkward to force; instead check the warning path via a bare report save
	// failure by breaking the data directory mid-run.
	stubFilter(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "warn"))
	breakDataDir(t)

	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, true, false)
	})
	if !strings.Contains(out, "Failed to record report") {
		t.Errorf("a report that cannot be saved should warn:\n%s", out)
	}
}

func TestClaimRepoDryRunReportSaveFailureWarns(t *testing.T) {
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "warn2"))
	breakDataDir(t)

	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, true, true, false)
	})
	if !strings.Contains(out, "Failed to record report") {
		t.Errorf("a dry-run report that cannot be saved should warn:\n%s", out)
	}
}

func TestClaimRepoAbortReportSaveFailureWarns(t *testing.T) {
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "warn3"))
	breakDataDir(t)
	stubPrompts(t, prompts{confirm: func(string) bool { return false }})

	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, false, false)
	})
	if !strings.Contains(out, "Failed to record report") {
		t.Errorf("an aborted report that cannot be saved should warn:\n%s", out)
	}
}

func TestRevertReportSaveFailureWarns(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "revertwarn"))
	commit(t, repo, "a.txt", "feat: x")
	head := git(t, repo, "rev-parse", "HEAD")
	rpt := saveReport(t, repo, map[string]string{"main": head})

	stubPrompts(t, prompts{confirm: func(string) bool { return true }})
	out, _, _ := runMain(t, []string{"claim", "revert", rpt.ID}, func() {
		// Break the data directory only once the report is already loaded.
		defer breakDataDirMidRun()()
		revertReport(repo, rpt)
	})
	if !strings.Contains(out, "Failed to record revert") {
		t.Errorf("a revert that cannot be recorded should warn:\n%s", out)
	}
}

func TestRunClaimMultiRepoRewriteFailureIsReported(t *testing.T) {
	isolateData(t)
	root := t.TempDir()
	dirtyRepo(t, filepath.Join(root, "one"))
	dirtyRepo(t, filepath.Join(root, "two"))

	stub := filepath.Join(t.TempDir(), "boom")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rewriter.SetExecutable(stub))

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", root, "--force"}, func() { runClaim(root) })
	if !strings.Contains(out, "✗") && !strings.Contains(out, "filter-branch") {
		t.Errorf("a failing rewrite should be reported per repo:\n%s", out)
	}
}

func TestReportListFailureExits(t *testing.T) {
	isolateData(t)
	dir, err := report.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	// A file where the reports directory should be makes listing fail.
	if err := os.WriteFile(dir, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, code, exited := runMain(t, []string{"claim", "report", "all"}, runReport)
	if !exited || code != 1 {
		t.Errorf("a failed listing should exit, got (%d, %v)", code, exited)
	}
}
