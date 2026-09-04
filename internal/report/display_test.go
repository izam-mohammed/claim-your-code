package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
)

// capture runs fn with stdout redirected and returns what it printed.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	prevNoColor := color.NoColor
	color.NoColor = true // compare plain text, not escape codes
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()

	w.Close()
	os.Stdout = orig
	color.NoColor = prevNoColor
	return <-done
}

// stubSelect makes the report picker return the given ID without a terminal.
// The returned pointer receives the option labels the picker was offered.
func stubSelect(t *testing.T, id string, err error) *[]string {
	t.Helper()
	var labels []string
	prev := selectReport
	selectReport = func(opts []huh.Option[string]) (string, error) {
		labels = labels[:0]
		for _, o := range opts {
			labels = append(labels, o.Key)
		}
		return id, err
	}
	t.Cleanup(func() { selectReport = prev })
	return &labels
}

func sampleReport() *Report {
	r := &Report{
		ID:        "clm_abc123",
		Version:   "v1",
		Repo:      "/home/izam/code/my-project",
		CreatedAt: time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
		Summary: Summary{
			TotalCommits:    45,
			AffectedCommits: 2,
			Models:          map[string]int{"Claude Opus 4.6": 2},
		},
		Branches: []Branch{{Name: "main", Count: 2, Models: map[string]int{"Claude Opus 4.6": 2}}},
		Commits: []Commit{
			{Hash: "a1b2c3d4e5f6", Subject: "feat: one", Model: "Claude Opus 4.6", Branches: []string{"main"}, Insertions: 10, Deletions: 2},
			{Hash: "b2c3d4e5f6a1", Subject: "fix: two", Model: "Claude Opus 4.6", Branches: []string{"main"}},
		},
	}
	return r
}

func TestStatusIconAndLabel(t *testing.T) {
	color.NoColor = true
	tests := []struct {
		status    string
		wantIcon  string
		wantLabel string
	}{
		{"cleaned", "✓", "cleaned"},
		{"dry_run", "◎", "dry run"},
		{"aborted", "✗", "aborted"},
		{"pending", "?", "unknown"},
		{"", "?", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := statusIcon(tt.status); got != tt.wantIcon {
				t.Errorf("statusIcon(%q) = %q, want %q", tt.status, got, tt.wantIcon)
			}
			if got := statusLabel(tt.status); got != tt.wantLabel {
				t.Errorf("statusLabel(%q) = %q, want %q", tt.status, got, tt.wantLabel)
			}
		})
	}
}

func TestRenderBar(t *testing.T) {
	color.NoColor = true
	tests := []struct {
		name       string
		pct        float64
		width      int
		wantFilled int
		wantPct    string
	}{
		{"zero", 0, 20, 0, "0%"},
		{"half", 50, 20, 10, "50%"},
		{"full", 100, 20, 20, "100%"},
		{"over 100 clamps", 150, 20, 20, "150%"},
		{"tiny but non-zero shows one block", 1, 20, 1, "1%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderBar(tt.pct, tt.width)
			if strings.Count(got, "█") != tt.wantFilled {
				t.Errorf("renderBar(%v) = %q, want %d filled blocks", tt.pct, got, tt.wantFilled)
			}
			if !strings.HasSuffix(got, tt.wantPct) {
				t.Errorf("renderBar(%v) = %q, want it to end in %q", tt.pct, got, tt.wantPct)
			}
			if strings.Count(got, "█")+strings.Count(got, "░") != tt.width {
				t.Errorf("renderBar(%v) = %q, want %d cells total", tt.pct, got, tt.width)
			}
		})
	}
}

func TestFormatDiffStat(t *testing.T) {
	color.NoColor = true
	tests := []struct {
		name     string
		ins, del int
		want     string
	}{
		{"nothing", 0, 0, ""},
		{"insertions only", 12, 0, " +12"},
		{"deletions only", 0, 4, " -4"},
		{"both", 12, 4, " +12/-4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDiffStat(tt.ins, tt.del); got != tt.want {
				t.Errorf("formatDiffStat(%d, %d) = %q, want %q", tt.ins, tt.del, got, tt.want)
			}
		})
	}
}

func TestPrintDetailLocalReport(t *testing.T) {
	r := sampleReport()
	r.SetOriginalRefs(map[string]string{"main": "aaa"})
	r.SetResult("cleaned", 2)

	out := capture(t, func() { PrintDetail(r) })

	for _, want := range []string{
		"clm_abc123",
		"/home/izam/code/my-project",
		"cleaned",
		"45", "2",
		"Claude Opus 4.6",
		"main",
		"a1b2c3d4", "feat: one", "+10/-2",
		"b2c3d4e5", "fix: two",
		"claim revert clm_abc123",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("PrintDetail output is missing %q\n---\n%s", want, out)
		}
	}
}

func TestPrintDetailRemotePRReport(t *testing.T) {
	r := sampleReport()
	r.RemoteURL = "https://github.com/izam-mohammed/claim-your-code"
	r.RemoteOwner = "izam-mohammed"
	r.RemoteRepo = "claim-your-code"
	r.PRNumber = 42
	r.PRTitle = "clean up history"
	r.PRBranch = "feature/cleanup"
	r.SetResult("dry_run", 0)

	out := capture(t, func() { PrintDetail(r) })

	for _, want := range []string{
		"https://github.com/izam-mohammed/claim-your-code",
		"#42", "clean up history", "feature/cleanup", "dry run",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("PrintDetail output is missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "/home/izam/code/my-project") {
		t.Error("a remote report should show the remote URL, not the local path")
	}
}

func TestPrintDetailShowsPushSuccess(t *testing.T) {
	r := sampleReport()
	r.SetResult("cleaned", 2)
	r.SetPushed()

	out := capture(t, func() { PrintDetail(r) })
	if !strings.Contains(out, "Pushed:") {
		t.Errorf("PrintDetail did not report the push\n---\n%s", out)
	}
}

func TestPrintDetailShowsPushError(t *testing.T) {
	r := sampleReport()
	r.SetResult("cleaned", 2)
	r.SetPushError("remote rejected the update")

	out := capture(t, func() { PrintDetail(r) })
	if !strings.Contains(out, "remote rejected the update") {
		t.Errorf("PrintDetail did not surface the push failure\n---\n%s", out)
	}
}

func TestPrintDetailRevertedReport(t *testing.T) {
	r := sampleReport()
	r.SetOriginalRefs(map[string]string{"main": "aaa"})
	r.SetResult("cleaned", 2)
	r.SetReverted()

	out := capture(t, func() { PrintDetail(r) })
	if !strings.Contains(out, "reverted") {
		t.Errorf("PrintDetail did not mark the report reverted\n---\n%s", out)
	}
	if strings.Contains(out, "claim revert clm_abc123") {
		t.Error("an already-reverted report should not offer a revert command")
	}
}

func TestPrintDetailPendingReport(t *testing.T) {
	r := sampleReport() // no Result set
	out := capture(t, func() { PrintDetail(r) })
	if !strings.Contains(out, "unknown") {
		t.Errorf("a report with no result should render as unknown\n---\n%s", out)
	}
}

func TestPrintDetailCommitWithoutModelOrBranches(t *testing.T) {
	r := sampleReport()
	r.Commits = []Commit{{Hash: "0123456789ab", Subject: "chore: bare"}}
	out := capture(t, func() { PrintDetail(r) })
	if !strings.Contains(out, "chore: bare") {
		t.Errorf("PrintDetail dropped a commit with no model or branches\n---\n%s", out)
	}
}

func TestPrintListAndSelectEmpty(t *testing.T) {
	out := capture(t, func() { PrintListAndSelect(nil) })
	if !strings.Contains(out, "No claim reports found") {
		t.Errorf("PrintListAndSelect(nil) printed %q", out)
	}
}

func TestPrintListAndSelectShowsEveryReport(t *testing.T) {
	labels := stubSelect(t, "", nil) // no selection

	local := sampleReport()
	local.SetResult("cleaned", 2)

	remote := sampleReport()
	remote.ID = "clm_remote"
	remote.RemoteOwner = "izam-mohammed"
	remote.RemoteRepo = "claim-your-code"
	remote.RemoteURL = "https://github.com/izam-mohammed/claim-your-code"
	remote.PRNumber = 7

	reverted := sampleReport()
	reverted.ID = "clm_undone"
	reverted.SetResult("cleaned", 1)
	reverted.SetReverted()

	out := capture(t, func() { PrintListAndSelect([]*Report{local, remote, reverted}) })

	if !strings.Contains(out, "Claim Reports (3)") {
		t.Errorf("header is missing from %q", out)
	}

	// The per-report detail lives in the picker's option labels.
	joined := strings.Join(*labels, "\n")
	for _, want := range []string{
		"clm_abc123", "my-project",
		"clm_remote", "izam-mohammed/claim-your-code#7",
		"clm_undone", "(reverted)",
		"Claude Opus 4.6×2",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("report list option labels are missing %q\n---\n%s", want, joined)
		}
	}
	if len(*labels) != 3 {
		t.Errorf("picker was offered %d options, want 3", len(*labels))
	}
}

func TestPrintListAndSelectRendersTheChosenReport(t *testing.T) {
	stubSelect(t, "clm_abc123", nil)
	r := sampleReport()

	out := capture(t, func() { PrintListAndSelect([]*Report{r}) })
	if !strings.Contains(out, "Claim Report") || !strings.Contains(out, "feat: one") {
		t.Errorf("selecting a report did not print its detail view\n---\n%s", out)
	}
}

func TestPrintListAndSelectUnknownSelectionPrintsNoDetail(t *testing.T) {
	stubSelect(t, "clm_not_in_list", nil)
	out := capture(t, func() { PrintListAndSelect([]*Report{sampleReport()}) })
	if strings.Contains(out, "Summary") {
		t.Errorf("an ID matching no report should not render a detail view\n---\n%s", out)
	}
}

func TestPrintListAndSelectCancelled(t *testing.T) {
	stubSelect(t, "", fmt.Errorf("cancelled"))
	out := capture(t, func() { PrintListAndSelect([]*Report{sampleReport()}) })
	if strings.Contains(out, "Summary") {
		t.Errorf("a cancelled picker should not render a detail view\n---\n%s", out)
	}
}
