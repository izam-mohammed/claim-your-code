package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

func TestFatalPrintsAndExits(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim"}, func() {
		fatal(errors.New("something broke"))
	})
	if !exited || code != 1 {
		t.Errorf("fatal did not exit with 1: (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "something broke") {
		t.Errorf("fatal printed %q", out)
	}
}

func TestFatalfFormats(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim"}, func() {
		fatalf("cannot read %q (attempt %d)", "/tmp/x", 2)
	})
	if !exited || code != 1 {
		t.Errorf("fatalf did not exit with 1: (%d, %v)", code, exited)
	}
	if !strings.Contains(out, `cannot read "/tmp/x" (attempt 2)`) {
		t.Errorf("fatalf printed %q", out)
	}
}

func TestFormatDiffStat(t *testing.T) {
	tests := []struct {
		name     string
		ins, del int
		want     string
	}{
		{"nothing", 0, 0, ""},
		{"insertions only", 7, 0, " +7"},
		{"deletions only", 0, 3, " -3"},
		{"both", 7, 3, " +7/-3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDiffStat(tt.ins, tt.del); got != tt.want {
				t.Errorf("formatDiffStat(%d, %d) = %q, want %q", tt.ins, tt.del, got, tt.want)
			}
		})
	}
}

func TestPrintProgressDeterminate(t *testing.T) {
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printProgress(5, 10, "scanning", time.Now().Add(-2*time.Second))
	})
	if !strings.Contains(out, "50%") {
		t.Errorf("progress bar = %q, want it to show 50%%", out)
	}
	if !strings.Contains(out, "5/10") {
		t.Errorf("progress bar = %q, want it to show the counts", out)
	}
	if !strings.Contains(out, "left") {
		t.Errorf("progress bar = %q, want an ETA part-way through", out)
	}
}

func TestPrintProgressIndeterminate(t *testing.T) {
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printProgress(120, 0, "120 commits scanned", time.Now())
	})
	if !strings.Contains(out, "120 commits scanned") {
		t.Errorf("indeterminate progress = %q", out)
	}
	if strings.Contains(out, "%") {
		t.Errorf("indeterminate progress should not show a percentage: %q", out)
	}
}

func TestPrintProgressComplete(t *testing.T) {
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printProgress(10, 10, "done", time.Now().Add(-time.Second))
	})
	if !strings.Contains(out, "100%") {
		t.Errorf("progress = %q, want 100%%", out)
	}
	if strings.Contains(out, "left") {
		t.Errorf("a finished bar should not show an ETA: %q", out)
	}
}

func TestPrintProgressTruncatesLongLabels(t *testing.T) {
	long := strings.Repeat("x", 60)
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printProgress(1, 2, long, time.Now())
	})
	if strings.Contains(out, long) {
		t.Error("a long label should be truncated")
	}
	if !strings.Contains(out, "...") {
		t.Errorf("a truncated label should end in an ellipsis: %q", out)
	}
}

func TestPrintProgressOverflowClampsTheBar(t *testing.T) {
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printProgress(15, 10, "over", time.Now())
	})
	if strings.Count(out, "█") > 25 {
		t.Errorf("the bar overflowed its width: %q", out)
	}
}

func TestRunConcurrentPreservesInputOrder(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	var results []int
	runMain(t, []string{"claim"}, func() {
		results = runConcurrent(items,
			func(i int) string { return "item" },
			func(i int) int { return i * 10 })
	})

	if len(results) != len(items) {
		t.Fatalf("got %d results, want %d", len(results), len(items))
	}
	for i, want := range []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100} {
		if results[i] != want {
			t.Errorf("results[%d] = %d, want %d — order must match the input", i, results[i], want)
		}
	}
}

func TestRunConcurrentBoundsInFlightWork(t *testing.T) {
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}
	inFlight, peak := 0, 0

	items := make([]int, 25)
	runMain(t, []string{"claim"}, func() {
		runConcurrent(items,
			func(int) string { return "x" },
			func(int) int {
				<-mu
				inFlight++
				if inFlight > peak {
					peak = inFlight
				}
				mu <- struct{}{}

				time.Sleep(time.Millisecond)

				<-mu
				inFlight--
				mu <- struct{}{}
				return 0
			})
	})

	if peak > maxWorkers {
		t.Errorf("peak concurrency was %d, want at most %d", peak, maxWorkers)
	}
}

func TestRunConcurrentEmptyInput(t *testing.T) {
	var results []int
	runMain(t, []string{"claim"}, func() {
		results = runConcurrent([]int{}, func(int) string { return "" }, func(i int) int { return i })
	})
	if len(results) != 0 {
		t.Errorf("runConcurrent on no items = %v, want empty", results)
	}
}

func TestSummariseReposAllClean(t *testing.T) {
	var proceed bool
	out, _, _ := runMain(t, []string{"claim"}, func() {
		proceed = summariseRepos(3, nil)
	})
	if proceed {
		t.Error("summariseRepos should report there is nothing to do")
	}
	if !strings.Contains(out, "3 repo(s) clean") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "No Claude co-authorship found") {
		t.Errorf("output = %q", out)
	}
}

func TestSummariseReposWithAffectedRepos(t *testing.T) {
	var proceed bool
	out, _, _ := runMain(t, []string{"claim"}, func() {
		proceed = summariseRepos(1, []affectedRepo{
			{label: "owner/one", commits: 4, insertions: 40, deletions: 5},
			{label: "owner/two", commits: 2, note: "on main"},
		})
	})
	if !proceed {
		t.Error("summariseRepos should report there is work to do")
	}
	for _, want := range []string{
		"1 repo(s) clean",
		"6 commit(s) co-authored by Claude",
		"2 repo(s)",
		"owner/one", "4 commit(s)", "+40/-5",
		"owner/two", "on main",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary is missing %q\n---\n%s", want, out)
		}
	}
}

func TestSummariseReposNoCleanCountIsOmitted(t *testing.T) {
	out, _, _ := runMain(t, []string{"claim"}, func() {
		summariseRepos(0, []affectedRepo{{label: "o/r", commits: 1}})
	})
	if strings.Contains(out, "clean") {
		t.Errorf("with no clean repos the clean line should be omitted: %q", out)
	}
}

func TestConfirmDefaultsToNoWithoutATerminal(t *testing.T) {
	// The real prompts, not the stubs — with no TTY they must answer safely.
	stubInteractive(t, false)
	if confirm("proceed?") {
		t.Error("confirm returned true with no terminal — the safe answer is no")
	}
	if confirmDangerous("  ") {
		t.Error("confirmDangerous returned true with no terminal — the safe answer is no")
	}
}

func TestHasTerminalSaysNoForARedirectedStdin(t *testing.T) {
	// The real check this time, not the seam the other tests drive.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = prev
		r.Close()
		w.Close()
	})

	if hasTerminal() {
		t.Error("hasTerminal = true with a pipe on stdin, want no terminal")
	}
}

func TestSelectPromptsErrorWithoutATerminal(t *testing.T) {
	stubInteractive(t, false)
	if _, err := selectOne("pick", nil, 0); err == nil {
		t.Error("selectOne should fail with no terminal")
	}
	if _, err := selectMany("pick", nil, 5); err == nil {
		t.Error("selectMany should fail with no terminal")
	}
}

func TestPrintScanResultsAndBranchSummaries(t *testing.T) {
	scan := &scanner.ScanOutput{
		TotalCommits: 30,
		Results: []scanner.Result{
			{Hash: "aaaaaaaabbbbbbbb", Subject: "feat: one", Model: "Claude Opus 4.6", Insertions: 10, Deletions: 1},
			{Hash: "bbbbbbbbcccccccc", Subject: "fix: two"},
		},
		BranchSummaries: []scanner.BranchSummary{{
			Branch: "main", Count: 2, Insertions: 10, Deletions: 1,
			Models:     map[string]int{"Claude Opus 4.6": 1},
			ModelStats: map[string]*scanner.ModelStat{"Claude Opus 4.6": {Count: 1, Insertions: 10, Deletions: 1}},
		}},
	}

	out, _, _ := runMain(t, []string{"claim"}, func() {
		printScanResults(scan, scan.Results)
	})

	for _, want := range []string{
		"2 commit(s) co-authored by Claude", "30 total commits",
		"Branches:", "main", "(2 commit(s))", "+10/-1",
		"Claude Opus 4.6",
		"Commits:", "aaaaaaaa", "feat: one", "bbbbbbbb", "fix: two",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("scan output is missing %q\n---\n%s", want, out)
		}
	}
}

func TestPrintBranchesAndCommitsTruncatesLongLists(t *testing.T) {
	var results []scanner.Result
	for i := 0; i < 14; i++ {
		results = append(results, scanner.Result{
			Hash:    strings.Repeat(string(rune('a'+i)), 40),
			Subject: "commit",
		})
	}
	scan := &scanner.ScanOutput{TotalCommits: 14, Results: results}

	out, _, _ := runMain(t, []string{"claim"}, func() {
		printBranchesAndCommits(scan, results)
	})
	if !strings.Contains(out, "... and 4 more") {
		t.Errorf("a list of 14 commits should be truncated at 10:\n%s", out)
	}
}

func TestPrintCompactBranchSummary(t *testing.T) {
	scan := &scanner.ScanOutput{
		Results: []scanner.Result{
			{Hash: "a", Model: "Claude Opus 4.6"},
			{Hash: "b", Model: "Claude Opus 4.6"},
			{Hash: "c", Model: ""},
		},
		BranchSummaries: []scanner.BranchSummary{
			{Branch: "main", Count: 2, Insertions: 5},
			{Branch: "dev", Count: 1},
		},
	}
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printCompactBranchSummary(scan)
	})
	for _, want := range []string{"Models:", "Claude Opus 4.6", "× 2", "Branches:", "(2 affected)", "main", "dev"} {
		if !strings.Contains(out, want) {
			t.Errorf("compact summary is missing %q\n---\n%s", want, out)
		}
	}
}

func TestPrintCompactBranchSummaryTruncatesBranches(t *testing.T) {
	var summaries []scanner.BranchSummary
	for i := 0; i < 8; i++ {
		summaries = append(summaries, scanner.BranchSummary{Branch: "branch", Count: 1})
	}
	scan := &scanner.ScanOutput{BranchSummaries: summaries}
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printCompactBranchSummary(scan)
	})
	if !strings.Contains(out, "... and 3 more branches") {
		t.Errorf("8 branches should be truncated at 5:\n%s", out)
	}
}

func TestPrintProgressTruncatesOnRuneBoundaries(t *testing.T) {
	// Byte-slicing a label split multi-byte characters and printed mojibake.
	label := strings.Repeat("日", 30)
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printProgress(1, 2, label, time.Now())
	})
	if !utf8.ValidString(out) {
		t.Errorf("progress output is not valid UTF-8: %q", out)
	}
	if strings.Contains(out, "�") {
		t.Errorf("truncation produced a replacement character: %q", out)
	}
	if !strings.Contains(out, strings.Repeat("日", 25)+"...") {
		t.Errorf("expected 25 runes then an ellipsis, got %q", out)
	}
}

func TestPrintProgressKeepsShortUnicodeLabelsIntact(t *testing.T) {
	out, _, _ := runMain(t, []string{"claim"}, func() {
		printProgress(1, 2, "café ☕", time.Now())
	})
	if !strings.Contains(out, "café ☕") {
		t.Errorf("a short label should be printed unchanged: %q", out)
	}
}
