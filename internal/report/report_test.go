package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

// setTestDataDir overrides environment so DataDir() returns a temp directory.
func setTestDataDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()

	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", tmp)
		// DataDir will resolve to tmp/Library/Application Support/claim/reports
	case "windows":
		t.Setenv("LocalAppData", tmp)
	default:
		t.Setenv("XDG_DATA_HOME", tmp)
	}

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() failed: %v", err)
	}
	return dir
}

func TestDataDir(t *testing.T) {
	dir := setTestDataDir(t)
	if dir == "" {
		t.Fatal("DataDir() returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("DataDir() should return absolute path, got %q", dir)
	}
	if filepath.Base(dir) != "reports" {
		t.Errorf("DataDir() should end with 'reports', got %q", dir)
	}
}

func TestGenerateID(t *testing.T) {
	id := generateID()
	if len(id) == 0 {
		t.Fatal("generateID() returned empty string")
	}
	if id[:4] != IDPrefix+"_" {
		t.Errorf("expected ID to start with %q, got %q", IDPrefix+"_", id[:4])
	}

	// IDs should be unique
	id2 := generateID()
	if id == id2 {
		t.Errorf("generateID() returned duplicate: %q", id)
	}
}

func TestBuild(t *testing.T) {
	results := []scanner.Result{
		{Hash: "aaaa" + "bbbb" + "cccc" + "dddd" + "eeee" + "ffff" + "1111" + "2222" + "3333" + "4444", Subject: "Fix bug", Model: "Claude Opus 4.6", Branches: []string{"main"}},
		{Hash: "1111" + "2222" + "3333" + "4444" + "5555" + "6666" + "7777" + "8888" + "9999" + "0000", Subject: "Add feature", Model: "Claude Sonnet 4.5", Branches: []string{"main", "dev"}},
	}
	summaries := []scanner.BranchSummary{
		{Branch: "main", Count: 2, Models: map[string]int{"Claude Opus 4.6": 1, "Claude Sonnet 4.5": 1}},
	}

	rpt := Build("1.0.0", "/tmp/repo", 10, results, summaries)

	if rpt.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", rpt.Version)
	}
	if rpt.Repo != "/tmp/repo" {
		t.Errorf("expected repo '/tmp/repo', got %q", rpt.Repo)
	}
	if rpt.Summary.TotalCommits != 10 {
		t.Errorf("expected 10 total commits, got %d", rpt.Summary.TotalCommits)
	}
	if rpt.Summary.AffectedCommits != 2 {
		t.Errorf("expected 2 affected commits, got %d", rpt.Summary.AffectedCommits)
	}
	if len(rpt.Commits) != 2 {
		t.Errorf("expected 2 commits, got %d", len(rpt.Commits))
	}
	if len(rpt.Branches) != 1 {
		t.Errorf("expected 1 branch, got %d", len(rpt.Branches))
	}
	if rpt.Summary.Models["Claude Opus 4.6"] != 1 {
		t.Error("expected model count for Claude Opus 4.6 to be 1")
	}
}

func TestSetResultAndRevert(t *testing.T) {
	rpt := &Report{ID: "clm_test01"}

	if rpt.IsRevertible() {
		t.Error("report without result should not be revertible")
	}

	rpt.SetResult("cleaned", 5)
	if rpt.Result == nil {
		t.Fatal("SetResult did not set result")
	}
	if rpt.Result.Status != "cleaned" {
		t.Errorf("expected status 'cleaned', got %q", rpt.Result.Status)
	}
	if rpt.Result.Cleaned != 5 {
		t.Errorf("expected cleaned 5, got %d", rpt.Result.Cleaned)
	}

	rpt.SetOriginalRefs(map[string]string{"main": "abc123"})
	if !rpt.IsRevertible() {
		t.Error("cleaned report with refs should be revertible")
	}

	rpt.SetReverted()
	if rpt.Reverted == nil {
		t.Fatal("SetReverted did not set reverted")
	}
	if rpt.IsRevertible() {
		t.Error("reverted report should not be revertible")
	}
}

func TestIsRevertible(t *testing.T) {
	tests := []struct {
		name string
		rpt  Report
		want bool
	}{
		{
			name: "no result",
			rpt:  Report{},
			want: false,
		},
		{
			name: "dry run",
			rpt:  Report{Result: &Result{Status: "dry_run"}},
			want: false,
		},
		{
			name: "aborted",
			rpt:  Report{Result: &Result{Status: "aborted"}},
			want: false,
		},
		{
			name: "cleaned without refs",
			rpt:  Report{Result: &Result{Status: "cleaned"}},
			want: false,
		},
		{
			name: "cleaned with refs",
			rpt:  Report{Result: &Result{Status: "cleaned"}, OriginalRefs: map[string]string{"main": "abc"}},
			want: true,
		},
		{
			name: "cleaned but already reverted",
			rpt:  Report{Result: &Result{Status: "cleaned"}, OriginalRefs: map[string]string{"main": "abc"}, Reverted: &Reverted{}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rpt.IsRevertible(); got != tt.want {
				t.Errorf("IsRevertible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	setTestDataDir(t)

	rpt := &Report{
		ID:        "clm_aabbcc",
		Version:   "1.0.0",
		Repo:      "/tmp/test-repo",
		CreatedAt: time.Now().UTC(),
		Summary: Summary{
			TotalCommits:    10,
			AffectedCommits: 3,
			Models:          map[string]int{"Claude Opus 4.6": 3},
		},
	}

	if err := rpt.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load("clm_aabbcc")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.ID != rpt.ID {
		t.Errorf("loaded ID = %q, want %q", loaded.ID, rpt.ID)
	}
	if loaded.Repo != rpt.Repo {
		t.Errorf("loaded Repo = %q, want %q", loaded.Repo, rpt.Repo)
	}
	if loaded.Summary.AffectedCommits != 3 {
		t.Errorf("loaded AffectedCommits = %d, want 3", loaded.Summary.AffectedCommits)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	setTestDataDir(t)

	dir, _ := DataDir()
	// Ensure the directory doesn't exist yet
	if _, err := os.Stat(dir); err == nil {
		t.Skip("directory already exists")
	}

	rpt := &Report{
		ID:      "clm_mkdir1",
		Version: "1.0.0",
		Repo:    "/tmp/repo",
	}
	if err := rpt.Save(); err != nil {
		t.Fatalf("Save() should create directory: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected directory %q to exist after Save()", dir)
	}
}

func TestList(t *testing.T) {
	setTestDataDir(t)

	// Save reports for two different repos
	now := time.Now().UTC()
	reports := []*Report{
		{ID: "clm_list01", Repo: "/tmp/repo-a", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "clm_list02", Repo: "/tmp/repo-a", CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "clm_list03", Repo: "/tmp/repo-b", CreatedAt: now},
	}
	for _, r := range reports {
		if err := r.Save(); err != nil {
			t.Fatalf("Save() failed: %v", err)
		}
	}

	// List all
	all, err := List("")
	if err != nil {
		t.Fatalf("List('') failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List('') returned %d reports, want 3", len(all))
	}
	// Should be sorted newest first
	if len(all) >= 2 && all[0].CreatedAt.Before(all[1].CreatedAt) {
		t.Error("List() should return reports sorted newest first")
	}

	// List filtered by repo
	repoA, err := List("/tmp/repo-a")
	if err != nil {
		t.Fatalf("List('/tmp/repo-a') failed: %v", err)
	}
	if len(repoA) != 2 {
		t.Errorf("List('/tmp/repo-a') returned %d reports, want 2", len(repoA))
	}

	repoB, err := List("/tmp/repo-b")
	if err != nil {
		t.Fatalf("List('/tmp/repo-b') failed: %v", err)
	}
	if len(repoB) != 1 {
		t.Errorf("List('/tmp/repo-b') returned %d reports, want 1", len(repoB))
	}
}

func TestListAll(t *testing.T) {
	setTestDataDir(t)

	rpt := &Report{ID: "clm_all001", Repo: "/tmp/repo"}
	if err := rpt.Save(); err != nil {
		t.Fatal(err)
	}

	all, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll() failed: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("ListAll() returned %d, want 1", len(all))
	}
}

func TestListEmptyDir(t *testing.T) {
	setTestDataDir(t)

	// DataDir doesn't exist yet — should return nil, nil
	reports, err := List("")
	if err != nil {
		t.Fatalf("List() on empty dir should not error: %v", err)
	}
	if reports != nil {
		t.Errorf("List() on empty dir should return nil, got %d reports", len(reports))
	}
}

func TestFindByPrefix(t *testing.T) {
	setTestDataDir(t)

	reports := []*Report{
		{ID: "clm_abc123", Repo: "/tmp/repo"},
		{ID: "clm_def456", Repo: "/tmp/repo"},
		{ID: "clm_abc789", Repo: "/tmp/repo"},
	}
	for _, r := range reports {
		if err := r.Save(); err != nil {
			t.Fatal(err)
		}
	}

	// Exact match
	rpt, err := FindByPrefix("", "clm_def456")
	if err != nil {
		t.Fatalf("FindByPrefix() exact: %v", err)
	}
	if rpt.ID != "clm_def456" {
		t.Errorf("expected clm_def456, got %q", rpt.ID)
	}

	// Unique prefix
	rpt, err = FindByPrefix("", "clm_def")
	if err != nil {
		t.Fatalf("FindByPrefix() unique prefix: %v", err)
	}
	if rpt.ID != "clm_def456" {
		t.Errorf("expected clm_def456, got %q", rpt.ID)
	}

	// Ambiguous prefix
	_, err = FindByPrefix("", "clm_abc")
	if err == nil {
		t.Error("FindByPrefix() should fail for ambiguous prefix")
	}

	// No match
	_, err = FindByPrefix("", "clm_zzz")
	if err == nil {
		t.Error("FindByPrefix() should fail for unknown prefix")
	}
}

func TestFindByPrefixWithRepoFilter(t *testing.T) {
	setTestDataDir(t)

	r1 := &Report{ID: "clm_filter1", Repo: "/tmp/repo-x"}
	r2 := &Report{ID: "clm_filter2", Repo: "/tmp/repo-y"}
	for _, r := range []*Report{r1, r2} {
		if err := r.Save(); err != nil {
			t.Fatal(err)
		}
	}

	// Filter by repo-x should only find r1
	rpt, err := FindByPrefix("/tmp/repo-x", "clm_filter")
	if err != nil {
		t.Fatalf("FindByPrefix with filter: %v", err)
	}
	if rpt.ID != "clm_filter1" {
		t.Errorf("expected clm_filter1, got %q", rpt.ID)
	}

	// Filter by repo-y should only find r2
	rpt, err = FindByPrefix("/tmp/repo-y", "clm_filter")
	if err != nil {
		t.Fatalf("FindByPrefix with filter: %v", err)
	}
	if rpt.ID != "clm_filter2" {
		t.Errorf("expected clm_filter2, got %q", rpt.ID)
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()

	rpt := &Report{
		ID:      "clm_load01",
		Version: "2.0.0",
		Repo:    "/tmp/repo",
	}
	data, _ := json.MarshalIndent(rpt, "", "  ")
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile() failed: %v", err)
	}
	if loaded.ID != "clm_load01" {
		t.Errorf("loaded ID = %q, want clm_load01", loaded.ID)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := loadFile("/nonexistent/path.json")
	if err == nil {
		t.Error("loadFile() should fail for missing file")
	}
}

func TestLoadFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadFile(path)
	if err == nil {
		t.Error("loadFile() should fail for invalid JSON")
	}
}

func TestListSkipsInvalidFiles(t *testing.T) {
	setTestDataDir(t)

	dir, _ := DataDir()
	os.MkdirAll(dir, 0o755)

	// Write a valid report
	valid := &Report{ID: "clm_valid1", Repo: "/tmp/repo"}
	data, _ := json.MarshalIndent(valid, "", "  ")
	os.WriteFile(filepath.Join(dir, "clm_valid1.json"), data, 0o644)

	// Write invalid JSON
	os.WriteFile(filepath.Join(dir, "clm_bad.json"), []byte("not json"), 0o644)

	// Write a non-json file
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644)

	// Create a subdirectory
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	reports, err := List("")
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(reports) != 1 {
		t.Errorf("List() should return 1 valid report, got %d", len(reports))
	}
	if reports[0].ID != "clm_valid1" {
		t.Errorf("expected clm_valid1, got %q", reports[0].ID)
	}
}
