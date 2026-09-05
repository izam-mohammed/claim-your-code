package report

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/izam-mohammed/claim-your-code/internal/crypt"
	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

func TestIsRemote(t *testing.T) {
	if (&Report{}).IsRemote() {
		t.Error("a report with no RemoteURL reported itself as remote")
	}
	if !(&Report{RemoteURL: "https://github.com/o/r"}).IsRemote() {
		t.Error("a report with a RemoteURL did not report itself as remote")
	}
}

func TestGitStateSetters(t *testing.T) {
	r := &Report{}

	r.SetOriginalRefs(map[string]string{"main": "aaa"})
	if r.GitState == nil || r.GitState.BeforeRefs["main"] != "aaa" {
		t.Fatal("SetOriginalRefs did not populate GitState.BeforeRefs")
	}
	if r.OriginalRefs["main"] != "aaa" {
		t.Error("SetOriginalRefs did not keep the legacy OriginalRefs field in sync")
	}

	r.SetAfterRefs(map[string]string{"main": "bbb"})
	if r.GitState.AfterRefs["main"] != "bbb" {
		t.Error("SetAfterRefs did not record the post-rewrite refs")
	}

	r.SetPushed()
	if r.GitState.PushedAt == nil {
		t.Error("SetPushed did not stamp PushedAt")
	}

	r.SetPushError("remote rejected")
	if r.GitState.PushError != "remote rejected" {
		t.Errorf("PushError = %q", r.GitState.PushError)
	}
}

func TestGitStateSettersInitialiseOnFreshReports(t *testing.T) {
	// Each setter must create GitState when it is still nil.
	r := &Report{}
	r.SetAfterRefs(map[string]string{"a": "1"})
	if r.GitState == nil {
		t.Fatal("SetAfterRefs left GitState nil")
	}

	r = &Report{}
	r.SetPushed()
	if r.GitState == nil || r.GitState.PushedAt == nil {
		t.Fatal("SetPushed left GitState nil")
	}

	r = &Report{}
	r.SetPushError("x")
	if r.GitState == nil || r.GitState.PushError != "x" {
		t.Fatal("SetPushError left GitState nil")
	}
}

func TestGetBeforeRefsFallsBackToLegacyField(t *testing.T) {
	legacy := &Report{OriginalRefs: map[string]string{"main": "old"}}
	if got := legacy.GetBeforeRefs(); got["main"] != "old" {
		t.Errorf("GetBeforeRefs = %v, want the legacy OriginalRefs to be used", got)
	}

	both := &Report{
		OriginalRefs: map[string]string{"main": "legacy"},
		GitState:     &GitState{BeforeRefs: map[string]string{"main": "current"}},
	}
	if got := both.GetBeforeRefs(); got["main"] != "current" {
		t.Errorf("GetBeforeRefs = %v, want GitState to win over the legacy field", got)
	}

	if got := (&Report{}).GetBeforeRefs(); got != nil {
		t.Errorf("GetBeforeRefs = %v on an empty report, want nil", got)
	}
}

func TestIsRevertibleCases(t *testing.T) {
	cleaned := func() *Report {
		r := &Report{}
		r.SetOriginalRefs(map[string]string{"main": "aaa"})
		r.SetResult("cleaned", 3)
		return r
	}

	tests := []struct {
		name string
		rpt  *Report
		want bool
	}{
		{"cleaned with refs", cleaned(), true},
		{"no result yet", &Report{OriginalRefs: map[string]string{"main": "a"}}, false},
		{"dry run", func() *Report { r := cleaned(); r.SetResult("dry_run", 0); return r }(), false},
		{"aborted", func() *Report { r := cleaned(); r.SetResult("aborted", 0); return r }(), false},
		{"already reverted", func() *Report { r := cleaned(); r.SetReverted(); return r }(), false},
		{"cleaned but no refs", func() *Report { r := &Report{}; r.SetResult("cleaned", 1); return r }(), false},
		{"legacy refs only", &Report{
			OriginalRefs: map[string]string{"main": "a"},
			Result:       &Result{Status: "cleaned"},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rpt.IsRevertible(); got != tt.want {
				t.Errorf("IsRevertible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildWithNoResults(t *testing.T) {
	r := Build("v1", "/repo", 10, nil, nil)
	if r.Summary.AffectedCommits != 0 {
		t.Errorf("AffectedCommits = %d, want 0", r.Summary.AffectedCommits)
	}
	if r.Summary.TotalCommits != 10 {
		t.Errorf("TotalCommits = %d, want 10", r.Summary.TotalCommits)
	}
	if !strings.HasPrefix(r.ID, IDPrefix+"_") {
		t.Errorf("ID = %q, want the %q prefix", r.ID, IDPrefix)
	}
	if r.Version != "v1" {
		t.Errorf("Version = %q", r.Version)
	}
}

func TestBuildSkipsBlankModelInSummary(t *testing.T) {
	results := []scanner.Result{
		{Hash: "a", Subject: "one", Model: "Claude Opus 4.6"},
		{Hash: "b", Subject: "two", Model: ""},
	}
	r := Build("v", "/repo", 2, results, nil)
	if len(r.Summary.Models) != 1 {
		t.Errorf("Models = %v, want only the named model counted", r.Summary.Models)
	}
	if len(r.Commits) != 2 {
		t.Errorf("got %d commits, want both recorded", len(r.Commits))
	}
}

func TestBuildCarriesDiffStats(t *testing.T) {
	results := []scanner.Result{{
		Hash: "a", Subject: "one", Model: "Claude Opus 4.6",
		FilesChanged: 2, Insertions: 30, Deletions: 4,
	}}
	summaries := []scanner.BranchSummary{{
		Branch: "main", Count: 1, Models: map[string]int{"Claude Opus 4.6": 1},
	}}
	r := Build("v", "/repo", 5, results, summaries)
	if r.Commits[0].Insertions != 30 || r.Commits[0].Deletions != 4 || r.Commits[0].FilesChanged != 2 {
		t.Errorf("commit stats = %+v, want them carried through from the scan", r.Commits[0])
	}
	if len(r.Branches) != 1 || r.Branches[0].Name != "main" || r.Branches[0].Count != 1 {
		t.Errorf("branches = %+v", r.Branches)
	}
}

func TestSaveAndLoadRoundTripPreservesEverything(t *testing.T) {
	setTestDataDir(t)

	r := Build("v2", "/some/repo", 42, []scanner.Result{
		{Hash: strings.Repeat("a", 40), Subject: "feat: x", Model: "Claude Opus 4.6", Branches: []string{"main"}, Insertions: 5},
	}, []scanner.BranchSummary{
		{Branch: "main", Count: 1, Models: map[string]int{"Claude Opus 4.6": 1}},
	})
	r.RemoteURL = "https://github.com/o/r"
	r.RemoteOwner = "o"
	r.RemoteRepo = "r"
	r.PRNumber = 7
	r.PRTitle = "a pull request"
	r.PRBranch = "feature"
	r.SetOriginalRefs(map[string]string{"main": strings.Repeat("b", 40)})
	r.SetAfterRefs(map[string]string{"main": strings.Repeat("c", 40)})
	r.SetPushed()
	r.SetResult("cleaned", 1)

	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(r.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.PRNumber != 7 || got.PRTitle != "a pull request" || got.PRBranch != "feature" {
		t.Errorf("PR metadata lost: %+v", got)
	}
	if got.RemoteOwner != "o" || got.RemoteRepo != "r" {
		t.Errorf("remote metadata lost: %+v", got)
	}
	if got.GitState == nil || got.GitState.AfterRefs["main"] != strings.Repeat("c", 40) {
		t.Error("GitState did not survive the round trip")
	}
	if got.GitState.PushedAt == nil {
		t.Error("PushedAt did not survive the round trip")
	}
	if got.Result == nil || got.Result.Cleaned != 1 {
		t.Errorf("Result did not survive the round trip: %+v", got.Result)
	}
}

func TestSaveWritesEncrypted(t *testing.T) {
	dir := setTestDataDir(t)
	r := Build("v", "/repo", 1, nil, nil)
	r.Repo = "/a/very/distinctive/repo/path"
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, r.ID+".bin"))
	if err != nil {
		t.Fatalf("report file missing: %v", err)
	}
	if strings.Contains(string(raw), "distinctive") {
		t.Error("the report was written in plaintext — it must be encrypted at rest")
	}
	if _, err := crypt.DecryptBytes(string(raw)); err != nil {
		t.Errorf("the report file does not decrypt: %v", err)
	}
}

func TestLoadMissingReport(t *testing.T) {
	setTestDataDir(t)
	if _, err := Load("clm_nothere"); err == nil {
		t.Error("Load succeeded for an ID that was never saved")
	}
}

func TestLoadReadsLegacyPlainJSON(t *testing.T) {
	dir := setTestDataDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"id":"clm_legacy","repo":"/old/repo","summary":{"total_commits":3,"affected_commits":1}}`
	if err := os.WriteFile(filepath.Join(dir, "clm_legacy.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load("clm_legacy")
	if err != nil {
		t.Fatalf("Load of a legacy .json report: %v", err)
	}
	if got.Repo != "/old/repo" || got.Summary.AffectedCommits != 1 {
		t.Errorf("legacy report decoded as %+v", got)
	}
}

func TestListPrefersNewestFirst(t *testing.T) {
	setTestDataDir(t)

	older := Build("v", "/repo", 1, nil, nil)
	older.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
	if err := older.Save(); err != nil {
		t.Fatal(err)
	}
	newer := Build("v", "/repo", 1, nil, nil)
	newer.CreatedAt = time.Now().UTC()
	if err := newer.Save(); err != nil {
		t.Fatal(err)
	}

	all, err := ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAll returned %d reports, want 2", len(all))
	}
	if all[0].ID != newer.ID {
		t.Error("ListAll did not sort newest first")
	}
}

func TestListFiltersByRepo(t *testing.T) {
	setTestDataDir(t)

	mine := Build("v", "/repo/mine", 1, nil, nil)
	if err := mine.Save(); err != nil {
		t.Fatal(err)
	}
	theirs := Build("v", "/repo/theirs", 1, nil, nil)
	if err := theirs.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := List("/repo/mine")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != mine.ID {
		t.Errorf("List(/repo/mine) = %d reports, want just the one for that repo", len(got))
	}
}

func TestListIgnoresUnrelatedFilesAndDirs(t *testing.T) {
	dir := setTestDataDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "a-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.bin"), []byte("not encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}

	good := Build("v", "/repo", 1, nil, nil)
	if err := good.Save(); err != nil {
		t.Fatal(err)
	}

	all, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 || all[0].ID != good.ID {
		t.Errorf("ListAll = %d reports, want only the one valid report", len(all))
	}
}

func TestFindByPrefixAmbiguous(t *testing.T) {
	dir := setTestDataDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"clm_ab1111", "clm_ab2222"} {
		r := Build("v", "/repo", 1, nil, nil)
		r.ID = id
		if err := r.Save(); err != nil {
			t.Fatal(err)
		}
	}

	_, err := FindByPrefix("", "clm_ab")
	if err == nil {
		t.Fatal("FindByPrefix succeeded for a prefix matching two reports")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want it to call the prefix ambiguous", err)
	}
}

func TestFindByPrefixNoMatch(t *testing.T) {
	setTestDataDir(t)
	_, err := FindByPrefix("", "clm_zzzzzz")
	if err == nil {
		t.Fatal("FindByPrefix succeeded for a prefix that matches nothing")
	}
	if !strings.Contains(err.Error(), "no report found") {
		t.Errorf("error = %v", err)
	}
}

func TestLoadFileRejectsUndecryptableBin(t *testing.T) {
	dir := setTestDataDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "clm_bad.bin")
	if err := os.WriteFile(path, []byte("!!!not base64!!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFile(path); err == nil {
		t.Error("loadFile accepted a .bin file that does not decrypt")
	}
}

func TestGenerateIDIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := generateID()
		if seen[id] {
			t.Fatalf("generateID produced a duplicate: %s", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, IDPrefix+"_") || len(id) != len(IDPrefix)+7 {
			t.Fatalf("id %q does not match the clm_xxxxxx shape", id)
		}
	}
}

// unsetHome makes DataDir fail on macOS/Linux by removing the home lookup.
func unsetHome(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("LocalAppData", "")
		return
	}
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")
}

func TestDataDirFailsWithoutAHome(t *testing.T) {
	unsetHome(t)
	if _, err := DataDir(); err == nil {
		t.Error("DataDir succeeded with no home directory available")
	}
}

func TestDataDirUsesXDGOnUnix(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("XDG_DATA_HOME only applies to the unix branch")
	}
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join("/custom/data", "claim", "reports") {
		t.Errorf("DataDir = %q, want it rooted at XDG_DATA_HOME", dir)
	}
}

func TestSaveFailsWithoutADataDir(t *testing.T) {
	unsetHome(t)
	err := Build("v", "/repo", 1, nil, nil).Save()
	if err == nil {
		t.Fatal("Save succeeded with no data directory")
	}
	if !strings.Contains(err.Error(), "unable to track report") {
		t.Errorf("error = %v, want it to explain the report could not be tracked", err)
	}
}

func TestSaveFailsWhenDataDirIsAFile(t *testing.T) {
	dir := setTestDataDir(t)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	// Put a regular file where the reports directory needs to be.
	if err := os.WriteFile(dir, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Build("v", "/repo", 1, nil, nil).Save(); err == nil {
		t.Error("Save succeeded even though the reports path is a file")
	}
}

func TestListFailsWhenDataDirIsAFile(t *testing.T) {
	dir := setTestDataDir(t)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListAll(); err == nil {
		t.Error("ListAll succeeded even though the reports path is a file")
	}
}

func TestListEmptyWhenDirectoryAbsent(t *testing.T) {
	setTestDataDir(t) // nothing written, so the directory does not exist
	got, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll = %v, want no error when the directory is simply absent", err)
	}
	if len(got) != 0 {
		t.Errorf("ListAll = %+v, want empty", got)
	}
}

func TestListAndLoadFailWithoutADataDir(t *testing.T) {
	unsetHome(t)
	if _, err := ListAll(); err == nil {
		t.Error("ListAll succeeded with no data directory")
	}
	if _, err := Load("clm_x"); err == nil {
		t.Error("Load succeeded with no data directory")
	}
	if _, err := FindByPrefix("", "clm_x"); err == nil {
		t.Error("FindByPrefix succeeded with no data directory")
	}
}

func TestRemoteReportsAreNotRevertible(t *testing.T) {
	// A remote clean runs in a temp clone that is deleted afterwards, so the
	// refs it recorded name nothing that still exists.
	r := &Report{RemoteURL: "https://github.com/owner/repo", RemoteOwner: "owner", RemoteRepo: "repo"}
	r.SetOriginalRefs(map[string]string{"main": strings.Repeat("a", 40)})
	r.SetResult("cleaned", 2)

	if r.IsRevertible() {
		t.Error("a remote clean reported itself as revertible")
	}
}

func TestLocalReportWithTheSameShapeIsRevertible(t *testing.T) {
	// The same report without a RemoteURL must still be revertible, so the
	// guard above is not over-broad.
	r := &Report{Repo: "/home/izam/code/project"}
	r.SetOriginalRefs(map[string]string{"main": strings.Repeat("a", 40)})
	r.SetResult("cleaned", 2)

	if !r.IsRevertible() {
		t.Error("a local clean should still be revertible")
	}
}

func TestPrintDetailDoesNotOfferRevertForARemoteReport(t *testing.T) {
	r := &Report{
		ID:          "clm_remote",
		RemoteURL:   "https://github.com/owner/repo",
		RemoteOwner: "owner",
		RemoteRepo:  "repo",
		CreatedAt:   time.Now().UTC(),
		Summary:     Summary{TotalCommits: 5, AffectedCommits: 1, Models: map[string]int{"Claude Opus 4.6": 1}},
	}
	r.SetOriginalRefs(map[string]string{"main": strings.Repeat("a", 40)})
	r.SetResult("cleaned", 1)

	out := capture(t, func() { PrintDetail(r) })
	if strings.Contains(out, "claim revert") {
		t.Errorf("a remote report should not offer a revert command:\n%s", out)
	}
}
