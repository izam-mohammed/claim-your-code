// Listing past runs and undoing them.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/rewriter"
)

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
			fatal(err)
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
				fatal(err)
			}
			repoFilter = absPath
		}
	}

	if idArg != "" && idArg != "all" {
		rpt, err := report.FindByPrefix(repoFilter, idArg)
		if err != nil {
			fatal(err)
		}
		report.PrintDetail(rpt)
		return
	}

	allReports, err := report.List(repoFilter)
	if err != nil {
		fatal(err)
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

	report.PrintListAndSelect(allReports)
}

// --- REVERT ---

func runRevert() {
	if len(os.Args) < 3 {
		fatalf("missing argument — run `claim revert --help`\n\n  Usage: claim revert <id>")
	}

	idArg := os.Args[2]

	rpt, err := report.FindByPrefix("", idArg)
	if err != nil {
		fatal(err)
	}

	revertReport(rpt.Repo, rpt)
}

func revertReport(repoPath string, rpt *report.Report) {
	if !rpt.IsRevertible() {
		switch {
		case rpt.Reverted != nil:
			fmt.Fprintf(os.Stderr, "%s Report %s has already been reverted.\n", red("Error:"), cyan(rpt.ID))
		case rpt.IsRemote():
			fmt.Fprintf(os.Stderr, "%s Report %s is a remote clean, which cannot be reverted.\n", red("Error:"), cyan(rpt.ID))
			fmt.Fprintf(os.Stderr, "  It ran in a temporary clone that no longer exists. To restore %s,\n", bold(rpt.RemoteURL))
			fmt.Fprintf(os.Stderr, "  push the history you want from a copy that still has it.\n")
		case rpt.Result == nil:
			fmt.Fprintf(os.Stderr, "%s Report %s has no recorded result.\n", red("Error:"), cyan(rpt.ID))
		case rpt.Result.Status != "cleaned":
			fmt.Fprintf(os.Stderr, "%s Report %s was not a successful clean (status: %s).\n", red("Error:"), cyan(rpt.ID), rpt.Result.Status)
		default:
			fmt.Fprintf(os.Stderr, "%s Report %s has no branch refs to revert to.\n", red("Error:"), cyan(rpt.ID))
		}
		exit(1)
		return
	}

	repoName := filepath.Base(repoPath)
	fmt.Printf("\n%s Reverting claim %s in %s\n", cyan("::"), boldCyan(rpt.ID), bold(repoName))
	fmt.Printf("  %s  %s\n", dim("Date:"), rpt.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	fmt.Printf("  %s  %s\n", dim("Cleaned:"), bold(fmt.Sprintf("%d commit(s)", rpt.Result.Cleaned)))
	fmt.Println()
	beforeRefs := rpt.GetBeforeRefs()
	fmt.Printf("  %s\n", bold("Branches to restore:"))
	for branch, hash := range beforeRefs {
		fmt.Printf("  %s %s %s %s\n", cyan("→"), bold(branch), dim("→"), dim(hash[:8]))
	}

	fmt.Printf("\n%s This will restore branches to their pre-claim state.\n", boldRed("⚠"))
	if !confirm("  Proceed?") {
		fmt.Printf("\n%s Aborted.\n", red("✗"))
		return
	}

	fmt.Printf("\n%s Restoring branches...\n", cyan("::"))
	if err := rewriter.Revert(repoPath, beforeRefs); err != nil {
		fatal(err)
	}

	rpt.SetReverted()
	if err := rpt.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to record revert: %v\n", yellow("Warning:"), err)
	}

	fmt.Printf("\n%s Reverted %s — Claude co-authorship restored.\n", green("✓"), boldCyan(rpt.ID))
	fmt.Printf("\n%s If you've already pushed the cleaned version, force-push to restore remote:\n", yellow("Note:"))
	fmt.Printf("  %s\n", cyan("git push --force-with-lease"))
}
