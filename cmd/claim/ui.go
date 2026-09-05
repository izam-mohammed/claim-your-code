// Terminal output: colours, prompts, progress and the summaries several
// commands share.

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
)

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

// exit ends the process. Tests replace it to observe fatal paths.
var exit = os.Exit

// fatal reports a failure the command cannot continue past.
func fatal(err error) {
	fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
	exit(1)
}

// fatalf is fatal for failures described in words rather than carried in an error.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", red("Error:"), fmt.Sprintf(format, args...))
	exit(1)
}

// promptFailed explains a prompt that could not be answered. Cancelling is a
// decision, so it ends quietly; having no terminal is a dead end, so it names
// what to run instead and fails.
func promptFailed(err error, action, hint string) {
	if !errors.Is(err, errNoTerminal) {
		fmt.Printf("\n%s Cancelled.\n", red("✗"))
		return
	}
	fmt.Fprintf(os.Stderr, "\n%s No terminal to %s on.\n", red("Error:"), action)
	if hint != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", dim(hint))
	}
	exit(1)
}

// hasFlag reports whether one of names was passed to the command.
func hasFlag(names ...string) bool {
	for _, arg := range os.Args[2:] {
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
}

// The prompts below are variables so tests can answer them without a terminal.

// errNoTerminal is what the pickers return when there is nobody to answer.
var errNoTerminal = errors.New("no terminal to prompt on")

// interactive reports whether there is a terminal to draw a prompt on. Without
// one a form has nobody to answer it: it either fails, or -- where a console
// is attached but no human is watching, as on a CI runner -- sits waiting for
// a keypress that never comes. The prompts answer for themselves instead.
// internal/auth makes the same check for the same reason.
var interactive = hasTerminal

func hasTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// confirm asks a yes/no question, answering no if the prompt cannot run.
var confirm = func(title string) bool {
	if !interactive() {
		return false
	}
	var yes bool
	err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&yes).
		Run()
	if err != nil {
		return false
	}
	return yes
}

// confirmDangerous requires explicit "confirm" input for destructive actions.
var confirmDangerous = func(title string) bool {
	if !interactive() {
		return false
	}
	var input string
	err := huh.NewInput().
		Title(title).
		Description(`Type "confirm" to proceed`).
		Value(&input).
		Run()
	if err != nil {
		return false
	}
	return strings.TrimSpace(strings.ToLower(input)) == "confirm"
}

// selectOne asks the user to pick a single option.
var selectOne = func(title string, options []huh.Option[string], height int) (string, error) {
	if !interactive() {
		return "", errNoTerminal
	}
	var choice string
	sel := huh.NewSelect[string]().
		Title(title).
		Options(options...).
		Value(&choice)
	if height > 0 {
		sel = sel.Height(height)
	}
	err := sel.Run()
	return choice, err
}

// selectMany asks the user to pick any number of options.
var selectMany = func(title string, options []huh.Option[string], height int) ([]string, error) {
	if !interactive() {
		return nil, errNoTerminal
	}
	var chosen []string
	sel := huh.NewMultiSelect[string]().
		Title(title).
		Options(options...).
		Value(&chosen)
	if height > 0 {
		sel = sel.Height(height)
	}
	err := sel.Run()
	return chosen, err
}

func printProgress(done, total int, current string, start time.Time) {
	// Truncate label if too long, by runes so multi-byte characters survive
	if utf8.RuneCountInString(current) > 25 {
		current = string([]rune(current)[:25]) + "..."
	}

	elapsed := time.Since(start)

	// Indeterminate progress (total == 0) — show counter
	if total <= 0 {
		fmt.Printf("\r  %s %s %s    ",
			dim("⣾"),
			dim(current),
			dim(elapsed.Round(time.Second).String()))
		return
	}

	pct := float64(done) / float64(total)
	barWidth := 25
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	eta := ""
	if done > 0 && done < total {
		remaining := time.Duration(float64(elapsed) / float64(done) * float64(total-done))
		eta = fmt.Sprintf(" %s left", remaining.Round(time.Second))
	}

	fmt.Printf("\r  %s %s%s %d/%d %s    ",
		dim(bar),
		green(fmt.Sprintf("%d%%", int(pct*100))),
		dim(eta),
		done, total,
		dim(current))
}

// cloneAndScanConcurrent clones repos with up to 5 concurrent clones,
// starting a new clone as soon as one finishes (worker pool pattern).

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

// maxWorkers bounds how much of GitHub we hit at once.
const maxWorkers = 5

// runConcurrent applies work to every item with at most maxWorkers in flight,
// drawing a progress bar as results arrive and returning them in input order.
func runConcurrent[T, R any](items []T, label func(T) string, work func(T) R) []R {
	type indexed struct {
		idx int
		res R
	}

	resultCh := make(chan indexed, len(items))
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, it T) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			resultCh <- indexed{idx, work(it)}
		}(i, item)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]R, len(items))
	done := 0
	start := time.Now()
	for item := range resultCh {
		results[item.idx] = item.res
		done++
		printProgress(done, len(items), label(items[item.idx]), start)
	}
	fmt.Println()

	return results
}

// affectedRepo is one repo with commits to clean, as the shared summary shows it.
type affectedRepo struct {
	label      string
	commits    int
	insertions int
	deletions  int
	note       string
}

// summariseRepos prints the "N clean / M affected" block that every multi-repo
// scan ends with. It reports whether anything needs cleaning.
func summariseRepos(clean int, affected []affectedRepo) bool {
	if clean > 0 {
		fmt.Printf("%s %s clean\n", green("✓"), bold(fmt.Sprintf("%d repo(s)", clean)))
	}

	if len(affected) == 0 {
		fmt.Printf("%s No Claude co-authorship found.\n", green("✓"))
		return false
	}

	total := 0
	for _, a := range affected {
		total += a.commits
	}

	fmt.Printf("\n%s %s across %s:\n",
		yellow("!"),
		bold(fmt.Sprintf("%d commit(s) co-authored by Claude", total)),
		bold(fmt.Sprintf("%d repo(s)", len(affected))))

	for _, a := range affected {
		note := a.note
		if note != "" {
			note = " " + dim(note)
		}
		fmt.Printf("  %s %s  %s%s%s\n",
			yellow("●"),
			bold(a.label),
			dim(fmt.Sprintf("%d commit(s)", a.commits)),
			formatDiffStat(a.insertions, a.deletions),
			note)
	}

	return true
}
