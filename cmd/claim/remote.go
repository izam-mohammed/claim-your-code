// The GitHub flows: a single repo, a pull request, or every repo belonging
// to a user or organisation.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/izam-mohammed/claim-your-code/internal/auth"
	githubpkg "github.com/izam-mohammed/claim-your-code/internal/github"
	"github.com/izam-mohammed/claim-your-code/internal/remote"
	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/rewriter"
	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

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

// nonFlagArg returns the first non-flag argument after the subcommand.
func nonFlagArg() string {
	for _, arg := range os.Args[2:] {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}

// getToken fetches a GitHub token. Tests replace it to avoid the
// interactive account picker.
var getToken = auth.GetToken

func requireAuth() *githubpkg.Client {
	fmt.Printf("%s Authenticating with GitHub...\n", cyan("::"))
	token, err := getToken()
	if err != nil {
		fatal(err)
	}
	if token == "" {
		return githubpkg.NewPublicClient()
	}
	return githubpkg.NewClient(token)
}

func runLogout() {
	accounts := auth.ListAccountUsernames()
	if len(accounts) == 0 {
		fmt.Printf("%s No saved accounts.\n", dim("→"))
		return
	}

	// If username provided as argument, remove that one
	if len(os.Args) >= 3 && !strings.HasPrefix(os.Args[2], "-") {
		username := os.Args[2]
		if err := auth.RemoveAccount(username); err != nil {
			fatal(err)
		}
		fmt.Printf("%s Removed account %s\n", green("✓"), bold("@"+username))
		return
	}

	// Interactive — show saved accounts and let user choose
	options := make([]huh.Option[string], 0, len(accounts)+1)
	for _, name := range accounts {
		options = append(options, huh.NewOption(fmt.Sprintf("@%s", name), name))
	}
	if len(accounts) > 1 {
		options = append(options, huh.NewOption("Remove all accounts", "__all__"))
	}

	choice, err := selectOne("Which account to remove?", options, 0)
	if err != nil {
		promptFailed(err, "choose an account", "Name the account instead: claim logout <username>")
		return
	}

	if choice == "__all__" {
		if err := auth.RemoveAllAccounts(); err != nil {
			fatal(err)
		}
		fmt.Printf("%s Removed all %d accounts.\n", green("✓"), len(accounts))
	} else {
		if err := auth.RemoveAccount(choice); err != nil {
			fatal(err)
		}
		fmt.Printf("%s Removed account %s\n", green("✓"), bold("@"+choice))
	}
}

// cloneBase is the host clone URLs are built from. Tests point it at a local
// directory so the clone paths can run without reaching GitHub.
var cloneBase = "https://github.com"

// cloneURLFor builds the URL used to clone owner/repo, embedding the token
// when the client has one.
func cloneURLFor(client *githubpkg.Client, owner, repo string) string {
	if client.IsAuthenticated() && cloneBase == "https://github.com" {
		return client.AuthCloneURL(owner, repo)
	}
	return fmt.Sprintf("%s/%s/%s.git", cloneBase, owner, repo)
}

// cloneToTemp clones a repo into a temporary directory, using an authenticated
// URL when the client has a token. The returned cleanup must be deferred by the
// caller -- doing it here would delete the clone before it could be scanned.
func cloneToTemp(client *githubpkg.Client, owner, repo string) (path string, cleanup func()) {
	tmpDir, err := githubpkg.CreateTempDir()
	if err != nil {
		fatal(err)
	}
	cleanup = func() { githubpkg.CleanupTempDir(tmpDir) }

	path, err = githubpkg.Clone(cloneURLFor(client, owner, repo), tmpDir)
	if err != nil {
		cleanup()
		fatal(err)
	}
	return path, cleanup
}

func runRemoteRepo() {
	arg := nonFlagArg()
	if arg == "" {
		fatalf("missing argument — run `claim repo --help`\n\n  Usage: claim repo <owner/repo or URL>")
	}
	target, err := remote.ParseRepo(arg)
	if err != nil {
		fatal(err)
	}
	runRemoteRepoWithTarget(target)
}

func runRemoteRepoWithTarget(target *remote.Target) {
	flags := parseRemoteFlags()

	// Try public access first (no auth needed for public repos)
	fmt.Printf("%s Fetching repo info for %s\n", cyan("::"), bold(target.String()))
	client := githubpkg.NewPublicClient()
	repoInfo, err := client.GetRepo(target.Owner, target.Repo)

	needsAuth := err != nil || repoInfo.Private || flags.apply
	if needsAuth {
		// Repo is private or we need to push — authenticate
		client = requireAuth()
		if err != nil {
			// Retry with auth
			repoInfo, err = client.GetRepo(target.Owner, target.Repo)
			if err != nil {
				fatal(err)
			}
		}
	}

	if flags.apiOnly {
		scanRepoViaAPI(client, repoInfo, target.Branch)
		return
	}

	fmt.Printf("%s Cloning %s...\n", cyan("::"), bold(target.String()))
	repoPath, cleanup := cloneToTemp(client, target.Owner, target.Repo)
	defer cleanup()

	// Checkout specific branch if specified
	if target.Branch != "" {
		fmt.Printf("%s Checking out branch %s...\n", cyan("::"), cyan(target.Branch))
		if err := githubpkg.Checkout(repoPath, target.Branch); err != nil {
			fatal(err)
		}
	}

	// Scan
	scan, err := scanner.Scan(repoPath)
	if err != nil {
		fatal(err)
	}

	results := scan.Results
	if len(results) == 0 {
		fmt.Printf("  %s No Claude co-authorship found in %s\n", green("✓"), bold(target.String()))
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
		fmt.Printf("\n%s Report tracked %s\n", dim("→"), boldCyan(rpt.ID))
		fmt.Printf("\n%s No changes made.\n", yellow("--dry-run:"))
		return
	}

	if !flags.apply {
		rpt.SetResult("dry_run", 0)
		_ = rpt.Save()
		fmt.Printf("\n%s Report tracked %s\n", dim("→"), boldCyan(rpt.ID))
		fmt.Printf("\n%s This was a scan-only run. To rewrite and push, add %s\n", yellow("Note:"), cyan("--apply"))
		return
	}

	// Apply: rewrite + push
	originalRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetOriginalRefs(originalRefs)

	if !flags.force {
		fmt.Printf("\n%s This will rewrite %s in %s\n",
			boldRed("⚠"), bold(fmt.Sprintf("%d commit(s)", len(results))), bold(target.String()))
		if !confirm("  Rewrite?") {
			rpt.SetResult("aborted", 0)
			_ = rpt.Save()
			fmt.Printf("\n%s Aborted.\n", red("✗"))
			return
		}
	}

	fmt.Printf("\n%s Rewriting commit messages...\n", cyan("::"))
	if err := rewriter.Rewrite(repoPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		return
	}

	afterRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetAfterRefs(afterRefs)
	rpt.SetResult("cleaned", len(results))

	fmt.Printf("\n%s Cleaned %s\n", green("✓"), bold(fmt.Sprintf("%d commit(s)", len(results))))

	// Push confirmation
	changed := rewriter.ChangedBranches(originalRefs, afterRefs)
	if len(changed) == 0 {
		_ = rpt.Save()
		return
	}

	fmt.Printf("\n%s Force-push to %s?\n", boldRed("⚠"), bold(target.String()))
	fmt.Printf("  Branches: %s\n", cyan(strings.Join(changed, ", ")))

	if !confirmDangerous("  ") {
		_ = rpt.Save()
		fmt.Printf("\n%s Push skipped. Rewrite was applied locally in temp clone only.\n", yellow("→"))
		return
	}

	pushBranch := repoInfo.DefaultBranch
	if target.Branch != "" {
		pushBranch = target.Branch
	}
	fmt.Printf("\n%s Pushing to %s...\n", cyan("::"), bold(target.String()))
	if err := githubpkg.PushForce(repoPath, pushBranch); err != nil {
		rpt.SetPushError(err.Error())
		_ = rpt.Save()
		fmt.Fprintf(os.Stderr, "%s Push failed: %v\n", red("Error:"), err)
		return
	}

	rpt.SetPushed()
	_ = rpt.Save()
	fmt.Printf("\n%s Pushed %s to %s\n",
		green("✓"), bold(strings.Join(changed, ", ")), bold(target.String()))
}

func runRemotePR() {
	arg := nonFlagArg()
	if arg == "" {
		fatalf("missing argument — run `claim pr --help`\n\n  Usage: claim pr <owner/repo#N or PR URL>")
	}
	target, err := remote.ParsePR(arg)
	if err != nil {
		fatal(err)
	}
	runRemotePRWithTarget(target)
}

func runRemotePRWithTarget(target *remote.Target) {
	flags := parseRemoteFlags()

	// Try public access first
	fmt.Printf("\n%s Fetching PR #%d from %s/%s\n", cyan("::"), target.PRNum, bold(target.Owner), bold(target.Repo))
	client := githubpkg.NewPublicClient()
	prInfo, err := client.GetPR(target.Owner, target.Repo, target.PRNum)

	needsAuth := err != nil || flags.apply
	if prInfo != nil {
		needsAuth = needsAuth || prInfo.Repo.Private
	}
	if needsAuth {
		client = requireAuth()
		if err != nil {
			prInfo, err = client.GetPR(target.Owner, target.Repo, target.PRNum)
			if err != nil {
				fatal(err)
			}
		}
	}

	fmt.Printf("  %s %s\n", dim("Title:"), bold(prInfo.Title))
	fmt.Printf("  %s %s → %s\n", dim("Branch:"), cyan(prInfo.HeadBranch), dim(prInfo.BaseBranch))

	if flags.apiOnly {
		// The head branch, not the repository default: a pull request's
		// commits are the point of scanning one.
		scanRepoViaAPI(client, &prInfo.Repo, prInfo.HeadBranch)
		return
	}

	fmt.Printf("\n%s Cloning and checking out %s...\n", cyan("::"), cyan(prInfo.HeadBranch))
	repoPath, cleanup := cloneToTemp(client, target.Owner, target.Repo)
	defer cleanup()
	if err := githubpkg.Checkout(repoPath, prInfo.HeadBranch); err != nil {
		fatal(err)
	}

	// Scan
	scan, err := scanner.Scan(repoPath)
	if err != nil {
		fatal(err)
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
		rpt.SetResult("dry_run", 0)
		_ = rpt.Save()
		fmt.Printf("\n%s Report tracked %s\n", dim("→"), boldCyan(rpt.ID))
		if !flags.apply {
			fmt.Printf("\n%s This was a scan-only run. To rewrite and push, add %s\n", yellow("Note:"), cyan("--apply"))
		} else {
			fmt.Printf("\n%s No changes made.\n", yellow("--dry-run:"))
		}
		return
	}

	// Apply: rewrite + push PR branch
	originalRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetOriginalRefs(originalRefs)

	if !flags.force {
		fmt.Printf("\n%s This will rewrite %s in PR #%d\n",
			boldRed("⚠"), bold(fmt.Sprintf("%d commit(s)", len(results))), prInfo.Number)
		if !confirm("  Rewrite?") {
			rpt.SetResult("aborted", 0)
			_ = rpt.Save()
			fmt.Printf("\n%s Aborted.\n", red("✗"))
			return
		}
	}

	fmt.Printf("\n%s Rewriting commit messages...\n", cyan("::"))
	if err := rewriter.Rewrite(repoPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", red("Error:"), err)
		return
	}

	afterRefs, _ := rewriter.GetBranchRefs(repoPath)
	rpt.SetAfterRefs(afterRefs)
	rpt.SetResult("cleaned", len(results))

	fmt.Printf("\n%s Cleaned %s\n", green("✓"), bold(fmt.Sprintf("%d commit(s)", len(results))))

	// Push confirmation
	fmt.Printf("\n%s Force-push branch %s for PR #%d?\n",
		boldRed("⚠"), cyan(prInfo.HeadBranch), prInfo.Number)

	if !confirmDangerous("  ") {
		_ = rpt.Save()
		fmt.Printf("\n%s Push skipped. Rewrite was applied locally in temp clone only.\n", yellow("→"))
		return
	}

	fmt.Printf("\n%s Pushing branch %s...\n", cyan("::"), cyan(prInfo.HeadBranch))
	if err := githubpkg.PushForce(repoPath, prInfo.HeadBranch); err != nil {
		rpt.SetPushError(err.Error())
		_ = rpt.Save()
		fmt.Fprintf(os.Stderr, "%s Push failed: %v\n", red("Error:"), err)
		return
	}

	rpt.SetPushed()
	_ = rpt.Save()
	fmt.Printf("\n%s Pushed branch %s for PR #%d\n",
		green("✓"), cyan(prInfo.HeadBranch), prInfo.Number)
}

// runRemoteMultiRepoByName auto-detects whether a name is an org or user.
// Tries public access first, authenticates only if needed.

// listRepos fetches an owner's repositories, as an org or as a user.
func listRepos(client *githubpkg.Client, name string, isOrg bool) ([]githubpkg.RepoInfo, error) {
	if isOrg {
		return client.ListOrgRepos(name)
	}
	return client.ListUserRepos(name)
}

// publicOrAuthedClient tries an owner's repos anonymously and falls back to
// authenticating, which is what private repos need.
func publicOrAuthedClient(name string, isOrg bool) *githubpkg.Client {
	client := githubpkg.NewPublicClient()
	if repos, err := listRepos(client, name, isOrg); err != nil || len(repos) == 0 {
		client = requireAuth()
	}
	return client
}

// ownerArg reads the owner name from the command line, accepting either a bare
// name or a GitHub URL.
func ownerArg(usage string) string {
	arg := nonFlagArg()
	if arg == "" {
		fatalf("%s", usage)
	}
	if !remote.IsURL(arg) {
		return arg
	}
	target, err := remote.ParseURL(arg)
	if err != nil {
		fatal(err)
	}
	return target.Owner
}

// runRemoteMultiRepoByName auto-detects whether a name is an org or a user.
func runRemoteMultiRepoByName(name string) {
	isOrg := githubpkg.NewPublicClient().IsOrg(name)
	runRemoteMultiRepo(publicOrAuthedClient(name, isOrg), name, isOrg)
}

func runRemoteOrg() {
	name := ownerArg("missing argument — run `claim org --help`\n\n  Usage: claim org <org-name or URL>")
	runRemoteMultiRepo(publicOrAuthedClient(name, true), name, true)
}

func runRemoteUser() {
	name := ownerArg("missing argument — run `claim user --help`\n\n  Usage: claim user <username or URL>")
	runRemoteMultiRepo(publicOrAuthedClient(name, false), name, false)
}

func runRemoteMultiRepo(client *githubpkg.Client, name string, isOrg bool) {
	flags := parseRemoteFlags()

	label := "user"
	if isOrg {
		label = "org"
	}

	fmt.Printf("\n%s Fetching repos for %s %s...\n", cyan("::"), label, bold(name))
	repos, err := listRepos(client, name, isOrg)
	if err != nil {
		fatal(err)
	}

	// Filter out archived and forks
	var active []githubpkg.RepoInfo
	for _, r := range repos {
		if !r.Archived && !r.Fork {
			active = append(active, r)
		}
	}

	if len(active) == 0 {
		fmt.Printf("\n%s No active repos found for %s %s\n", yellow("!"), label, bold(name))
		return
	}

	fmt.Printf("\n%s Found %s %s\n", green("✓"), bold(fmt.Sprintf("%d repo(s)", len(active))), dim(fmt.Sprintf("(%d total, excluding archived/forks)", len(repos))))

	// Offer to authenticate for full access if currently unauthenticated
	if !client.IsAuthenticated() {
		if confirm("Authenticate for full access? (includes private repos)") {
			client = requireAuth()
			// Re-fetch with auth
			fmt.Printf("\n%s Re-fetching repos with full access...\n", cyan("::"))
			repos, err = listRepos(client, name, isOrg)
			if err != nil {
				fatal(err)
			}
			active = nil
			for _, r := range repos {
				if !r.Archived && !r.Fork {
					active = append(active, r)
				}
			}
			fmt.Printf("\n%s Found %s %s\n", green("✓"), bold(fmt.Sprintf("%d repo(s)", len(active))), dim(fmt.Sprintf("(%d total, excluding archived/forks)", len(repos))))
		}
	}

	// Ask: all repos or select individual?
	var selected []githubpkg.RepoInfo
	if flags.force {
		selected = active
	} else {
		scope, err := selectOne(
			fmt.Sprintf("Scan all %d repos or select individually?", len(active)),
			[]huh.Option[string]{
				huh.NewOption(fmt.Sprintf("All %d repos", len(active)), "all"),
				huh.NewOption("Select repos individually", "select"),
			}, 0)
		if err != nil {
			promptFailed(err, "choose repositories", "Pass --force to scan every repository without prompting.")
			return
		}

		if scope == "all" {
			selected = active
		} else {
			repoOptions := make([]huh.Option[string], len(active))
			for i, r := range active {
				label := r.Name
				if r.Private {
					label += " [private]"
				}
				repoOptions[i] = huh.NewOption(label, r.Name)
			}

			selectedNames, err := selectMany("Select repos to scan", repoOptions, 12)
			if err != nil || len(selectedNames) == 0 {
				fmt.Printf("\n%s No repos selected.\n", yellow("!"))
				return
			}

			nameSet := make(map[string]bool)
			for _, n := range selectedNames {
				nameSet[n] = true
			}
			for _, r := range active {
				if nameSet[r.Name] {
					selected = append(selected, r)
				}
			}
		}
	}

	if len(selected) == 0 {
		fmt.Printf("\n%s No repos selected.\n", yellow("!"))
		return
	}

	// Ask scan method. --force takes the default answer, as it does for
	// every other prompt, so the whole command can run unattended.
	useAPI := true
	if !flags.force {
		scanMethod, err := selectOne("Scan method", []huh.Option[string]{
			huh.NewOption("Quick scan via API (recent commits, fast)", "api"),
			huh.NewOption("Full clone & scan (all history, complete)", "clone"),
		}, 0)
		if err != nil {
			promptFailed(err, "choose a scan method", "Pass --force to take the quick API scan.")
			return
		}
		useAPI = scanMethod == "api"
	}

	fmt.Printf("\n%s Scanning %d repo(s)...\n", cyan("::"), len(selected))

	if useAPI {
		scanAPIConcurrent(client, selected)
	} else {
		cloneAndScanConcurrent(client, selected)
	}
}

// scanAPIConcurrent scans repos via API with up to 5 concurrent requests.
