package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

// saveReport stores a cleaned, revertible report for repoPath.
func saveReport(t *testing.T, repoPath string, refs map[string]string) *report.Report {
	t.Helper()
	rpt := report.Build(version, repoPath, 5, []scanner.Result{
		{Hash: strings.Repeat("a", 40), Subject: "feat: one", Model: "Claude Opus 4.6", Branches: []string{"main"}},
	}, []scanner.BranchSummary{
		{Branch: "main", Count: 1, Models: map[string]int{"Claude Opus 4.6": 1}},
	})
	rpt.SetOriginalRefs(refs)
	rpt.SetResult("cleaned", 1)
	if err := rpt.Save(); err != nil {
		t.Fatal(err)
	}
	return rpt
}

func TestRunReportEmptyList(t *testing.T) {
	isolateData(t)
	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", "report"}, runReport)
	if !strings.Contains(out, "No claim reports found") {
		t.Errorf("output = %q", out)
	}
}

func TestRunReportAllEmpty(t *testing.T) {
	isolateData(t)
	out, _, _ := runMain(t, []string{"claim", "report", "all"}, runReport)
	if !strings.Contains(out, "No claim reports found") {
		t.Errorf("output = %q", out)
	}
}

func TestRunReportAllShowsEveryReportInDetail(t *testing.T) {
	isolateData(t)
	saveReport(t, "/repo/one", map[string]string{"main": "aaa"})
	saveReport(t, "/repo/two", map[string]string{"main": "bbb"})

	out, _, _ := runMain(t, []string{"claim", "report", "all"}, runReport)
	if !strings.Contains(out, "/repo/one") || !strings.Contains(out, "/repo/two") {
		t.Errorf("`report all` should show both reports:\n%s", out)
	}
	if strings.Count(out, "Claim Report") != 2 {
		t.Errorf("expected two detail views:\n%s", out)
	}
}

func TestRunReportByID(t *testing.T) {
	isolateData(t)
	rpt := saveReport(t, "/repo/one", map[string]string{"main": "aaa"})

	out, _, _ := runMain(t, []string{"claim", "report", rpt.ID}, runReport)
	if !strings.Contains(out, rpt.ID) || !strings.Contains(out, "feat: one") {
		t.Errorf("report detail is missing:\n%s", out)
	}
}

func TestRunReportByAbbreviatedID(t *testing.T) {
	isolateData(t)
	rpt := saveReport(t, "/repo/one", map[string]string{"main": "aaa"})

	out, _, _ := runMain(t, []string{"claim", "report", rpt.ID[:6]}, runReport)
	if !strings.Contains(out, rpt.ID) {
		t.Errorf("an abbreviated id should resolve:\n%s", out)
	}
}

func TestRunReportUnknownID(t *testing.T) {
	isolateData(t)
	_, code, exited := runMain(t, []string{"claim", "report", "clm_zzzzzz"}, runReport)
	if !exited || code != 1 {
		t.Errorf("an unknown id should exit, got (%d, %v)", code, exited)
	}
}

func TestRunReportFiltersByFolder(t *testing.T) {
	isolateData(t)
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	mine := saveReport(t, abs, map[string]string{"main": "aaa"})
	saveReport(t, "/somewhere/else", map[string]string{"main": "bbb"})

	out, _, _ := runMain(t, []string{"claim", "report", dir, "all"}, runReport)
	if !strings.Contains(out, mine.ID) {
		t.Errorf("the folder's own report is missing:\n%s", out)
	}
	if strings.Contains(out, "/somewhere/else") {
		t.Errorf("a report for another repository leaked through:\n%s", out)
	}
}

func TestRunReportFolderOnlyListsThatFolder(t *testing.T) {
	isolateData(t)
	dir := t.TempDir()
	abs, _ := filepath.Abs(dir)
	saveReport(t, abs, map[string]string{"main": "aaa"})
	saveReport(t, "/elsewhere", map[string]string{"main": "bbb"})

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", "report", dir}, runReport)
	if !strings.Contains(out, "Claim Reports (1)") {
		t.Errorf("only the folder's reports should be listed:\n%s", out)
	}
}

func TestRunReportPicker(t *testing.T) {
	isolateData(t)
	saveReport(t, "/repo/one", map[string]string{"main": "aaa"})

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", "report"}, runReport)
	if !strings.Contains(out, "Claim Reports (1)") {
		t.Errorf("output = %q", out)
	}
}

func TestRunRevertMissingArgument(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim", "revert"}, runRevert)
	if !exited || code != 1 {
		t.Errorf("expected a usage failure, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "claim revert --help") {
		t.Errorf("the error should point at help:\n%s", out)
	}
}

func TestRunRevertUnknownID(t *testing.T) {
	isolateData(t)
	_, code, exited := runMain(t, []string{"claim", "revert", "clm_zzzzzz"}, runRevert)
	if !exited || code != 1 {
		t.Errorf("an unknown id should exit, got (%d, %v)", code, exited)
	}
}

func TestRevertRestoresBranches(t *testing.T) {
	isolateData(t)
	stubFilter(t)

	repo := dirtyRepo(t, filepath.Join(t.TempDir(), "revertme"))
	before := git(t, repo, "rev-parse", "HEAD")

	runMain(t, []string{"claim"}, func() { claimRepo(repo, false, true, false) })
	after := git(t, repo, "rev-parse", "HEAD")
	if after == before {
		t.Fatal("the clean did not rewrite anything, so there is nothing to revert")
	}

	reports, err := report.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(reports))
	}
	rpt := reports[0]

	stubPrompts(t, prompts{confirm: func(string) bool { return true }})
	out, _, _ := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)

	if git(t, repo, "rev-parse", "HEAD") != before {
		t.Error("revert did not restore the original branch tip")
	}
	if !strings.Contains(git(t, repo, "log", "--format=%B"), "anthropic.com") {
		t.Error("the Claude trailer was not restored")
	}
	if !strings.Contains(out, "Reverted") {
		t.Errorf("output = %q", out)
	}

	reloaded, err := report.Load(rpt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Reverted == nil {
		t.Error("the report was not marked reverted")
	}
}

func TestRevertDeclinedLeavesHistoryAlone(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "declined"))
	commit(t, repo, "a.txt", "feat: x")
	head := git(t, repo, "rev-parse", "HEAD")

	rpt := saveReport(t, repo, map[string]string{"main": head})

	stubPrompts(t, prompts{confirm: func(string) bool { return false }})
	out, _, _ := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)

	if !strings.Contains(out, "Aborted") {
		t.Errorf("output = %q", out)
	}
	reloaded, _ := report.Load(rpt.ID)
	if reloaded.Reverted != nil {
		t.Error("a declined revert marked the report reverted")
	}
}

func TestRevertShowsBranchesItWillRestore(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "show"))
	commit(t, repo, "a.txt", "feat: x")
	head := git(t, repo, "rev-parse", "HEAD")
	rpt := saveReport(t, repo, map[string]string{"main": head})

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)
	if !strings.Contains(out, "Branches to restore") || !strings.Contains(out, "main") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, head[:8]) {
		t.Errorf("the target commit should be shown:\n%s", out)
	}
}

func TestRevertRejectsAnAlreadyRevertedReport(t *testing.T) {
	isolateData(t)
	rpt := saveReport(t, "/repo", map[string]string{"main": strings.Repeat("a", 40)})
	rpt.SetReverted()
	if err := rpt.Save(); err != nil {
		t.Fatal(err)
	}

	out, code, exited := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)
	if !exited || code != 1 {
		t.Errorf("expected an exit, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "already been reverted") {
		t.Errorf("output = %q", out)
	}
}

func TestRevertRejectsADryRunReport(t *testing.T) {
	isolateData(t)
	rpt := report.Build(version, "/repo", 3, nil, nil)
	rpt.SetOriginalRefs(map[string]string{"main": strings.Repeat("a", 40)})
	rpt.SetResult("dry_run", 0)
	if err := rpt.Save(); err != nil {
		t.Fatal(err)
	}

	out, code, exited := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)
	if !exited || code != 1 {
		t.Errorf("expected an exit, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "not a successful clean") {
		t.Errorf("output = %q", out)
	}
}

func TestRevertRejectsAReportWithNoRefs(t *testing.T) {
	isolateData(t)
	rpt := report.Build(version, "/repo", 3, nil, nil)
	rpt.SetResult("cleaned", 1)
	if err := rpt.Save(); err != nil {
		t.Fatal(err)
	}

	out, code, exited := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)
	if !exited || code != 1 {
		t.Errorf("expected an exit, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "no branch refs") {
		t.Errorf("output = %q", out)
	}
}

func TestRevertFailureExits(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "badref"))
	commit(t, repo, "a.txt", "feat: x")
	// A ref that does not exist in this repository.
	rpt := saveReport(t, repo, map[string]string{"main": strings.Repeat("f", 40)})

	stubPrompts(t, prompts{confirm: func(string) bool { return true }})
	_, code, exited := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)
	if !exited || code != 1 {
		t.Errorf("a failed revert should exit, got (%d, %v)", code, exited)
	}
}

func TestRevertRejectsARemoteReportWithAClearMessage(t *testing.T) {
	isolateData(t)
	rpt := report.Build(version, "github.com/owner/repo", 5, nil, nil)
	rpt.RemoteURL = "https://github.com/owner/repo"
	rpt.RemoteOwner = "owner"
	rpt.RemoteRepo = "repo"
	rpt.SetOriginalRefs(map[string]string{"main": strings.Repeat("a", 40)})
	rpt.SetResult("cleaned", 2)
	if err := rpt.Save(); err != nil {
		t.Fatal(err)
	}

	out, code, exited := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)
	if !exited || code != 1 {
		t.Errorf("expected an exit, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "remote clean, which cannot be reverted") {
		t.Errorf("the error should explain why, not fail at chdir:\n%s", out)
	}
	if strings.Contains(out, "no such file or directory") {
		t.Errorf("revert should refuse up front, not try to chdir into a deleted clone:\n%s", out)
	}
}

func TestRevertReportWithNoRecordedResult(t *testing.T) {
	// The guard allowed Result to be nil and the message then dereferenced it.
	isolateData(t)
	rpt := report.Build(version, "/repo", 3, nil, nil)
	rpt.SetOriginalRefs(map[string]string{"main": strings.Repeat("a", 40)})

	out, code, exited := runMain(t, []string{"claim"}, func() {
		revertReport("/repo", rpt, false)
	})
	if !exited || code != 1 {
		t.Errorf("expected an exit, got (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "no recorded result") {
		t.Errorf("output = %q", out)
	}
}

func TestClaimRepoOnADetachedHEADWithDefaultScope(t *testing.T) {
	isolateData(t)
	stubFilter(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "detached"))
	commit(t, repo, "a.txt", "feat: base")
	commit(t, repo, "b.txt", "feat: claimed\n\n"+claudeTrailer)
	head := git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "checkout", "--detach", head)

	out, _, _ := runMain(t, []string{"claim"}, func() {
		claimRepo(repo, false, true, true) // default-branch scope
	})
	if strings.Contains(out, "filter-branch failed") {
		t.Errorf("cleaning a detached HEAD failed:\n%s", out)
	}
	if strings.Contains(git(t, repo, "log", "--format=%B"), "anthropic.com") {
		t.Error("the trailer survived on a detached HEAD")
	}
}

func TestRunRevertWithForceSkipsTheConfirmation(t *testing.T) {
	// revert never looked at --force, so it could only ever be answered by
	// hand: in a script it printed the plan and aborted.
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "revertforce"))
	commit(t, repo, "a.txt", "feat: x")
	head := git(t, repo, "rev-parse", "HEAD")
	rpt := saveReport(t, repo, map[string]string{"main": head})
	commit(t, repo, "b.txt", "feat: y") // move main past the recorded tip

	stubPrompts(t, prompts{}) // an unset confirm answers no
	out, _, _ := runMain(t, []string{"claim", "revert", rpt.ID, "--force"}, runRevert)

	if strings.Contains(out, "Aborted") {
		t.Errorf("--force should skip the confirmation:\n%s", out)
	}
	if got := git(t, repo, "rev-parse", "main"); got != head {
		t.Errorf("main = %q, want it restored to %q", got, head)
	}
}

func TestRunRevertStillConfirmsWithoutForce(t *testing.T) {
	isolateData(t)
	repo := newRepo(t, filepath.Join(t.TempDir(), "revertask"))
	commit(t, repo, "a.txt", "feat: x")
	head := git(t, repo, "rev-parse", "HEAD")
	rpt := saveReport(t, repo, map[string]string{"main": head})
	commit(t, repo, "b.txt", "feat: y")
	moved := git(t, repo, "rev-parse", "HEAD")

	stubPrompts(t, prompts{})
	out, _, _ := runMain(t, []string{"claim", "revert", rpt.ID}, runRevert)

	if !strings.Contains(out, "Aborted") {
		t.Errorf("a declined confirmation should abort:\n%s", out)
	}
	if got := git(t, repo, "rev-parse", "main"); got != moved {
		t.Errorf("main = %q, want it left alone at %q", got, moved)
	}
}
