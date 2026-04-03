package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/izam-mohammed/claim-your-code/internal/auth"
	"github.com/izam-mohammed/claim-your-code/internal/discover"
	"github.com/izam-mohammed/claim-your-code/internal/filter"
	"github.com/izam-mohammed/claim-your-code/internal/github"
	"github.com/izam-mohammed/claim-your-code/internal/pattern"
	"github.com/izam-mohammed/claim-your-code/internal/remote"
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
	case "repo":
		runRemoteRepo()
	case "pr":
		runRemotePR()
	case "org":
		runRemoteOrg()
	case "user":
		runRemoteUser()
	case "report":
		runReport()
	case "revert":
		runRevert()
	case "--version", "-v":
		fmt.Printf("claim %s\n", version)
	case "--help", "-h":
		printUsage()
	default:
		arg := os.Args[1]
		if remote.IsURL(arg) {
			target, err := remote.ParseURL(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
				os.Exit(1)
			}
			if target.PRNum > 0 {
				runRemotePRWithTarget(target)
			} else {
				runRemoteRepoWithTarget(target)
			}
		} else {
			runClaim(arg)
		}
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s - Remove Claude as co-author from git commits

%s
  claim <folder>                Scan and clean local git repositories
  claim <github-url>            Auto-detect repo or PR from URL

%s
  claim repo <owner/repo>       Scan a remote GitHub repo
  claim pr <owner/repo#N>       Scan a pull request
  claim org <name>              Scan all repos in an organization
  claim user <name>             Scan all repos of a user

%s
  claim report [id|all]         List or show claim reports
  claim revert <id>             Revert a specific clean

%s
  --dry-run          Show what would be changed without modifying anything
  --force            Skip confirmation prompt
  --apply            For remote repos, rewrite and force-push (default: scan only)
  --api-only         Scan via GitHub API without cloning (faster, may miss old commits)
`, bold("claim"), bold("Local:"), bold("Remote:"), bold("Reports:"), bold("Flags:"))
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
		fmt.Printf("  %s No Claude co-authorship found.\n", green("✓"))
		return
	}

	rpt := report.Build(version, repoPath, scan.TotalCommits, results, scan.BranchSummaries)

	fmt.Printf("\n%s Found %s across %s\n",
		yellow("!"),
		bold(fmt.Sprintf("%d commit(s) co-authored by Claude", len(results))),
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

// --- REMOTE ---

type remoteFlags struct {
	dryRun  bool
	force   bool
	apply   bool
	apiOnly bool
}

func parseRemoteFlags() remoteFlags {
	var f remoteFlags
	for _, arg := range os.Args[2:] {
		switch arg {
		case "--dry-run":
			f.dryRun = true
		case "--force", "-f":
			f.force = true
		case "--apply":
			f.apply = true
		case "--api-only":
			f.apiOnly = true
		}
	}
	return f
}

// nonFlagArg returns the first non-flag argument after the subcommand.
func nonFlagArg() string {
	for _, arg := range os.Args[2:] {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}

func requireAuth() (*github.Client, string) {
	fmt.Printf("%s Authenticating with GitHub...\n", cyan("::"))
	token, err := auth.GetToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	client := github.NewClient(token)
	user, err := client.GetUser()
	if err == nil {
		fmt.Printf("  %s Authenticated as %s\n", green("✓"), bold(user.Login))
	}
	return client, token
}

func runRemoteRepo() {
	arg := nonFlagArg()
	if arg == "" {
		fmt.Fprintf(os.Stderr, "%s Usage: claim repo <owner/repo or URL>\n", red("Error:"))
		os.Exit(1)
	}
	target, err := remote.ParseRepo(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	runRemoteRepoWithTarget(target)
}

func runRemoteRepoWithTarget(target *remote.Target) {
	flags := parseRemoteFlags()
	client, _ := requireAuth()

	fmt.Printf("\n%s Fetching repo info for %s\n", cyan("::"), bold(target.String()))
	repoInfo, err := client.GetRepo(target.Owner, target.Repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	if flags.apiOnly {
		scanRepoViaAPI(client, repoInfo, flags)
		return
	}

	// Clone and scan
	tmpDir, err := github.CreateTempDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	defer github.CleanupTempDir(tmpDir)

	cloneURL := client.AuthCloneURL(target.Owner, target.Repo)
	fmt.Printf("%s Cloning %s...\n", cyan("::"), bold(target.String()))
	repoPath, err := github.Clone(cloneURL, tmpDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	// Scan
	scan, err := scanner.Scan(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	results := scan.Results
	if len(results) == 0 {
		fmt.Printf("\n%s No Claude co-authorship found in %s\n", green("✓"), bold(target.String()))
		return
	}

	// Build report with remote metadata
	rpt := report.Build(version, "github.com/"+target.Owner+"/"+target.Repo, scan.TotalCommits, results, scan.BranchSummaries)
	rpt.RemoteURL = fmt.Sprintf("https://github.com/%s/%s", target.Owner, target.Repo)
	rpt.RemoteOwner = target.Owner
	rpt.RemoteRepo = target.Repo

	printScanResults(scan, results)

	if flags.dryRun {
		rpt.SetResult("dry_run", 0)
		_ = rpt.Save()
		fmt.Printf("\n%s Report tracked %s\n", dim("→"), dim(rpt.ID))
		fmt.Printf("\n%s No changes made.\n", yellow("--dry-run:"))
		return
	}

	if !flags.apply {
		rpt.SetResult("dry_run", 0)
		_ = rpt.Save()
		fmt.Printf("\n%s Report tracked %s\n", dim("→"), dim(rpt.ID))
		fmt.Printf("\n%s This was a scan-only run. To rewrite and push, add %s\n", yellow("Note:"), cyan("--apply"))
		return
	}

	// Apply: rewrite + push
	if !flags.force {
		fmt.Printf("\n%s This will rewrite and %s %s to %s\n",
			boldRed("⚠"), bold("force-push"), bold(target.String()), bold(repoInfo.DefaultBranch))
		if !confirm("  Proceed?") {
			rpt.SetResult("aborted", 0)
			_ = rpt.Save()
			fmt.Printf("\n%s Aborted.\n", red("✗"))
			return
		}
	}

	originalRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetOriginalRefs(originalRefs)

	fmt.Printf("\n%s Rewriting commit messages...\n", cyan("::"))
	if err := rewriter.Rewrite(repoPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		return
	}

	afterRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetAfterRefs(afterRefs)

	fmt.Printf("%s Pushing to %s...\n", cyan("::"), bold(target.String()))
	if err := github.PushForce(repoPath, repoInfo.DefaultBranch); err != nil {
		rpt.SetPushError(err.Error())
		rpt.SetResult("cleaned", len(results))
		_ = rpt.Save()
		fmt.Fprintf(os.Stderr, "%s Push failed: %v\n", red("Error:"), err)
		return
	}

	rpt.SetPushed()
	rpt.SetResult("cleaned", len(results))
	_ = rpt.Save()

	fmt.Printf("\n%s Cleaned and pushed %s in %s\n",
		green("✓"),
		bold(fmt.Sprintf("%d commit(s)", len(results))),
		bold(target.String()))
}

func runRemotePR() {
	arg := nonFlagArg()
	if arg == "" {
		fmt.Fprintf(os.Stderr, "%s Usage: claim pr <owner/repo#N or PR URL>\n", red("Error:"))
		os.Exit(1)
	}
	target, err := remote.ParsePR(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	runRemotePRWithTarget(target)
}

func runRemotePRWithTarget(target *remote.Target) {
	flags := parseRemoteFlags()
	client, _ := requireAuth()

	fmt.Printf("\n%s Fetching PR #%d from %s/%s\n", cyan("::"), target.PRNum, bold(target.Owner), bold(target.Repo))
	prInfo, err := client.GetPR(target.Owner, target.Repo, target.PRNum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	fmt.Printf("  %s %s\n", dim("Title:"), bold(prInfo.Title))
	fmt.Printf("  %s %s → %s\n", dim("Branch:"), cyan(prInfo.HeadBranch), dim(prInfo.BaseBranch))

	if flags.apiOnly {
		scanRepoViaAPI(client, &prInfo.Repo, flags)
		return
	}

	// Clone and checkout PR branch
	tmpDir, err := github.CreateTempDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	defer github.CleanupTempDir(tmpDir)

	cloneURL := client.AuthCloneURL(target.Owner, target.Repo)
	fmt.Printf("\n%s Cloning and checking out %s...\n", cyan("::"), cyan(prInfo.HeadBranch))
	repoPath, err := github.Clone(cloneURL, tmpDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}
	if err := github.Checkout(repoPath, prInfo.HeadBranch); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	// Scan
	scan, err := scanner.Scan(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	results := scan.Results
	if len(results) == 0 {
		fmt.Printf("\n%s No Claude co-authorship found in PR #%d\n", green("✓"), target.PRNum)
		return
	}

	rpt := report.Build(version, "github.com/"+target.Owner+"/"+target.Repo, scan.TotalCommits, results, scan.BranchSummaries)
	rpt.RemoteURL = fmt.Sprintf("https://github.com/%s/%s", target.Owner, target.Repo)
	rpt.RemoteOwner = target.Owner
	rpt.RemoteRepo = target.Repo
	rpt.PRNumber = prInfo.Number
	rpt.PRTitle = prInfo.Title
	rpt.PRBranch = prInfo.HeadBranch

	printScanResults(scan, results)

	if flags.dryRun || !flags.apply {
		status := "dry_run"
		rpt.SetResult(status, 0)
		_ = rpt.Save()
		fmt.Printf("\n%s Report tracked %s\n", dim("→"), dim(rpt.ID))
		if !flags.apply {
			fmt.Printf("\n%s This was a scan-only run. To rewrite and push, add %s\n", yellow("Note:"), cyan("--apply"))
		} else {
			fmt.Printf("\n%s No changes made.\n", yellow("--dry-run:"))
		}
		return
	}

	// Apply: rewrite + push PR branch
	if !flags.force {
		fmt.Printf("\n%s This will rewrite and force-push branch %s for PR #%d\n",
			boldRed("⚠"), cyan(prInfo.HeadBranch), prInfo.Number)
		if !confirm("  Proceed?") {
			rpt.SetResult("aborted", 0)
			_ = rpt.Save()
			fmt.Printf("\n%s Aborted.\n", red("✗"))
			return
		}
	}

	originalRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetOriginalRefs(originalRefs)

	fmt.Printf("\n%s Rewriting commit messages...\n", cyan("::"))
	if err := rewriter.Rewrite(repoPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		return
	}

	afterRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetAfterRefs(afterRefs)

	fmt.Printf("%s Pushing branch %s...\n", cyan("::"), cyan(prInfo.HeadBranch))
	if err := github.PushForce(repoPath, prInfo.HeadBranch); err != nil {
		rpt.SetPushError(err.Error())
		rpt.SetResult("cleaned", len(results))
		_ = rpt.Save()
		fmt.Fprintf(os.Stderr, "%s Push failed: %v\n", red("Error:"), err)
		return
	}

	rpt.SetPushed()
	rpt.SetResult("cleaned", len(results))
	_ = rpt.Save()

	fmt.Printf("\n%s Cleaned and pushed PR #%d (%s)\n",
		green("✓"), prInfo.Number, bold(fmt.Sprintf("%d commit(s)", len(results))))
}

func runRemoteOrg() {
	name := nonFlagArg()
	if name == "" {
		fmt.Fprintf(os.Stderr, "%s Usage: claim org <org-name>\n", red("Error:"))
		os.Exit(1)
	}
	client, _ := requireAuth()
	runRemoteMultiRepo(client, name, true)
}

func runRemoteUser() {
	name := nonFlagArg()
	if name == "" {
		fmt.Fprintf(os.Stderr, "%s Usage: claim user <username>\n", red("Error:"))
		os.Exit(1)
	}
	client, _ := requireAuth()
	runRemoteMultiRepo(client, name, false)
}

func runRemoteMultiRepo(client *github.Client, name string, isOrg bool) {
	flags := parseRemoteFlags()

	label := "user"
	if isOrg {
		label = "org"
	}

	fmt.Printf("\n%s Fetching repos for %s %s...\n", cyan("::"), label, bold(name))
	var repos []github.RepoInfo
	var err error
	if isOrg {
		repos, err = client.ListOrgRepos(name)
	} else {
		repos, err = client.ListUserRepos(name)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	// Filter out archived and forks
	var active []github.RepoInfo
	for _, r := range repos {
		if !r.Archived && !r.Fork {
			active = append(active, r)
		}
	}

	if len(active) == 0 {
		fmt.Printf("\n%s No active repos found for %s %s\n", yellow("!"), label, bold(name))
		return
	}

	fmt.Printf("\n%s Found %s %s:\n", green("✓"), bold(fmt.Sprintf("%d repo(s)", len(active))), dim(fmt.Sprintf("(%d total, excluding archived/forks)", len(repos))))
	for i, r := range active {
		desc := ""
		if r.Description != "" {
			desc = dim(" — " + r.Description)
		}
		private := ""
		if r.Private {
			private = yellow(" [private]")
		}
		fmt.Printf("  %s %s%s%s\n", cyan(fmt.Sprintf("[%d]", i+1)), bold(r.Name), private, desc)
	}

	// Ask scan method
	fmt.Printf("\n%s\n", bold("Scan method:"))
	fmt.Printf("  %s Quick scan via API %s\n", cyan("[1]"), dim("(checks recent commits, fast)"))
	fmt.Printf("  %s Full clone & scan %s\n", cyan("[2]"), dim("(all history, slower but complete)"))
	fmt.Printf("\n  Choose %s ", dim("[1/2]"))
	answer, _ := stdinReader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	useAPI := answer != "2"

	// Ask which repos
	var selected []github.RepoInfo
	if !flags.force {
		fmt.Printf("\n  Which repos? %s ", dim("[all/numbers e.g. 1,3,5]"))
		answer, _ := stdinReader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer == "" || answer == "all" || answer == "y" || answer == "yes" {
			selected = active
		} else {
			parts := strings.FieldsFunc(answer, func(r rune) bool { return r == ',' || r == ' ' })
			for _, p := range parts {
				var idx int
				if _, err := fmt.Sscanf(p, "%d", &idx); err == nil && idx >= 1 && idx <= len(active) {
					selected = append(selected, active[idx-1])
				}
			}
		}
	} else {
		selected = active
	}

	if len(selected) == 0 {
		fmt.Printf("\n%s No repos selected.\n", yellow("!"))
		return
	}

	fmt.Printf("\n%s Scanning %d repo(s)...\n\n", cyan("::"), len(selected))

	for i, r := range selected {
		if i > 0 {
			fmt.Println(dim("  ═══════════════════════════════════════════"))
			fmt.Println()
		}

		if useAPI {
			scanRepoViaAPI(client, &r, flags)
		} else {
			target := &remote.Target{
				Owner:    r.Owner,
				Repo:     r.Name,
				CloneURL: client.AuthCloneURL(r.Owner, r.Name),
			}
			runRemoteRepoWithTarget(target)
		}
	}
}

// scanRepoViaAPI does a quick API-based scan of recent commits.
func scanRepoViaAPI(client *github.Client, repo *github.RepoInfo, flags remoteFlags) {
	fmt.Printf("%s Scanning %s via API...\n", cyan("::"), bold(repo.FullName))

	commits, err := client.ListCommits(repo.Owner, repo.Name, repo.DefaultBranch, 100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s %v\n", red("Error:"), err)
		return
	}

	var matches []github.CommitInfo
	for _, c := range commits {
		if pattern.ContainsClaudeCoAuthor(c.Message) {
			matches = append(matches, c)
		}
	}

	if len(matches) == 0 {
		fmt.Printf("  %s No Claude co-authorship found %s\n", green("✓"), dim(fmt.Sprintf("(%d commits checked)", len(commits))))
		return
	}

	fmt.Printf("\n  %s Found %s %s\n",
		yellow("!"),
		bold(fmt.Sprintf("%d commit(s) co-authored by Claude", len(matches))),
		dim(fmt.Sprintf("in %d recent commits", len(commits))))

	limit := len(matches)
	if limit > 5 {
		limit = 5
	}
	for _, c := range matches[:limit] {
		model := pattern.ExtractModelName(c.Message)
		modelTag := ""
		if model != "" {
			modelTag = dim(" (" + model + ")")
		}
		subject := firstLine(c.Message)
		fmt.Printf("  %s %s%s\n", dim(c.SHA[:8]), subject, modelTag)
	}
	if len(matches) > 5 {
		fmt.Printf("  %s\n", dim(fmt.Sprintf("... and %d more", len(matches)-5)))
	}
}

// printScanResults displays the common scan output (used by remote flows).
func printScanResults(scan *scanner.ScanOutput, results []scanner.Result) {
	fmt.Printf("\n%s Found %s across %s\n",
		yellow("!"),
		bold(fmt.Sprintf("%d commit(s) co-authored by Claude", len(results))),
		dim(fmt.Sprintf("%d total commits", scan.TotalCommits)))

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
}

func firstLine(s string) string {
	for i, ch := range s {
		if ch == '\n' {
			return s[:i]
		}
	}
	return s
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
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		os.Exit(1)
	}

	rpt.SetReverted()
	if err := rpt.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to record revert: %v\n", yellow("Warning:"), err)
	}

	fmt.Printf("\n%s Reverted %s — Claude co-authorship restored.\n", green("✓"), boldCyan(rpt.ID))
	fmt.Printf("\n%s If you've already pushed the cleaned version, force-push to restore remote:\n", yellow("Note:"))
	fmt.Printf("  %s\n", cyan("git push --force-with-lease"))
}
