package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
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
  claim <folder>               Scan and clean a git repository
  claim report <folder>        List all claim reports
  claim report <folder> <id>   Show a specific report in detail
  claim report <folder> all    Show all reports in detail
  claim revert <folder>        Revert the most recent clean
  claim revert <folder> <id>   Revert a specific clean by report ID
  claim --version              Show version
  claim --help                 Show this help

%s
  --dry-run          Show what would be changed without modifying anything
  --force            Skip confirmation prompt
`, bold("claim"), bold("Usage:"), bold("Flags:"))
}

func runFilterMsg() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("error:"), err)
		os.Exit(1)
	}
	fmt.Print(filter.StripClaudeCoAuthor(string(input)))
}

func runReport() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "%s Usage: claim report <folder> [id|all]\n", red("Error:"))
		os.Exit(1)
	}

	folder := os.Args[2]
	absPath, err := filepath.Abs(folder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	// No ID given → list all reports
	if len(os.Args) < 4 {
		reports, err := report.List(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
			os.Exit(1)
		}
		report.PrintList(reports)
		return
	}

	idArg := os.Args[3]

	// "all" → show every report in detail
	if idArg == "all" {
		reports, err := report.List(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
			os.Exit(1)
		}
		if len(reports) == 0 {
			fmt.Printf("\n%s No claim reports found.\n", dim("→"))
			return
		}
		for i, r := range reports {
			if i > 0 {
				fmt.Println(dim("  ─────────────────────────────────────────"))
			}
			report.PrintDetail(r)
		}
		return
	}

	// Specific ID (or prefix)
	rpt, err := report.FindByPrefix(absPath, idArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	report.PrintDetail(rpt)
}

func runRevert() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "%s Usage: claim revert <folder> [id]\n", red("Error:"))
		os.Exit(1)
	}

	folder := os.Args[2]
	absPath, err := filepath.Abs(folder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	var rpt *report.Report

	if len(os.Args) >= 4 {
		// Specific ID
		rpt, err = report.FindByPrefix(absPath, os.Args[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
			os.Exit(1)
		}
	} else {
		// Find most recent revertible report
		reports, err := report.List(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
			os.Exit(1)
		}
		for _, r := range reports {
			if r.IsRevertible() {
				rpt = r
				break
			}
		}
		if rpt == nil {
			fmt.Printf("%s No revertible claims found.\n", yellow("!"))
			return
		}
	}

	if !rpt.IsRevertible() {
		if rpt.Reverted != nil {
			fmt.Fprintf(os.Stderr, "%s Report %s has already been reverted.\n", red("Error:"), cyan(rpt.ID))
		} else if rpt.Result == nil || rpt.Result.Status != "cleaned" {
			fmt.Fprintf(os.Stderr, "%s Report %s was not a successful clean (status: %s).\n", red("Error:"), cyan(rpt.ID), rpt.Result.Status)
		} else {
			fmt.Fprintf(os.Stderr, "%s Report %s has no stored branch refs to revert to.\n", red("Error:"), cyan(rpt.ID))
		}
		os.Exit(1)
	}

	// Show what will be reverted
	fmt.Printf("\n%s Reverting claim %s\n", cyan("::"), boldCyan(rpt.ID))
	fmt.Printf("  %s  %s\n", dim("Date:"), rpt.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	fmt.Printf("  %s  %s\n", dim("Cleaned:"), bold(fmt.Sprintf("%d commit(s)", rpt.Result.Cleaned)))
	fmt.Println()
	fmt.Printf("  %s\n", bold("Branches to restore:"))
	for branch, hash := range rpt.OriginalRefs {
		fmt.Printf("  %s %s %s %s\n", cyan("→"), bold(branch), dim("→"), dim(hash[:8]))
	}

	// Confirm
	fmt.Printf("\n%s This will restore branches to their pre-claim state.\n", boldRed("⚠"))
	fmt.Printf("  Proceed? %s ", dim("[y/N]"))

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Printf("\n%s Aborted.\n", red("✗"))
		return
	}

	// Perform revert
	fmt.Printf("\n%s Restoring branches...\n", cyan("::"))
	if err := rewriter.Revert(absPath, rpt.OriginalRefs); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	// Mark as reverted and save
	rpt.SetReverted()
	if err := rpt.Save(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to update report: %v\n", yellow("Warning:"), err)
	}

	fmt.Printf("\n%s Reverted %s — Co-Authored-By lines are back.\n", green("✓"), boldCyan(rpt.ID))
	fmt.Printf("\n%s If you've already pushed the cleaned version, force-push to restore remote:\n", yellow("Note:"))
	fmt.Printf("  %s\n", cyan("git push --force-with-lease"))
}

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

	// Resolve to absolute path
	absPath, err := filepath.Abs(folder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	// Check folder exists
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s '%s' is not a valid directory\n", red("Error:"), folder)
		os.Exit(1)
	}

	// Check for .git
	gitDir := filepath.Join(absPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		fmt.Fprintf(os.Stderr, "%s '%s' is not a git repository %s\n", red("Error:"), folder, dim("(no .git directory found)"))
		os.Exit(1)
	}

	// Scan for Claude co-author commits
	fmt.Printf("%s Scanning commits in %s\n", cyan("::"), bold(folder))
	scan, err := scanner.Scan(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	results := scan.Results

	if len(results) == 0 {
		fmt.Printf("\n%s No Co-Authored-By: Claude lines found. Nothing to do.\n", green("✓"))
		return
	}

	// Build the report
	rpt := report.Build(version, absPath, scan.TotalCommits, results, scan.BranchSummaries)

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
		if err := rpt.Save(absPath); err != nil {
			fmt.Fprintf(os.Stderr, "%s Failed to save report: %v\n", yellow("Warning:"), err)
		} else {
			fmt.Printf("\n%s Report saved %s\n", dim("→"), dim(rpt.ID))
		}
		fmt.Printf("\n%s No changes made.\n", yellow("--dry-run:"))
		return
	}

	// Confirm before rewriting
	if !force {
		fmt.Printf("\n%s This will rewrite git history for %s.\n",
			boldRed("⚠"),
			bold(fmt.Sprintf("%d commit(s)", len(results))))
		fmt.Printf("  Proceed? %s ", dim("[y/N]"))

		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer != "y" && answer != "yes" {
			rpt.SetResult("aborted", 0)
			if err := rpt.Save(absPath); err != nil {
				fmt.Fprintf(os.Stderr, "%s Failed to save report: %v\n", yellow("Warning:"), err)
			}
			fmt.Printf("\n%s Aborted.\n", red("✗"))
			return
		}
	}

	// Capture branch refs before rewriting (for revert)
	originalRefs, err := rewriter.GetBranchRefs(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to capture branch refs: %v\n", yellow("Warning:"), err)
	}
	rpt.SetOriginalRefs(originalRefs)

	fmt.Printf("\n%s Rewriting commit messages...\n", cyan("::"))
	if err := rewriter.Rewrite(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	rpt.SetResult("cleaned", len(results))
	if err := rpt.Save(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to save report: %v\n", yellow("Warning:"), err)
	} else {
		fmt.Printf("\n%s Report saved %s\n", dim("→"), dim(rpt.ID))
	}

	fmt.Printf("\n%s Cleaned %s\n",
		green("✓"),
		bold(fmt.Sprintf("%d commit(s)", len(results))))
	fmt.Printf("\n%s If you've already pushed, force-push to update remote:\n", yellow("Note:"))
	fmt.Printf("  %s\n", cyan("git push --force-with-lease"))
}
