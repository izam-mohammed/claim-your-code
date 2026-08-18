// Rendering a scan: which branches were touched, and by which commits.

package main

import (
	"fmt"

	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

// printScanResults displays the common scan output (used by remote flows).
func printScanResults(scan *scanner.ScanOutput, results []scanner.Result) {
	fmt.Printf("\n%s Found %s across %s\n",
		yellow("!"),
		bold(fmt.Sprintf("%d commit(s) co-authored by Claude", len(results))),
		dim(fmt.Sprintf("%d total commits", scan.TotalCommits)))

	printBranchesAndCommits(scan, results)
}

func printBranchesAndCommits(scan *scanner.ScanOutput, results []scanner.Result) {
	fmt.Printf("\n%s\n", bold("Branches:"))
	for _, bs := range scan.BranchSummaries {
		branchStat := formatDiffStat(bs.Insertions, bs.Deletions)
		fmt.Printf("  %s %s  %s%s\n",
			cyan("→"),
			bold(bs.Branch),
			dim(fmt.Sprintf("(%d commit(s))", bs.Count)),
			branchStat)
		for model, count := range bs.Models {
			modelStat := ""
			if ms, ok := bs.ModelStats[model]; ok {
				modelStat = formatDiffStat(ms.Insertions, ms.Deletions)
			}
			fmt.Printf("      %s  %s%s\n",
				yellow(model),
				dim(fmt.Sprintf("× %d", count)),
				modelStat)
		}
	}

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
		stat := formatDiffStat(r.Insertions, r.Deletions)
		fmt.Printf("  %s %s%s%s\n", dim(r.Hash[:8]), r.Subject, modelTag, stat)
	}
	if len(results) > 10 {
		fmt.Printf("  %s\n", dim(fmt.Sprintf("... and %d more", len(results)-10)))
	}
}

func printCompactBranchSummary(scan *scanner.ScanOutput) {
	branches := scan.BranchSummaries

	// Aggregate models across all branches
	modelTotals := map[string]int{}
	for _, r := range scan.Results {
		if r.Model != "" {
			modelTotals[r.Model]++
		}
	}

	// Show models
	fmt.Printf("  %s\n", bold("Models:"))
	for model, count := range modelTotals {
		fmt.Printf("    %s %s  %s\n", yellow("●"), yellow(model), dim(fmt.Sprintf("× %d", count)))
	}

	// Show branches — top 5 by count, then "and N more"
	fmt.Printf("  %s %s\n", bold("Branches:"), dim(fmt.Sprintf("(%d affected)", len(branches))))

	limit := len(branches)
	if limit > 5 {
		limit = 5
	}
	for _, bs := range branches[:limit] {
		branchStat := formatDiffStat(bs.Insertions, bs.Deletions)
		fmt.Printf("    %s %s  %s%s\n", cyan("→"), bs.Branch, dim(fmt.Sprintf("%d commit(s)", bs.Count)), branchStat)
	}
	if len(branches) > 5 {
		fmt.Printf("    %s\n", dim(fmt.Sprintf("... and %d more branches", len(branches)-5)))
	}
	fmt.Println()
}
