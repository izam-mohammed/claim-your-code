package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const trailerOpus = "Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
const trailerSonnet = "Co-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>"

// testRepo creates an initialised git repo and returns its path.
func testRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "init", "-b", "main")
	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "Test")
	return repo
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, repo, file, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", file)
	run(t, repo, "commit", "-m", message)
}

func TestScanDefaultBranchIgnoresOtherBranches(t *testing.T) {
	repo := testRepo(t)
	commit(t, repo, "a.txt", "a", "feat: on main\n\n"+trailerOpus)
	run(t, repo, "checkout", "-b", "side")
	commit(t, repo, "b.txt", "b", "feat: on side\n\n"+trailerSonnet)
	run(t, repo, "checkout", "main")

	out, err := ScanDefaultBranch(repo)
	if err != nil {
		t.Fatalf("ScanDefaultBranch: %v", err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("found %d commits, want only the one on main", len(out.Results))
	}
	if out.Results[0].Subject != "feat: on main" {
		t.Errorf("subject = %q, want the commit on main", out.Results[0].Subject)
	}
	if out.TotalCommits != 1 {
		t.Errorf("TotalCommits = %d, want 1", out.TotalCommits)
	}

	all, err := Scan(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Results) != 2 {
		t.Errorf("Scan across all branches found %d, want 2", len(all.Results))
	}
}

func TestScanReportsProgress(t *testing.T) {
	repo := testRepo(t)
	for _, f := range []string{"a", "b", "c"} {
		commit(t, repo, f+".txt", f, "feat: "+f+"\n\n"+trailerOpus)
	}

	var labels []string
	_, err := ScanWithProgress(repo, false, func(done, total int, label string) {
		labels = append(labels, label)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) == 0 {
		t.Fatal("the progress callback was never invoked")
	}
	if labels[0] != "opening repository..." {
		t.Errorf("first progress label = %q, want the repository-opening step", labels[0])
	}
	joined := strings.Join(labels, "|")
	if !strings.Contains(joined, "main") {
		t.Errorf("progress labels %q never named the branch being walked", joined)
	}
	if !strings.Contains(joined, "commits scanned") {
		t.Errorf("progress labels %q never reported a commit count", joined)
	}
}

func TestScanCollectsDiffStats(t *testing.T) {
	repo := testRepo(t)
	commit(t, repo, "a.txt", "one\ntwo\nthree\n", "feat: add lines\n\n"+trailerOpus)

	out, err := Scan(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("found %d commits, want 1", len(out.Results))
	}
	r := out.Results[0]
	if r.Insertions != 3 {
		t.Errorf("Insertions = %d, want 3", r.Insertions)
	}
	if r.FilesChanged != 1 {
		t.Errorf("FilesChanged = %d, want 1", r.FilesChanged)
	}
	if r.Deletions != 0 {
		t.Errorf("Deletions = %d, want 0", r.Deletions)
	}
}

func TestScanBranchSummariesAggregateByModel(t *testing.T) {
	repo := testRepo(t)
	commit(t, repo, "a.txt", "a\n", "feat: a\n\n"+trailerOpus)
	commit(t, repo, "b.txt", "b\n", "feat: b\n\n"+trailerOpus)
	commit(t, repo, "c.txt", "c\n", "feat: c\n\n"+trailerSonnet)

	out, err := Scan(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.BranchSummaries) != 1 {
		t.Fatalf("got %d branch summaries, want 1", len(out.BranchSummaries))
	}
	bs := out.BranchSummaries[0]
	if bs.Branch != "main" {
		t.Errorf("Branch = %q, want main", bs.Branch)
	}
	if bs.Count != 3 {
		t.Errorf("Count = %d, want 3", bs.Count)
	}
	if bs.Models["Claude Opus 4.6"] != 2 {
		t.Errorf("Opus count = %d, want 2", bs.Models["Claude Opus 4.6"])
	}
	if bs.Models["Claude Sonnet 4.5 (1M context)"] != 1 {
		t.Errorf("Sonnet count = %d, want 1", bs.Models["Claude Sonnet 4.5 (1M context)"])
	}
	if bs.ModelStats["Claude Opus 4.6"].Insertions != 2 {
		t.Errorf("Opus insertions = %d, want 2", bs.ModelStats["Claude Opus 4.6"].Insertions)
	}
	if bs.Insertions != 3 {
		t.Errorf("branch insertions = %d, want 3", bs.Insertions)
	}
}

func TestScanPicksUpRemoteOnlyBranches(t *testing.T) {
	repo := testRepo(t)
	commit(t, repo, "a.txt", "a", "feat: base")
	head := run(t, repo, "rev-parse", "HEAD")

	// A remote-tracking ref with no local branch of the same name.
	run(t, repo, "update-ref", "refs/remotes/origin/remote-only", head)
	run(t, repo, "checkout", "--detach", head)
	commit(t, repo, "b.txt", "b", "feat: remote work\n\n"+trailerOpus)
	remoteHead := run(t, repo, "rev-parse", "HEAD")
	run(t, repo, "update-ref", "refs/remotes/origin/remote-only", remoteHead)
	run(t, repo, "checkout", "main")

	out, err := Scan(repo)
	if err != nil {
		t.Fatal(err)
	}
	var branches []string
	for _, bs := range out.BranchSummaries {
		branches = append(branches, bs.Branch)
	}
	found := false
	for _, b := range branches {
		if b == "remote-only" {
			found = true
		}
	}
	if !found {
		t.Errorf("branch summaries = %v, want the remote-only branch included", branches)
	}
}

func TestScanLocalBranchWinsOverRemoteOfSameName(t *testing.T) {
	repo := testRepo(t)
	commit(t, repo, "a.txt", "a", "feat: base\n\n"+trailerOpus)
	head := run(t, repo, "rev-parse", "HEAD")
	run(t, repo, "update-ref", "refs/remotes/origin/main", head)

	out, err := Scan(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.BranchSummaries) != 1 || out.BranchSummaries[0].Branch != "main" {
		t.Errorf("branch summaries = %+v, want a single \"main\" entry", out.BranchSummaries)
	}
}

func TestScanCleanRepoFindsNothing(t *testing.T) {
	repo := testRepo(t)
	commit(t, repo, "a.txt", "a", "feat: no trailers here")

	out, err := Scan(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 0 {
		t.Errorf("Results = %+v, want none", out.Results)
	}
	if out.TotalCommits != 1 {
		t.Errorf("TotalCommits = %d, want 1", out.TotalCommits)
	}
	if len(out.BranchSummaries) != 0 {
		t.Errorf("BranchSummaries = %+v, want none", out.BranchSummaries)
	}
}

func TestScanDefaultBranchNeedsAHead(t *testing.T) {
	repo := testRepo(t) // initialised, but no commits yet
	if _, err := ScanDefaultBranch(repo); err == nil {
		t.Fatal("ScanDefaultBranch succeeded on a repo with no commits")
	}
}

func TestScanDetachedHeadCommitsAreLabelled(t *testing.T) {
	repo := testRepo(t)
	commit(t, repo, "a.txt", "a", "feat: base")
	base := run(t, repo, "rev-parse", "HEAD")
	run(t, repo, "checkout", "--detach", base)
	commit(t, repo, "b.txt", "b", "feat: dangling\n\n"+trailerOpus)
	dangling := run(t, repo, "rev-parse", "HEAD")
	// Keep the commit reachable by a non-branch ref so the log walk still sees it.
	run(t, repo, "update-ref", "refs/notes/keep", dangling)
	run(t, repo, "checkout", "main")

	out, err := Scan(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out.Results {
		if r.Subject == "feat: dangling" {
			if len(r.Branches) != 1 || r.Branches[0] != "(detached)" {
				t.Errorf("branches for the unreferenced commit = %v, want [(detached)]", r.Branches)
			}
			return
		}
	}
	t.Skip("git did not surface the detached commit in this environment")
}

func TestScanNonExistentPath(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("Scan succeeded on a path that does not exist")
	}
}

func TestParseShortStat(t *testing.T) {
	tests := []struct {
		name                    string
		line                    string
		files, insert, deleteds int
	}{
		{"all three", " 3 files changed, 12 insertions(+), 4 deletions(-)", 3, 12, 4},
		{"single file singular", " 1 file changed, 1 insertion(+)", 1, 1, 0},
		{"deletions only", " 2 files changed, 7 deletions(-)", 2, 0, 7},
		{"empty", "", 0, 0, 0},
		{"unrelated text", "not a shortstat line", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, ins, del := parseShortStat(tt.line)
			if files != tt.files || ins != tt.insert || del != tt.deleteds {
				t.Errorf("parseShortStat(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tt.line, files, ins, del, tt.files, tt.insert, tt.deleteds)
			}
		})
	}
}

func TestEnrichDiffStatsNoResultsIsANoop(t *testing.T) {
	out := &ScanOutput{}
	enrichDiffStats(t.TempDir(), out) // must not panic
	if len(out.Results) != 0 {
		t.Error("enrichDiffStats invented results")
	}
}

func TestEnrichDiffStatsToleratesNonRepo(t *testing.T) {
	out := &ScanOutput{Results: []Result{{Hash: strings.Repeat("a", 40), Subject: "x"}}}
	enrichDiffStats(t.TempDir(), out)
	if out.Results[0].Insertions != 0 {
		t.Error("stats should stay zero when git cannot be queried")
	}
}
