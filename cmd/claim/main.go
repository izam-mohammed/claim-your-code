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
	"github.com/izam-mohammed/claim-your-code/internal/rewriter"
	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

var version = "dev"

var (
	bold    = color.New(color.Bold).SprintFunc()
	cyan    = color.New(color.FgCyan).SprintFunc()
	green   = color.New(color.FgGreen).SprintFunc()
	yellow  = color.New(color.FgYellow).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	dim     = color.New(color.Faint).SprintFunc()
	boldRed = color.New(color.Bold, color.FgRed).SprintFunc()
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "__filter-msg":
		runFilterMsg()
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
  claim <folder>     Scan and clean a git repository
  claim --version    Show version
  claim --help       Show this help

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
	results, branchSummaries, err := scanner.Scan(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Printf("\n%s No Co-Authored-By: Claude lines found. Nothing to do.\n", green("✓"))
		return
	}

	fmt.Printf("\n%s Found %s with Co-Authored-By: Claude lines\n",
		yellow("!"),
		bold(fmt.Sprintf("%d commit(s)", len(results))))

	// Show per-branch breakdown with model info
	fmt.Printf("\n%s\n", bold("Branches:"))
	for _, bs := range branchSummaries {
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
			fmt.Printf("\n%s Aborted.\n", red("✗"))
			return
		}
	}

	fmt.Printf("\n%s Rewriting commit messages...\n", cyan("::"))
	if err := rewriter.Rewrite(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	fmt.Printf("\n%s Cleaned %s\n",
		green("✓"),
		bold(fmt.Sprintf("%d commit(s)", len(results))))
	fmt.Printf("\n%s If you've already pushed, force-push to update remote:\n", yellow("Note:"))
	fmt.Printf("  %s\n", cyan("git push --force-with-lease"))
}
