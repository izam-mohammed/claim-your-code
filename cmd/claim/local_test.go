package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/rewriter"
)

func TestRunFilterMsgStripsTrailers(t *testing.T) {
	prev := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prev })

	go func() {
		w.WriteString("feat: thing\n\n" + claudeTrailer + "\nCo-Authored-By: A Human <human@example.com>\n")
		w.Close()
	}()

	out, _, exited := runMain(t, []string{"claim", "__filter-msg"}, runFilterMsg)
	if exited {
		t.Error("runFilterMsg should not exit")
	}
	if strings.Contains(out, "anthropic.com") {
		t.Errorf("the Claude trailer survived: %q", out)
	}
	if !strings.Contains(out, "A Human") {
		t.Errorf("the human co-author was dropped: %q", out)
	}
	if !strings.Contains(out, "feat: thing") {
		t.Errorf("the subject was dropped: %q", out)
	}
}

func TestResolveReposPlainRepo(t *testing.T) {
	repo := newRepo(t, filepath.Join(t.TempDir(), "solo"))
	var got []string
	runMain(t, []string{"claim"}, func() { got = resolveRepos(repo, false) })
	if len(got) != 1 || got[0] != repo {
		t.Errorf("resolveRepos = %v, want just the repo itself", got)
	}
}

func TestResolveReposIncludesNestedWhenConfirmed(t *testing.T) {
	root := newRepo(t, filepath.Join(t.TempDir(), "outer"))
	inner := newRepo(t, filepath.Join(root, "packages", "inner"))

	stubPrompts(t, prompts{confirm: func(string) bool { return true }})
	var got []string
	out, _, _ := runMain(t, []string{"claim"}, func() { got = resolveRepos(root, false) })

	if len(got) != 2 || got[0] != root || got[1] != inner {
		t.Errorf("resolveRepos = %v, want the outer repo then the nested one", got)
	}
	if !strings.Contains(out, "1 nested repo(s)") {
		t.Errorf("the nested repo was not announced:\n%s", out)
	}
}

func TestResolveReposSkipsNestedWhenDeclined(t *testing.T) {
	root := newRepo(t, filepath.Join(t.TempDir(), "outer"))
	newRepo(t, filepath.Join(root, "inner"))

	stubPrompts(t, prompts{confirm: func(string) bool { return false }})
	var got []string
	runMain(t, []string{"claim"}, func() { got = resolveRepos(root, false) })
	if len(got) != 1 {
		t.Errorf("resolveRepos = %v, want only the outer repo", got)
	}
}

func TestResolveReposForceIncludesNestedWithoutAsking(t *testing.T) {
	root := newRepo(t, filepath.Join(t.TempDir(), "outer"))
	newRepo(t, filepath.Join(root, "inner"))

	stubPrompts(t, prompts{}) // any prompt would answer "no"
	var got []string
	runMain(t, []string{"claim"}, func() { got = resolveRepos(root, true) })
	if len(got) != 2 {
		t.Errorf("resolveRepos with --force = %v, want the nested repo included", got)
	}
}

func TestResolveReposSearchesAFolder(t *testing.T) {
	root := t.TempDir()
	newRepo(t, filepath.Join(root, "one"))
	newRepo(t, filepath.Join(root, "two"))

	stubPrompts(t, prompts{selectOne: answer("__all__")})
	var got []string
	out, _, _ := runMain(t, []string{"claim"}, func() { got = resolveRepos(root, false) })

	if len(got) != 2 {
		t.Errorf("resolveRepos = %v, want both repos", got)
	}
	if !strings.Contains(out, "2 repo(s)") {
		t.Errorf("output = %q", out)
	}
}

func TestResolveReposSearchPicksOne(t *testing.T) {
	root := t.TempDir()
	one := newRepo(t, filepath.Join(root, "one"))
	newRepo(t, filepath.Join(root, "two"))

	stubPrompts(t, prompts{selectOne: answer(one)})
	var got []string
	runMain(t, []string{"claim"}, func() { got = resolveRepos(root, false) })
	if len(got) != 1 || got[0] != one {
		t.Errorf("resolveRepos = %v, want just the selected repo", got)
	}
}

func TestResolveReposSearchOffersAllAndEachRepo(t *testing.T) {
	root := t.TempDir()
	newRepo(t, filepath.Join(root, "alpha"))
	newRepo(t, filepath.Join(root, "beta"))

	var offered []huh.Option[string]
	stubPrompts(t, prompts{selectOne: func(_ string, opts []huh.Option[string], _ int) (string, error) {
		offered = opts
		return "__all__", nil
	}})
	runMain(t, []string{"claim"}, func() { resolveRepos(root, false) })

	if len(offered) != 3 {
		t.Fatalf("offered %d options, want an \"all\" entry plus both repos", len(offered))
	}
	if offered[0].Value != "__all__" {
		t.Errorf("the first option should be \"all repos\", got %q", offered[0].Value)
	}
}

func TestResolveReposSearchForceTakesAll(t *testing.T) {
	root := t.TempDir()
	newRepo(t, filepath.Join(root, "one"))
	newRepo(t, filepath.Join(root, "two"))

	stubPrompts(t, prompts{})
	var got []string
	runMain(t, []string{"claim"}, func() { got = resolveRepos(root, true) })
	if len(got) != 2 {
		t.Errorf("resolveRepos with --force = %v, want both repos without prompting", got)
	}
}

func TestResolveReposAbortsWhenSelectionCancelled(t *testing.T) {
	root := t.TempDir()
	newRepo(t, filepath.Join(root, "one"))

	stubPrompts(t, prompts{}) // selectOne returns an error
	out, code, exited := runMain(t, []string{"claim"}, func() { resolveRepos(root, false) })
	if !exited || code != 0 {
		t.Errorf("a cancelled selection should exit cleanly, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("output = %q", out)
	}
}

func TestResolveReposNoRepositoriesFound(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim"}, func() {
		resolveRepos(t.TempDir(), false)
	})
	if !exited || code != 1 {
		t.Errorf("an empty folder should fail, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "No git repositories found") {
		t.Errorf("output = %q", out)
	}
}

func TestClaimRepoCleanRepositoryDoesNothing(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "clean"))
	commit(t, repo, "a.txt", "feat: nothing to claim")

	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, true, false)
	})
	if !strings.Contains(out, "No Claude co-authorship found") {
		t.Errorf("output = %q", out)
	}
}

func TestClaimRepoDryRunChangesNothing(t *testing.T) {
	isolateData(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "dirty"))
	before := git(t, repo, "rev-parse", "HEAD")

	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, true, true, false)
	})

	if git(t, repo, "rev-parse", "HEAD") != before {
		t.Error("a dry run rewrote history")
	}
	if !strings.Contains(out, "No changes made") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "Report tracked") {
		t.Errorf("a dry run should still record a report:\n%s", out)
	}

	reports, err := report.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Result.Status != "dry_run" {
		t.Errorf("reports = %+v, want one dry_run report", reports)
	}
}

func TestClaimRepoRewritesAndRecordsAReport(t *testing.T) {
	isolateData(t)
	stubFilter(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "dirty"))
	before := git(t, repo, "rev-parse", "HEAD")

	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, true, false)
	})

	if strings.Contains(git(t, repo, "log", "--all", "--format=%B"), "anthropic.com") {
		t.Error("the Claude trailer survived the rewrite")
	}
	if git(t, repo, "rev-parse", "HEAD") == before {
		t.Error("history was not rewritten")
	}
	if !strings.Contains(out, "Cleaned 1 commit(s)") {
		t.Errorf("output = %q", out)
	}

	reports, err := report.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(reports))
	}
	r := reports[0]
	if r.Result.Status != "cleaned" || r.Result.Cleaned != 1 {
		t.Errorf("result = %+v, want a cleaned report", r.Result)
	}
	if !r.IsRevertible() {
		t.Error("the report should be revertible")
	}
	if r.GitState.BeforeRefs["main"] != before {
		t.Errorf("BeforeRefs = %v, want the pre-rewrite tip %s", r.GitState.BeforeRefs, before)
	}
}

func TestClaimRepoAbortRecordsAnAbortedReport(t *testing.T) {
	isolateData(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "dirty"))
	before := git(t, repo, "rev-parse", "HEAD")

	stubPrompts(t, prompts{confirm: func(string) bool { return false }})
	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, false, false)
	})

	if git(t, repo, "rev-parse", "HEAD") != before {
		t.Error("declining the prompt still rewrote history")
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("output = %q", out)
	}
	reports, _ := report.ListAll()
	if len(reports) != 1 || reports[0].Result.Status != "aborted" {
		t.Errorf("reports = %+v, want one aborted report", reports)
	}
}

func TestClaimRepoConfirmedRewrite(t *testing.T) {
	isolateData(t)
	stubFilter(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "dirty"))

	stubPrompts(t, prompts{confirm: func(string) bool { return true }})
	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, false, false)
	})
	if !strings.Contains(out, "Cleaned") {
		t.Errorf("output = %q", out)
	}
	if strings.Contains(git(t, repo, "log", "--all", "--format=%B"), "anthropic.com") {
		t.Error("the trailer survived a confirmed rewrite")
	}
}

func TestClaimRepoDefaultBranchOnly(t *testing.T) {
	isolateData(t)
	stubFilter(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "multi"))
	commit(t, repo, "a.txt", "feat: main work\n\n"+claudeTrailer)
	git(t, repo, "checkout", "-b", "side")
	commit(t, repo, "b.txt", "feat: side work\n\n"+claudeTrailer)
	git(t, repo, "checkout", "main")

	runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, true, true) // defaultOnly
	})

	if strings.Contains(git(t, repo, "log", "main", "--format=%B"), "anthropic.com") {
		t.Error("main was not cleaned")
	}
	if !strings.Contains(git(t, repo, "log", "side", "--format=%B"), "anthropic.com") {
		t.Error("the side branch was cleaned, but only the default branch was in scope")
	}
}

func TestClaimRepoOnANonRepositoryReportsTheError(t *testing.T) {
	isolateData(t)
	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(t.TempDir(), false, true, false)
	})
	if !strings.Contains(out, "Error:") {
		t.Errorf("output = %q, want a scan error", out)
	}
}

func TestClaimRepoRewriteFailureIsReported(t *testing.T) {
	isolateData(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "dirty"))
	// A filter binary that always fails.
	stub := filepath.Join(t.TempDir(), "boom")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rewriter.SetExecutable(stub))

	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, true, false)
	})
	if !strings.Contains(out, "Error:") {
		t.Errorf("a failing rewrite should be reported:\n%s", out)
	}
}

func TestPromptPushWithoutARemote(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "no-remote"))
	commit(t, repo, "a.txt", "feat: x")

	out, _, _ := runMain(t, []string{"claim"}, func() {
		promptPush(repo, report.Build("v", repo, 1, nil, nil), nil, nil)
	})
	if !strings.Contains(out, "No remote found") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "git push --force-with-lease --all") {
		t.Errorf("the manual push hint is missing:\n%s", out)
	}
}

func TestPromptPushWithNothingChanged(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "unchanged"))
	commit(t, repo, "a.txt", "feat: x")
	git(t, repo, "remote", "add", "origin", "https://github.com/o/r.git")

	refs := map[string]string{"main": "same"}
	out, _, _ := runMain(t, []string{"claim"}, func() {
		promptPush(repo, report.Build("v", repo, 1, nil, nil), refs, refs)
	})
	if strings.Contains(out, "force-push") {
		t.Errorf("nothing changed, so no push should be offered:\n%s", out)
	}
}

func TestPromptPushDeclined(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "declined"))
	commit(t, repo, "a.txt", "feat: x")
	git(t, repo, "remote", "add", "origin", "https://github.com/o/r.git")

	stubPrompts(t, prompts{dangerous: func(string) bool { return false }})
	out, _, _ := runMain(t, []string{"claim"}, func() {
		promptPush(repo, report.Build("v", repo, 1, nil, nil),
			map[string]string{"main": "old"}, map[string]string{"main": "new"})
	})
	if !strings.Contains(out, "Push skipped") {
		t.Errorf("output = %q", out)
	}
}

func TestPromptPushFailureIsRecorded(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "pushfail"))
	commit(t, repo, "a.txt", "feat: x")
	head := git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git"))

	rpt := report.Build("v", repo, 1, nil, nil)
	stubPrompts(t, prompts{dangerous: func(string) bool { return true }})
	out, _, _ := runMain(t, []string{"claim"}, func() {
		promptPush(repo, rpt, map[string]string{"main": "0000000"}, map[string]string{"main": head})
	})

	if !strings.Contains(out, "Push failed") {
		t.Errorf("output = %q", out)
	}
	if rpt.GitState == nil || rpt.GitState.PushError == "" {
		t.Error("the push failure was not recorded on the report")
	}
}

func TestPromptPushSucceeds(t *testing.T) {
	isolateData(t)
	root := t.TempDir()

	seed := newRepo(t, filepath.Join(root, "seed"))
	commit(t, seed, "a.txt", "feat: x")
	origin := filepath.Join(root, "origin.git")
	git(t, root, "clone", "--bare", "--quiet", seed, origin)

	work := filepath.Join(root, "work")
	git(t, root, "clone", "--quiet", origin, work)
	git(t, work, "config", "user.email", "test@example.com")
	git(t, work, "config", "user.name", "Test")
	before := git(t, work, "rev-parse", "HEAD")
	commit(t, work, "b.txt", "feat: y")
	after := git(t, work, "rev-parse", "HEAD")

	rpt := report.Build("v", work, 1, nil, nil)
	stubPrompts(t, prompts{dangerous: func(string) bool { return true }})
	out, _, _ := runMain(t, []string{"claim"}, func() {
		promptPush(work, rpt, map[string]string{"main": before}, map[string]string{"main": after})
	})

	if !strings.Contains(out, "Pushed") {
		t.Errorf("output = %q", out)
	}
	if git(t, origin, "rev-parse", "main") != after {
		t.Error("origin was not updated")
	}
	if rpt.GitState == nil || rpt.GitState.PushedAt == nil {
		t.Error("the push was not recorded on the report")
	}
}

func TestRunClaimSingleRepoEndToEnd(t *testing.T) {
	isolateData(t)
	stubFilter(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "single"))

	stubPrompts(t, prompts{
		selectOne: answer("all"),
		confirm:   func(string) bool { return true },
	})
	out, _, _ := runMain(t, []string{"claim", repo}, func() { runClaim(repo) })

	if !strings.Contains(out, "Cleaned") {
		t.Errorf("output = %q", out)
	}
	if strings.Contains(git(t, repo, "log", "--all", "--format=%B"), "anthropic.com") {
		t.Error("the trailer survived")
	}
}

func TestRunClaimParsesFlags(t *testing.T) {
	isolateData(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "flags"))
	before := git(t, repo, "rev-parse", "HEAD")

	// --dry-run and --force together: no prompts, no changes.
	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", repo, "--dry-run", "--force"}, func() { runClaim(repo) })

	if git(t, repo, "rev-parse", "HEAD") != before {
		t.Error("--dry-run rewrote history")
	}
	if !strings.Contains(out, "No changes made") {
		t.Errorf("output = %q", out)
	}
}

func TestRunClaimShortForceFlag(t *testing.T) {
	isolateData(t)
	stubFilter(t)
	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "shortflag"))

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", repo, "-f"}, func() { runClaim(repo) })
	if !strings.Contains(out, "Cleaned") {
		t.Errorf("-f should skip every prompt and clean:\n%s", out)
	}
}

func TestRunClaimMultiRepoDryRun(t *testing.T) {
	isolateData(t)
	root := t.TempDir()
	dirtyRepo(t, filepath.Join(root, "one"))
	dirtyRepo(t, filepath.Join(root, "two"))
	newRepo(t, filepath.Join(root, "clean"))

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", root, "--dry-run", "--force"}, func() { runClaim(root) })

	if !strings.Contains(out, "2 repo(s)") {
		t.Errorf("the summary should name the two affected repos:\n%s", out)
	}
	if !strings.Contains(out, "No changes made") {
		t.Errorf("output = %q", out)
	}
	reports, _ := report.ListAll()
	if len(reports) != 2 {
		t.Errorf("got %d reports, want one per affected repo", len(reports))
	}
}

func TestRunClaimMultiRepoRewrites(t *testing.T) {
	isolateData(t)
	stubFilter(t)
	root := t.TempDir()
	one := dirtyRepo(t, filepath.Join(root, "one"))
	two := dirtyRepo(t, filepath.Join(root, "two"))

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", root, "--force"}, func() { runClaim(root) })

	for _, repo := range []string{one, two} {
		if strings.Contains(git(t, repo, "log", "--all", "--format=%B"), "anthropic.com") {
			t.Errorf("%s still has a Claude trailer", filepath.Base(repo))
		}
	}
	if !strings.Contains(out, "Cleaned") {
		t.Errorf("output = %q", out)
	}
}

func TestRunClaimMultiRepoDeclinedDoesNothing(t *testing.T) {
	isolateData(t)
	root := t.TempDir()
	one := dirtyRepo(t, filepath.Join(root, "one"))
	dirtyRepo(t, filepath.Join(root, "two"))
	before := git(t, one, "rev-parse", "HEAD")

	stubPrompts(t, prompts{
		selectOne: answer("all"),
		confirm:   func(string) bool { return false },
	})
	runMain(t, []string{"claim", root}, func() { runClaim(root) })

	if git(t, one, "rev-parse", "HEAD") != before {
		t.Error("declining the prompt still rewrote history")
	}
}

func TestRunClaimAllCleanRepos(t *testing.T) {
	isolateData(t)
	root := t.TempDir()
	newRepo(t, filepath.Join(root, "one"))
	newRepo(t, filepath.Join(root, "two"))
	commit(t, filepath.Join(root, "one"), "a.txt", "feat: clean")
	commit(t, filepath.Join(root, "two"), "a.txt", "feat: clean")

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", root, "--force"}, func() { runClaim(root) })
	if !strings.Contains(out, "No Claude co-authorship found") {
		t.Errorf("output = %q", out)
	}
}
