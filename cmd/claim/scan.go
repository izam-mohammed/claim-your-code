// Scanning many repositories at once, either through the API or by cloning.

package main

import (
	"fmt"
	"os"

	githubpkg "github.com/izam-mohammed/claim-your-code/internal/github"
	"github.com/izam-mohammed/claim-your-code/internal/pattern"
	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

// scanAPIConcurrent scans repos through the API, several at a time.
func scanAPIConcurrent(client *githubpkg.Client, repos []githubpkg.RepoInfo) {
	type apiResult struct {
		repo    githubpkg.RepoInfo
		branch  string
		matches []githubpkg.CommitInfo
		err     error
	}

	results := runConcurrent(repos,
		func(r githubpkg.RepoInfo) string { return r.Name },
		func(repo githubpkg.RepoInfo) apiResult {
			branch := repo.DefaultBranch
			commits, err := client.ListCommits(repo.Owner, repo.Name, branch, 100)
			if err != nil {
				return apiResult{repo: repo, branch: branch, err: err}
			}
			var matches []githubpkg.CommitInfo
			for _, c := range commits {
				if pattern.ContainsClaudeCoAuthor(c.Message) {
					matches = append(matches, c)
				}
			}
			return apiResult{repo: repo, branch: branch, matches: matches}
		})

	var affected []affectedRepo
	clean := 0
	for _, res := range results {
		if res.err != nil || len(res.matches) == 0 {
			clean++
			continue
		}
		affected = append(affected, affectedRepo{
			label:   res.repo.Owner + "/" + res.repo.Name,
			commits: len(res.matches),
			note:    "on " + cyan(res.branch),
		})
	}

	summariseRepos(clean, affected)
}

// scanRepoViaAPI does a quick API-based scan of recent commits.
// If branch is empty, uses the repo's default branch.

// scanRepoViaAPI does a quick API-based scan of recent commits.
// If branch is empty, uses the repo's default branch.
func scanRepoViaAPI(client *githubpkg.Client, repo *githubpkg.RepoInfo, branch ...string) {
	scanBranch := repo.DefaultBranch
	if len(branch) > 0 && branch[0] != "" {
		scanBranch = branch[0]
	}

	fmt.Printf("%s Scanning %s via API...\n", cyan("::"), bold(repo.FullName))

	commits, err := client.ListCommits(repo.Owner, repo.Name, scanBranch, 100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s %v\n", red("Error:"), err)
		return
	}

	var matches []githubpkg.CommitInfo
	for _, c := range commits {
		if pattern.ContainsClaudeCoAuthor(c.Message) {
			matches = append(matches, c)
		}
	}

	branchNote := fmt.Sprintf("on %s", cyan(scanBranch))

	if len(matches) == 0 {
		fmt.Printf("  %s No Claude co-authorship found %s %s\n", green("✓"), dim(fmt.Sprintf("(%d commits checked", len(commits))), dim(branchNote+")"))
		return
	}

	fmt.Printf("\n  %s Found %s %s %s\n",
		yellow("!"),
		bold(fmt.Sprintf("%d commit(s) co-authored by Claude", len(matches))),
		dim(fmt.Sprintf("in %d recent commits", len(commits))),
		dim(branchNote))

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
		subject := pattern.Subject(c.Message)
		fmt.Printf("  %s %s%s\n", dim(c.SHA[:8]), subject, modelTag)
	}
	if len(matches) > 5 {
		fmt.Printf("  %s\n", dim(fmt.Sprintf("... and %d more", len(matches)-5)))
	}
}

// printScanResults displays the common scan output (used by remote flows).

// cloneAndScanConcurrent clones repos several at a time and scans each clone's
// full history, which the API scan cannot see.
func cloneAndScanConcurrent(client *githubpkg.Client, repos []githubpkg.RepoInfo) {
	type clone struct {
		repo   githubpkg.RepoInfo
		path   string
		tmpDir string
		err    error
	}

	clones := runConcurrent(repos,
		func(r githubpkg.RepoInfo) string { return r.Name },
		func(repo githubpkg.RepoInfo) clone {
			tmpDir, err := githubpkg.CreateTempDir()
			if err != nil {
				return clone{repo: repo, err: err}
			}
			cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", repo.Owner, repo.Name)
			if client.IsAuthenticated() {
				cloneURL = client.AuthCloneURL(repo.Owner, repo.Name)
			}
			path, err := githubpkg.Clone(cloneURL, tmpDir)
			return clone{repo: repo, path: path, tmpDir: tmpDir, err: err}
		})

	type scanned struct {
		repo  githubpkg.RepoInfo
		label string
		scan  *scanner.ScanOutput
	}

	var found []scanned
	var affected []affectedRepo
	clean := 0
	for _, c := range clones {
		if c.err != nil {
			continue
		}
		scan, err := scanner.Scan(c.path)
		githubpkg.CleanupTempDir(c.tmpDir)
		if err != nil {
			continue
		}
		if len(scan.Results) == 0 {
			clean++
			continue
		}

		label := c.repo.Owner + "/" + c.repo.Name
		ins, del := 0, 0
		for _, r := range scan.Results {
			ins += r.Insertions
			del += r.Deletions
		}
		found = append(found, scanned{repo: c.repo, label: label, scan: scan})
		affected = append(affected, affectedRepo{
			label: label, commits: len(scan.Results), insertions: ins, deletions: del,
		})
	}

	if !summariseRepos(clean, affected) {
		return
	}

	for _, f := range found {
		rpt := report.Build(version, "github.com/"+f.label, f.scan.TotalCommits,
			f.scan.Results, f.scan.BranchSummaries)
		rpt.RemoteURL = "https://github.com/" + f.label
		rpt.RemoteOwner = f.repo.Owner
		rpt.RemoteRepo = f.repo.Name
		rpt.SetResult("dry_run", 0)
		_ = rpt.Save()
	}
}
