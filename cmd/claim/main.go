package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/izam-mohammed/claim-your-code/internal/discover"
	"github.com/izam-mohammed/claim-your-code/internal/filter"
	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/rewriter"
	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

var version = "dev"

var (
	bold     = color.New(color.Bold).SprintFunc()
	cyan     = color.New(color.FgCyan).SprintFunc()
	green    = color.New(color.FgGreen).SprintFunc()
	yellow   = color.New(color.FgYellow).SprintFunc()
	red      = color.New(color.FgRed).SprintFunc()
	dim      = color.New(color.Faint).SprintFunc()
	boldRed  = color.New(color.Bold, color.FgRed).SprintFunc()
	boldCyan = color.New(color.Bold, color.FgCyan).SprintFunc()
)

var stdinReader = bufio.NewReader(os.Stdin)

func confirm(prompt string) bool {
	fmt.Printf("%s %s ", prompt, dim("[y/N]"))
	answer, _ := stdinReader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "__filter-msg":
		runFilterMsg()
	case "report":
		runReport()
	case "revert":
		runRevert()
	case "--version", "-v":
		fmt.Printf("claim %s\n", version)
	case "--help", "-h":
		printUsage()
	default:
		runClaim(os.Args[1])
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s - Strip Co-Authored-By: Claude lines from git commits

%s
  claim <folder>               Scan and clean git repositories
  claim report                 List all claim reports
  claim report <folder>        List reports for a specific folder
  claim report <id>            Show a specific report in detail
  claim report all             Show all reports in detail
  claim revert <id>            Revert a specific clean by report ID
  claim --version              Show version
  claim --help                 Show this help

%s
  --dry-run          Show what would be changed without modifying anything
  --force            Skip confirmation prompt

%s
  If <folder> is not a git repo, claim searches for repos inside it.
  Nested repos inside a git repo are detected and offered for scanning.
`, bold("claim"), bold("Usage:"), bold("Flags:"), bold("Discovery:"))
}

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
func resolveRepos(absPath string, force bool) []string {
	if discover.IsGitRepo(absPath) {
		repos := []string{absPath}

		// Check for nested repos one level deep
		nested := discover.FindNestedRepos(absPath)
		if len(nested) > 0 {
			fmt.Printf("\n%s Found %s inside %s:\n",
				yellow("!"),
				bold(fmt.Sprintf("%d nested repo(s)", len(nested))),
				bold(filepath.Base(absPath)))
			for _, r := range nested {
				fmt.Printf("  %s %s\n", cyan("→"), r.Name)
			}
			if force || confirm("\n  Include nested repos?") {
				for _, r := range nested {
					repos = append(repos, r.Path)
				}
			}
		}

		return repos
	}

	// Not a git repo — search for repos inside
	fmt.Printf("%s No .git found in %s, searching for repositories...\n", cyan("::"), bold(filepath.Base(absPath)))
	found := discover.FindRepos(absPath, 4) // search up to 4 levels deep

	if len(found) == 0 {
		fmt.Fprintf(os.Stderr, "%s No git repositories found in '%s'\n", red("Error:"), absPath)
		os.Exit(1)
	}

	fmt.Printf("\n%s Found %s:\n", green("✓"), bold(fmt.Sprintf("%d repo(s)", len(found))))
	for i, r := range found {
		relPath, _ := filepath.Rel(absPath, r.Path)
		if relPath == "" {
			relPath = r.Path
		}
		fmt.Printf("  %s %s  %s\n", cyan(fmt.Sprintf("[%d]", i+1)), bold(r.Name), dim(relPath))
	}

	if !force {
		fmt.Printf("\n  Process all? %s ", dim("[y/N/numbers e.g. 1,3]"))
		answer, _ := stdinReader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer == "" || answer == "n" || answer == "no" {
			fmt.Printf("\n%s Aborted.\n", red("✗"))
			os.Exit(0)
		}

		if answer != "y" && answer != "yes" && answer != "all" {
			// Parse number selection like "1,3" or "1 3"
			return parseSelection(answer, found)
		}
	}

	var paths []string
	for _, r := range found {
		paths = append(paths, r.Path)
	}
	return paths
}

func parseSelection(input string, repos []discover.Repo) []string {
	// Replace commas and spaces with a single delimiter
	input = strings.ReplaceAll(input, ",", " ")
	parts := strings.Fields(input)

	var paths []string
	for _, p := range parts {
		var idx int
		if _, err := fmt.Sscanf(p, "%d", &idx); err == nil {
			if idx >= 1 && idx <= len(repos) {
				paths = append(paths, repos[idx-1].Path)
			}
		}
	}
	if len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "%s No valid selection.\n", red("Error:"))
		os.Exit(1)
	}
	return paths
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
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s '%s' is not a valid directory\n", red("Error:"), folder)
		os.Exit(1)
	}

	repos := resolveRepos(absPath, force)

	for i, repoPath := range repos {
		if i > 0 {
			fmt.Println()
			fmt.Println(dim("  ═══════════════════════════════════════════"))
			fmt.Println()
		}
		claimRepo(repoPath, dryRun, force)
	}
}

func claimRepo(repoPath string, dryRun, force bool) {
	repoName := filepath.Base(repoPath)

	fmt.Printf("%s Scanning commits in %s\n", cyan("::"), bold(repoName))
	scan, err := scanner.Scan(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		return
	}

	results := scan.Results

	if len(results) == 0 {
		fmt.Printf("  %s No Co-Authored-By: Claude lines found.\n", green("✓"))
		return
	}

	rpt := report.Build(version, repoPath, scan.TotalCommits, results, scan.BranchSummaries)

	fmt.Printf("\n%s Found %s across %s\n",
		yellow("!"),
		bold(fmt.Sprintf("%d co-authored commit(s)", len(results))),
		dim(fmt.Sprintf("%d total commits", scan.TotalCommits)))

	// Show per-branch breakdown with model info
	fmt.Printf("\n%s\n", bold("Branches:"))
	for _, bs := range scan.BranchSummaries {
		fmt.Printf("  %s %s  %s\n",
			cyan("→"),
			bold(bs.Branch),
			dim(fmt.Sprintf("(%d commit(s))", bs.Count)))
		for model, count := range bs.Models {
			fmt.Printf("      %s  %s\n",
				yellow(model),
				dim(fmt.Sprintf("× %d", count)))
		}
	}

	// Show affected commits
	fmt.Printf("\n%s\n", bold("Commits:"))
	limit := len(results)
	if limit > 10 {
		limit = 10
	}
	for _, r := range results[:limit] {
		modelTag := ""
		if r.Model != "" {
			modelTag = dim(" (" + r.Model + ")")
		}
		fmt.Printf("  %s %s%s\n", dim(r.Hash[:8]), r.Subject, modelTag)
	}
	if len(results) > 10 {
		fmt.Printf("  %s\n", dim(fmt.Sprintf("... and %d more", len(results)-10)))
	}

	if dryRun {
		rpt.SetResult("dry_run", 0)
		if err := rpt.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "%s Failed to record report: %v\n", yellow("Warning:"), err)
		} else {
			fmt.Printf("\n%s Report tracked %s\n", dim("→"), dim(rpt.ID))
		}
		fmt.Printf("\n%s No changes made.\n", yellow("--dry-run:"))
		return
	}

	if !force {
		fmt.Printf("\n%s This will rewrite git history for %s in %s.\n",
			boldRed("⚠"),
			bold(fmt.Sprintf("%d commit(s)", len(results))),
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

	fmt.Printf("\n%s Rewriting commit messages...\n", cyan("::"))
	if err := rewriter.Rewrite(repoPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		return
	}

	rpt.SetResult("cleaned", len(results))
	if err := rpt.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to record report: %v\n", yellow("Warning:"), err)
	} else {
		fmt.Printf("\n%s Report tracked %s\n", dim("→"), dim(rpt.ID))
	}

	fmt.Printf("\n%s Cleaned %s\n",
		green("✓"),
		bold(fmt.Sprintf("%d commit(s)", len(results))))
	fmt.Printf("\n%s If you've already pushed, force-push to update remote:\n", yellow("Note:"))
	fmt.Printf("  %s\n", cyan("git push --force-with-lease"))
}

// --- REPORT ---

func runReport() {
	// Usage: claim report [folder] [id|all]
	// folder is optional — if omitted, lists all reports globally.
	idArg := ""
	repoFilter := ""

	switch {
	case len(os.Args) >= 4:
		// claim report <folder> <id|all>
		folder := os.Args[2]
		absPath, err := filepath.Abs(folder)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
			os.Exit(1)
		}
		repoFilter = absPath
		idArg = os.Args[3]
	case len(os.Args) == 3:
		// claim report <folder-or-id>
		arg := os.Args[2]
		// If it looks like an ID prefix (starts with "clm" or "all"), treat it as such
		if arg == "all" || strings.HasPrefix(arg, report.IDPrefix) {
			idArg = arg
		} else {
			absPath, err := filepath.Abs(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
				os.Exit(1)
			}
			repoFilter = absPath
		}
	}

	if idArg != "" && idArg != "all" {
		rpt, err := report.FindByPrefix(repoFilter, idArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
			os.Exit(1)
		}
		report.PrintDetail(rpt)
		return
	}

	allReports, err := report.List(repoFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	if idArg == "all" {
		if len(allReports) == 0 {
			fmt.Printf("\n%s No claim reports found.\n", dim("→"))
			return
		}
		for i, r := range allReports {
			if i > 0 {
				fmt.Println(dim("  ─────────────────────────────────────────"))
			}
			report.PrintDetail(r)
		}
		return
	}

	report.PrintList(allReports)
}

// --- REVERT ---

func runRevert() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "%s Usage: claim revert <id>\n", red("Error:"))
		os.Exit(1)
	}

	idArg := os.Args[2]

	rpt, err := report.FindByPrefix("", idArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	revertReport(rpt.Repo, rpt)
}

func revertReport(repoPath string, rpt *report.Report) {
	if !rpt.IsRevertible() {
		if rpt.Reverted != nil {
			fmt.Fprintf(os.Stderr, "%s Report %s has already been reverted.\n", red("Error:"), cyan(rpt.ID))
		} else if rpt.Result == nil || rpt.Result.Status != "cleaned" {
			fmt.Fprintf(os.Stderr, "%s Report %s was not a successful clean (status: %s).\n", red("Error:"), cyan(rpt.ID), rpt.Result.Status)
		} else {
			fmt.Fprintf(os.Stderr, "%s Report %s has no branch refs to revert to.\n", red("Error:"), cyan(rpt.ID))
		}
		os.Exit(1)
	}

	repoName := filepath.Base(repoPath)
	fmt.Printf("\n%s Reverting claim %s in %s\n", cyan("::"), boldCyan(rpt.ID), bold(repoName))
	fmt.Printf("  %s  %s\n", dim("Date:"), rpt.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	fmt.Printf("  %s  %s\n", dim("Cleaned:"), bold(fmt.Sprintf("%d commit(s)", rpt.Result.Cleaned)))
	fmt.Println()
	fmt.Printf("  %s\n", bold("Branches to restore:"))
	for branch, hash := range rpt.OriginalRefs {
		fmt.Printf("  %s %s %s %s\n", cyan("→"), bold(branch), dim("→"), dim(hash[:8]))
	}

	fmt.Printf("\n%s This will restore branches to their pre-claim state.\n", boldRed("⚠"))
	if !confirm("  Proceed?") {
		fmt.Printf("\n%s Aborted.\n", red("✗"))
		return
	}

	fmt.Printf("\n%s Restoring branches...\n", cyan("::"))
	if err := rewriter.Revert(repoPath, rpt.OriginalRefs); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	rpt.SetReverted()
	if err := rpt.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to record revert: %v\n", yellow("Warning:"), err)
	}

	fmt.Printf("\n%s Reverted %s — Co-Authored-By lines are back.\n", green("✓"), boldCyan(rpt.ID))
	fmt.Printf("\n%s If you've already pushed the cleaned version, force-push to restore remote:\n", yellow("Note:"))
	fmt.Printf("  %s\n", cyan("git push --force-with-lease"))
}
