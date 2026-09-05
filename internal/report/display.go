package report

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
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

// selectReport asks which report to view. Tests replace it to answer
// without a terminal.
var selectReport = func(options []huh.Option[string]) (string, error) {
	var selected string
	err := huh.NewSelect[string]().
		Title("Select a report to view").
		Options(options...).
		Height(15).
		Value(&selected).
		Run()
	return selected, err
}

// SetSelectReport replaces the report picker and returns a function that puts
// the old one back. Tests in other packages use it: without a stub the picker
// draws a real form, and on a runner with a console attached it then blocks on
// a keypress nobody is there to press.
func SetSelectReport(fn func(options []huh.Option[string]) (string, error)) (restore func()) {
	prev := selectReport
	selectReport = fn
	return func() { selectReport = prev }
}

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

// PrintListAndSelect displays all reports and lets user select one to view.
func PrintListAndSelect(reports []*Report) {
	if len(reports) == 0 {
		fmt.Printf("\n%s No claim reports found.\n", dim("→"))
		return
	}

	fmt.Printf("\n%s %s\n", boldCyan("⚡"), bold(fmt.Sprintf("Claim Reports (%d)", len(reports))))

	// Build select options
	options := make([]huh.Option[string], len(reports))
	for i, r := range reports {
		status := "pending"
		if r.Result != nil {
			status = r.Result.Status
		}
		icon := statusIcon(status)

		revertTag := ""
		if r.Reverted != nil {
			icon = red("↩")
			revertTag = red(" (reverted)")
		}

		modelParts := []string{}
		for model, count := range r.Summary.Models {
			modelParts = append(modelParts, fmt.Sprintf("%s×%d", model, count))
		}
		modelStr := strings.Join(modelParts, ", ")

		source := filepath.Base(r.Repo)
		if r.IsRemote() {
			source = r.RemoteOwner + "/" + r.RemoteRepo
			if r.PRNumber > 0 {
				source = fmt.Sprintf("%s/%s#%d", r.RemoteOwner, r.RemoteRepo, r.PRNumber)
			}
		}

		label := fmt.Sprintf("%s %s  %s  %s  %d commits  %s%s",
			icon, r.ID, source,
			r.CreatedAt.Local().Format("01-02 15:04"),
			r.Summary.AffectedCommits,
			modelStr, revertTag)

		options[i] = huh.NewOption(label, r.ID)
	}

	selected, err := selectReport(options)
	if err != nil || selected == "" {
		return
	}

	// Find and display the selected report
	for _, r := range reports {
		if r.ID == selected {
			PrintDetail(r)
			return
		}
	}
}

// PrintDetail displays a single report with full details.
func PrintDetail(r *Report) {
	fmt.Println()

	// Header
	fmt.Printf("  %s  %s\n", boldCyan("⚡"), bold("Claim Report"))
	fmt.Println()

	// Meta
	fmt.Printf("  %s  %s\n", dim("ID:"), boldCyan(r.ID))
	if r.IsRemote() {
		fmt.Printf("  %s  %s\n", dim("Repo:"), cyan(r.RemoteURL))
		if r.PRNumber > 0 {
			fmt.Printf("  %s  %s %s\n", dim("PR:"), bold(fmt.Sprintf("#%d", r.PRNumber)), dim(r.PRTitle))
			if r.PRBranch != "" {
				fmt.Printf("  %s  %s\n", dim("Branch:"), cyan(r.PRBranch))
			}
		}
	} else {
		fmt.Printf("  %s  %s\n", dim("Repo:"), bold(r.Repo))
	}
	fmt.Printf("  %s  %s\n", dim("Date:"), r.CreatedAt.Local().Format("2006-01-02 15:04:05"))
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

	// Push status
	if r.GitState != nil && r.GitState.PushedAt != nil {
		fmt.Printf("  %s  %s %s\n", dim("Pushed:"), green("✓"), dim(r.GitState.PushedAt.Local().Format("2006-01-02 15:04:05")))
	} else if r.GitState != nil && r.GitState.PushError != "" {
		fmt.Printf("  %s  %s %s\n", dim("Pushed:"), red("✗"), dim(r.GitState.PushError))
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
		stat := formatDiffStat(c.Insertions, c.Deletions)
		fmt.Printf("  %s %s%s%s%s\n", dim(c.Hash[:8]), c.Subject, stat, modelTag, branchTags)
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
		fmt.Printf("  %s  Revert with: %s\n", dim("↩"), cyan(fmt.Sprintf("claim revert %s", r.ID)))
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

func formatDiffStat(ins, del int) string {
	if ins == 0 && del == 0 {
		return ""
	}
	parts := []string{}
	if ins > 0 {
		parts = append(parts, green(fmt.Sprintf("+%d", ins)))
	}
	if del > 0 {
		parts = append(parts, red(fmt.Sprintf("-%d", del)))
	}
	return " " + strings.Join(parts, dim("/"))
}
