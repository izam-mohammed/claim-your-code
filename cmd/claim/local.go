// The local flow: find repositories under a folder, scan them, and rewrite
// the ones that need it.

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/izam-mohammed/claim-your-code/internal/discover"
	"github.com/izam-mohammed/claim-your-code/internal/filter"
	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/rewriter"
	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

func runFilterMsg() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("error:"), err)
		os.Exit(1)
	}
	fmt.Print(filter.StripClaudeCoAuthor(string(input)))
}

// resolveRepos takes a folder path and returns the list of repo paths to process.
// If the folder is a git repo, it returns that repo (and optionally nested repos).
// If not, it searches for repos inside.

// resolveRepos takes a folder path and returns the list of repo paths to process.
// If the folder is a git repo, it returns that repo (and optionally nested repos).
// If not, it searches for repos inside.
func resolveRepos(absPath string, force bool) []string {
	if discover.IsGitRepo(absPath) {
		repos := []string{absPath}

		// Check for nested repos
		nested := discover.FindNestedRepos(absPath)
		if len(nested) > 0 && !force {
			fmt.Printf("\n%s Found %s inside %s\n",
				yellow("!"),
				bold(fmt.Sprintf("%d nested repo(s)", len(nested))),
				bold(filepath.Base(absPath)))

			var include bool
			_ = huh.NewConfirm().
				Title("Include nested repos?").
				Affirmative("Yes").
				Negative("No").
				Value(&include).
				Run()
			if include {
				for _, r := range nested {
					repos = append(repos, r.Path)
				}
			}
		} else if len(nested) > 0 && force {
			for _, r := range nested {
				repos = append(repos, r.Path)
			}
		}

		return repos
	}

	// Not a git repo — search for repos inside
	fmt.Printf("%s No .git found in %s, searching for repositories...\n", cyan("::"), bold(filepath.Base(absPath)))
	found := discover.FindRepos(absPath, 4) // search up to 4 levels deep

	if len(found) == 0 {
		fatalf("No git repositories found in '%s'", absPath)
	}

	fmt.Printf("\n%s Found %s:\n", green("✓"), bold(fmt.Sprintf("%d repo(s)", len(found))))

	if force {
		var paths []string
		for _, r := range found {
			paths = append(paths, r.Path)
		}
		return paths
	}

	// "All repos" at the top, then individual repos
	options := make([]huh.Option[string], 0, len(found)+1)
	options = append(options, huh.NewOption(fmt.Sprintf("All %d repos", len(found)), "__all__"))
	for _, r := range found {
		relPath, _ := filepath.Rel(absPath, r.Path)
		if relPath == "" {
			relPath = r.Path
		}
		options = append(options, huh.NewOption(r.Name+" — "+relPath, r.Path))
	}

	var selected string
	err := huh.NewSelect[string]().
		Title("Select a repo").
		Options(options...).
		Height(15).
		Value(&selected).
		Run()
	if err != nil || selected == "" {
		fmt.Printf("\n%s Aborted.\n", red("✗"))
		os.Exit(0)
	}

	if selected == "__all__" {
		var paths []string
		for _, r := range found {
			paths = append(paths, r.Path)
		}
		return paths
	}

	return []string{selected}
}

// --- CLAIM ---

func runClaim(folder string) {
	dryRun := false
	force := false
	for _, arg := range os.Args[2:] {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--force", "-f":
			force = true
		}
	}

	absPath, err := filepath.Abs(folder)
	if err != nil {
		fatal(err)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		fatalf("'%s' is not a valid directory", folder)
	}

	repos := resolveRepos(absPath, force)

	// Ask branch scope once for all repos
	defaultOnly := false
	if !force {
		var scope string
		title := "Which branches to scan?"
		if len(repos) == 1 {
			title = fmt.Sprintf("Which branches to scan in %s?", filepath.Base(repos[0]))
		}
		_ = huh.NewSelect[string]().
			Title(title).
			Options(
				huh.NewOption("Default branch only (faster)", "default"),
				huh.NewOption("All branches (thorough)", "all"),
			).
			Value(&scope).
			Run()
		defaultOnly = scope == "default"
	}

	if len(repos) == 1 {
		claimRepo(repos[0], dryRun, force, defaultOnly)
		return
	}

	// Multi-repo: scan all first, then rewrite
	branchMode := "all branches"
	if defaultOnly {
		branchMode = "default branch"
	}
	fmt.Printf("\n%s Scanning %d repo(s) %s...\n", cyan("::"), len(repos), dim("("+branchMode+")"))

	type scanResult struct {
		path string
		name string
		scan *scanner.ScanOutput
		err  error
	}

	results := make([]scanResult, len(repos))
	start := time.Now()
	for i, repoPath := range repos {
		name := filepath.Base(repoPath)
		printProgress(i, len(repos), name, start)
		scan, err := scanner.ScanWithProgress(repoPath, defaultOnly, nil)
		results[i] = scanResult{path: repoPath, name: name, scan: scan, err: err}
	}
	printProgress(len(repos), len(repos), "done", start)
	fmt.Println()

	// Separate clean from affected, and show the same summary the remote
	// multi-repo scans show.
	var affected []scanResult
	var summary []affectedRepo
	cleanCount := 0
	for _, r := range results {
		if r.err != nil || len(r.scan.Results) == 0 {
			cleanCount++
			continue
		}
		affected = append(affected, r)
		ins, del := 0, 0
		for _, c := range r.scan.Results {
			ins += c.Insertions
			del += c.Deletions
		}
		summary = append(summary, affectedRepo{
			label: r.name, commits: len(r.scan.Results), insertions: ins, deletions: del,
		})
	}

	if !summariseRepos(cleanCount, summary) {
		return
	}

	totalCommits := 0
	for _, r := range affected {
		totalCommits += len(r.scan.Results)
	}

	if dryRun {
		for _, r := range affected {
			rpt := report.Build(version, r.path, r.scan.TotalCommits, r.scan.Results, r.scan.BranchSummaries)
			rpt.SetResult("dry_run", 0)
			_ = rpt.Save()
		}
		fmt.Printf("\n%s No changes made.\n", yellow("--dry-run:"))
		return
	}

	if !force {
		if !confirm(fmt.Sprintf("Rewrite %d commit(s) across %d repo(s)?", totalCommits, len(affected))) {
			return
		}
	}

	// Rewrite all affected repos
	for i, r := range affected {
		if i > 0 {
			fmt.Println(dim("  ───"))
		}
		rpt := report.Build(version, r.path, r.scan.TotalCommits, r.scan.Results, r.scan.BranchSummaries)
		originalRefs, _ := rewriter.GetBranchRefs(r.path)
		rpt.SetOriginalRefs(originalRefs)

		var branches []string
		if defaultOnly {
			for _, bs := range r.scan.BranchSummaries {
				branches = append(branches, bs.Branch)
			}
		}
		branchLbl := "all branches"
		if len(branches) > 0 {
			branchLbl = fmt.Sprintf("%d branch(es)", len(branches))
		}
		fmt.Printf("  %s Rewriting %s %s...\n", cyan("::"), bold(r.name), dim("("+branchLbl+")"))
		if err := rewriter.RewriteBranches(r.path, branches); err != nil {
			fmt.Fprintf(os.Stderr, "  %s %s: %v\n", red("✗"), r.name, err)
			continue
		}

		afterRefs, _ := rewriter.GetBranchRefs(r.path)
		rpt.SetAfterRefs(afterRefs)
		rpt.SetResult("cleaned", len(r.scan.Results))
		_ = rpt.Save()
		fmt.Printf("  %s Cleaned %s %s\n", green("✓"), bold(r.name), dim(fmt.Sprintf("(%d commits)", len(r.scan.Results))))
		promptPush(r.path, rpt, originalRefs, afterRefs)
	}
}

func claimRepo(repoPath string, dryRun, force, defaultOnly bool) {
	repoName := filepath.Base(repoPath)

	branchMode := "all branches"
	if defaultOnly {
		branchMode = "default branch"
	}
	fmt.Printf("%s Scanning %s %s\n", cyan("::"), bold(repoName), dim("("+branchMode+")"))

	scanStart := time.Now()
	progress := func(done, total int, label string) {
		printProgress(done, total, label, scanStart)
	}

	scan, err := scanner.ScanWithProgress(repoPath, defaultOnly, progress)
	fmt.Print("\r\033[K") // clear progress line
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		return
	}

	results := scan.Results

	if len(results) == 0 {
		fmt.Printf("  %s No Claude co-authorship found.\n", green("✓"))
		return
	}

	rpt := report.Build(version, repoPath, scan.TotalCommits, results, scan.BranchSummaries)

	branchCount := len(scan.BranchSummaries)
	fmt.Printf("\n%s %s co-authored by Claude %s\n",
		yellow("!"),
		bold(fmt.Sprintf("%d unique commit(s)", len(results))),
		dim(fmt.Sprintf("· %d total commits · %d branches", scan.TotalCommits, branchCount)))

	printCompactBranchSummary(scan)

	if dryRun {
		rpt.SetResult("dry_run", 0)
		if err := rpt.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "%s Failed to record report: %v\n", yellow("Warning:"), err)
		} else {
			fmt.Printf("\n%s Report tracked %s\n", dim("→"), boldCyan(rpt.ID))
		}
		fmt.Printf("\n%s No changes made.\n", yellow("--dry-run:"))
		return
	}

	if !force {
		fmt.Printf("\n%s Will scan %s across %s to rewrite %s in %s\n",
			boldRed("⚠"),
			dim(fmt.Sprintf("%d commits", scan.TotalCommits)),
			dim(fmt.Sprintf("%d branch(es)", len(scan.BranchSummaries))),
			bold(fmt.Sprintf("%d affected commit(s)", len(results))),
			bold(repoName))
		if !confirm("  Proceed?") {
			rpt.SetResult("aborted", 0)
			if err := rpt.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "%s Failed to record report: %v\n", yellow("Warning:"), err)
			}
			fmt.Printf("\n%s Aborted.\n", red("✗"))
			return
		}
	}

	// Capture branch refs before rewriting (for revert)
	originalRefs, err := rewriter.GetBranchRefs(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to capture branch refs: %v\n", yellow("Warning:"), err)
	}
	rpt.SetOriginalRefs(originalRefs)

	// Rewrite only the branches that were scanned
	// If scanned all branches and there are many, use --all for efficiency
	var rewriteBranches []string
	if !defaultOnly {
		// All branches mode — use --all
		rewriteBranches = nil
	} else {
		for _, bs := range scan.BranchSummaries {
			rewriteBranches = append(rewriteBranches, bs.Branch)
		}
	}

	branchLabel := "all branches"
	if len(rewriteBranches) > 0 {
		branchLabel = fmt.Sprintf("%d branch(es)", len(rewriteBranches))
	}
	fmt.Printf("\n%s Rewriting commit messages %s...\n", cyan("::"), dim("("+branchLabel+")"))
	if err := rewriter.RewriteBranches(repoPath, rewriteBranches); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		return
	}

	afterRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetAfterRefs(afterRefs)

	rpt.SetResult("cleaned", len(results))
	if err := rpt.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to record report: %v\n", yellow("Warning:"), err)
	} else {
		fmt.Printf("\n%s Report tracked %s\n", dim("→"), boldCyan(rpt.ID))
	}

	fmt.Printf("\n%s Cleaned %s\n",
		green("✓"),
		bold(fmt.Sprintf("%d commit(s)", len(results))))

	// Offer to push if repo has a remote
	promptPush(repoPath, rpt, originalRefs, afterRefs)
}

// promptPush offers to force-push rewritten branches to remote.

// promptPush offers to force-push rewritten branches to remote.
func promptPush(repoPath string, rpt *report.Report, beforeRefs, afterRefs map[string]string) {
	remoteURL := rewriter.GetRemoteURL(repoPath)
	if remoteURL == "" {
		fmt.Printf("\n%s No remote found. When you add one, push with:\n", yellow("→"))
		fmt.Printf("  %s\n", cyan("git push --force-with-lease --all"))
		return
	}

	changed := rewriter.ChangedBranches(beforeRefs, afterRefs)
	if len(changed) == 0 {
		return
	}

	fmt.Printf("\n%s Push rewritten branches to %s?\n", boldRed("⚠"), bold(remoteURL))
	fmt.Printf("  Branches: %s\n", cyan(strings.Join(changed, ", ")))
	fmt.Printf("\n  %s This will %s to the remote.\n",
		boldRed("⚠"),
		bold("force-push"))

	if !confirmDangerous("  ") {
		fmt.Printf("\n%s Push skipped. You can push manually:\n", yellow("→"))
		fmt.Printf("  %s\n", cyan("git push --force-with-lease --all"))
		return
	}

	fmt.Printf("\n%s Pushing to remote...\n", cyan("::"))
	if err := rewriter.PushBranches(repoPath, beforeRefs, afterRefs); err != nil {
		rpt.SetPushError(err.Error())
		_ = rpt.Save()
		fmt.Fprintf(os.Stderr, "%s Push failed: %v\n", red("Error:"), err)
		return
	}

	rpt.SetPushed()
	_ = rpt.Save()
	fmt.Printf("\n%s Pushed %s to remote\n", green("✓"), bold(strings.Join(changed, ", ")))
}

// --- REMOTE ---
