package report

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

var (
	bold      = color.New(color.Bold).SprintFunc()
	cyan      = color.New(color.FgCyan).SprintFunc()
	green     = color.New(color.FgGreen).SprintFunc()
	yellow    = color.New(color.FgYellow).SprintFunc()
	red       = color.New(color.FgRed).SprintFunc()
	dim       = color.New(color.Faint).SprintFunc()
	boldCyan  = color.New(color.Bold, color.FgCyan).SprintFunc()
	boldGreen = color.New(color.Bold, color.FgGreen).SprintFunc()
)

// statusIcon returns a colored icon for the result status.
func statusIcon(status string) string {
	switch status {
	case "cleaned":
		return green("✓")
	case "dry_run":
		return yellow("◎")
	case "aborted":
		return red("✗")
	default:
		return dim("?")
	}
}

// statusLabel returns a colored label for the result status.
func statusLabel(status string) string {
	switch status {
	case "cleaned":
		return green("cleaned")
	case "dry_run":
		return yellow("dry run")
	case "aborted":
		return red("aborted")
	default:
		return dim("unknown")
	}
}

// PrintList displays all reports in a compact overview table.
func PrintList(reports []*Report) {
	if len(reports) == 0 {
		fmt.Printf("\n%s No claim reports found.\n", dim("→"))
		return
	}

	fmt.Printf("\n%s %s\n\n", boldCyan("⚡"), bold(fmt.Sprintf("Claim Reports (%d)", len(reports))))

	for _, r := range reports {
		status := "pending"
		if r.Result != nil {
			status = r.Result.Status
		}
		icon := statusIcon(status)

		// Reverted tag
		revertTag := ""
		if r.Reverted != nil {
			icon = dim("↩")
			revertTag = dim(" (reverted)")
		}

		// Models summary
		modelParts := []string{}
		for model, count := range r.Summary.Models {
			modelParts = append(modelParts, fmt.Sprintf("%s × %d", model, count))
		}
		modelStr := strings.Join(modelParts, ", ")

		fmt.Printf("  %s %s  %s  %s  %s%s\n",
			icon,
			boldCyan(r.ID),
			dim(r.CreatedAt.Local().Format("2006-01-02 15:04")),
			bold(fmt.Sprintf("%d commits", r.Summary.AffectedCommits)),
			dim(modelStr),
			revertTag)
	}

	fmt.Printf("\n%s Run %s to see details\n", dim("→"), cyan("claim report <folder> <id>"))
}

// PrintDetail displays a single report with full details.
func PrintDetail(r *Report) {
	fmt.Println()

	// Header
	fmt.Printf("  %s  %s\n", boldCyan("⚡"), bold("Claim Report"))
	fmt.Println()

	// Meta
	fmt.Printf("  %s  %s\n", dim("ID:"), boldCyan(r.ID))
	fmt.Printf("  %s  %s\n", dim("Date:"), r.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	fmt.Printf("  %s  %s\n", dim("Tool:"), fmt.Sprintf("claim %s", r.Version))

	// Status
	status := "pending"
	if r.Result != nil {
		status = r.Result.Status
	}
	if r.Reverted != nil {
		fmt.Printf("  %s  %s %s\n", dim("Status:"), dim("↩"), dim("reverted"))
	} else {
		fmt.Printf("  %s  %s %s\n", dim("Status:"), statusIcon(status), statusLabel(status))
	}
	fmt.Println()

	// Summary
	fmt.Printf("  %s\n", bold("Summary"))
	fmt.Printf("  %s  %s scanned, %s co-authored\n",
		cyan("→"),
		bold(fmt.Sprintf("%d", r.Summary.TotalCommits)),
		boldGreen(fmt.Sprintf("%d", r.Summary.AffectedCommits)))
	fmt.Println()

	// Models
	fmt.Printf("  %s\n", bold("Models"))
	for model, count := range r.Summary.Models {
		pct := float64(count) / float64(r.Summary.AffectedCommits) * 100
		bar := renderBar(pct, 20)
		fmt.Printf("  %s  %s  %s  %s\n",
			yellow("●"),
			yellow(model),
			dim(fmt.Sprintf("× %d", count)),
			dim(bar))
	}
	fmt.Println()

	// Branches
	fmt.Printf("  %s\n", bold("Branches"))
	for _, b := range r.Branches {
		fmt.Printf("  %s %s  %s\n",
			cyan("→"),
			bold(b.Name),
			dim(fmt.Sprintf("(%d commits)", b.Count)))
		for model, count := range b.Models {
			fmt.Printf("      %s  %s\n",
				yellow(model),
				dim(fmt.Sprintf("× %d", count)))
		}
	}
	fmt.Println()

	// Commits
	fmt.Printf("  %s\n", bold("Commits"))
	for _, c := range r.Commits {
		modelTag := ""
		if c.Model != "" {
			modelTag = dim(" (" + c.Model + ")")
		}
		branchTags := ""
		if len(c.Branches) > 0 {
			branchTags = dim(" [" + strings.Join(c.Branches, ", ") + "]")
		}
		fmt.Printf("  %s %s%s%s\n", dim(c.Hash[:8]), c.Subject, modelTag, branchTags)
	}

	// Result details
	if r.Result != nil {
		fmt.Println()
		fmt.Printf("  %s\n", bold("Result"))
		fmt.Printf("  %s  %s %s\n", dim("Action:"), statusIcon(r.Result.Status), statusLabel(r.Result.Status))
		if r.Result.Cleaned > 0 {
			fmt.Printf("  %s  %s\n", dim("Cleaned:"), boldGreen(fmt.Sprintf("%d commit(s)", r.Result.Cleaned)))
		}
		fmt.Printf("  %s  %s\n", dim("Completed:"), r.Result.CompletedAt.Local().Format("2006-01-02 15:04:05"))
	}

	// Revert details
	if r.Reverted != nil {
		fmt.Println()
		fmt.Printf("  %s\n", bold("Revert"))
		fmt.Printf("  %s  %s %s\n", dim("Status:"), dim("↩"), dim("reverted"))
		fmt.Printf("  %s  %s\n", dim("Reverted at:"), r.Reverted.RevertedAt.Local().Format("2006-01-02 15:04:05"))
	} else if r.IsRevertible() {
		fmt.Println()
		fmt.Printf("  %s  Revert with: %s\n", dim("↩"), cyan(fmt.Sprintf("claim revert <folder> %s", r.ID)))
	}

	fmt.Println()
}

func renderBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 1 && pct > 0 {
		filled = 1
	}
	return fmt.Sprintf("▓%s%s %0.0f%%",
		strings.Repeat("█", filled),
		strings.Repeat("░", width-filled),
		pct)
}
