package scanner

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/izam-mohammed/claim-your-code/internal/pattern"
)

// Result represents a commit that contains a Claude co-author line.
type Result struct {
	Hash         string
	Subject      string // First line of the commit message
	Model        string // e.g. "Claude Opus 4.6"
	Branches     []string
	FilesChanged int
	Insertions   int
	Deletions    int
}

// ModelStat holds count and diff stats for a single model.
type ModelStat struct {
	Count      int
	Insertions int
	Deletions  int
}

// BranchSummary holds per-branch stats.
type BranchSummary struct {
	Branch     string
	Count      int
	Models     map[string]int        // model name -> count
	ModelStats map[string]*ModelStat // model name -> diff stats
	Insertions int
	Deletions  int
}

// ScanOutput holds the complete scan results.
type ScanOutput struct {
	Results         []Result
	BranchSummaries []BranchSummary
	TotalCommits    int
}

// ProgressFunc is called with (done, total, label) during scanning.
type ProgressFunc func(done, total int, label string)

// Scan opens a git repository and scans all branches.
func Scan(repoPath string) (*ScanOutput, error) {
	return ScanWithProgress(repoPath, false, nil)
}

// ScanDefaultBranch opens a git repository and scans only the default (HEAD) branch.
func ScanDefaultBranch(repoPath string) (*ScanOutput, error) {
	return ScanWithProgress(repoPath, true, nil)
}

// ScanWithProgress scans with an optional progress callback.
func ScanWithProgress(repoPath string, defaultBranchOnly bool, progress ProgressFunc) (*ScanOutput, error) {
	if progress != nil {
		progress(0, 0, "opening repository...")
	}
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}

	// Build a map of commit hash -> branch names
	commitBranches := map[string][]string{}
	branchTips := map[string]plumbing.Hash{}

	if defaultBranchOnly {
		// Only scan HEAD branch
		headRef, err := repo.Head()
		if err != nil {
			return nil, fmt.Errorf("cannot resolve HEAD: %w", err)
		}
		branchName := "HEAD"
		if headRef.Name().IsBranch() {
			branchName = headRef.Name().Short()
		}
		branchTips[branchName] = headRef.Hash()
	} else {
		// All branches (local + remote)
		refs, err := repo.References()
		if err != nil {
			return nil, err
		}
		err = refs.ForEach(func(ref *plumbing.Reference) error {
			name := ref.Name()
			if name.IsBranch() {
				branchTips[name.Short()] = ref.Hash()
			} else if name.IsRemote() {
				short := name.Short()
				parts := strings.SplitN(short, "/", 2)
				if len(parts) == 2 {
					branchName := parts[1]
					if _, exists := branchTips[branchName]; !exists {
						branchTips[branchName] = ref.Hash()
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// For each branch, walk its commits and record membership
	totalBranches := len(branchTips)
	branchDone := 0
	for branchName, tipHash := range branchTips {
		branchDone++
		if progress != nil {
			progress(branchDone, totalBranches, branchName)
		}
		iter, err := repo.Log(&git.LogOptions{From: tipHash})
		if err != nil {
			continue
		}
		_ = iter.ForEach(func(c *object.Commit) error {
			h := c.Hash.String()
			commitBranches[h] = append(commitBranches[h], branchName)
			return nil
		})
	}

	// Scan commits
	var logOpts git.LogOptions
	if defaultBranchOnly {
		// Only HEAD branch
		headRef, _ := repo.Head()
		logOpts = git.LogOptions{From: headRef.Hash()}
	} else {
		logOpts = git.LogOptions{All: true}
	}
	iter, err := repo.Log(&logOpts)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	totalCommits := 0
	var results []Result
	err = iter.ForEach(func(c *object.Commit) error {
		h := c.Hash.String()
		if seen[h] {
			return nil
		}
		seen[h] = true
		totalCommits++

		if progress != nil && (totalCommits == 1 || totalCommits%50 == 0) {
			progress(totalCommits, 0, fmt.Sprintf("%d commits scanned", totalCommits))
		}

		if pattern.ContainsClaudeCoAuthor(c.Message) {
			model := pattern.ExtractModelName(c.Message)
			branches := commitBranches[h]
			if len(branches) == 0 {
				branches = []string{"(detached)"}
			}
			results = append(results, Result{
				Hash:     h,
				Subject:  pattern.Subject(c.Message),
				Model:    model,
				Branches: branches,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	output := &ScanOutput{
		Results:      results,
		TotalCommits: totalCommits,
	}

	// Enrich with diff stats before building branch summaries
	enrichDiffStats(repoPath, output)

	// Build per-branch summary (after enrichment so stats are available)
	branchStats := map[string]*BranchSummary{}
	for _, r := range output.Results {
		for _, b := range r.Branches {
			bs, ok := branchStats[b]
			if !ok {
				bs = &BranchSummary{Branch: b, Models: map[string]int{}, ModelStats: map[string]*ModelStat{}}
				branchStats[b] = bs
			}
			bs.Count++
			bs.Insertions += r.Insertions
			bs.Deletions += r.Deletions
			if r.Model != "" {
				bs.Models[r.Model]++
				ms, ok := bs.ModelStats[r.Model]
				if !ok {
					ms = &ModelStat{}
					bs.ModelStats[r.Model] = ms
				}
				ms.Count++
				ms.Insertions += r.Insertions
				ms.Deletions += r.Deletions
			}
		}
	}
	for _, bs := range branchStats {
		output.BranchSummaries = append(output.BranchSummaries, *bs)
	}

	return output, nil
}

// enrichDiffStats shells out to git log --shortstat to get additions/deletions
// per commit. This is much faster than computing diffs in Go.
func enrichDiffStats(repoPath string, output *ScanOutput) {
	if len(output.Results) == 0 {
		return
	}

	// Build a set of hashes we care about
	hashSet := make(map[string]int, len(output.Results))
	for i, r := range output.Results {
		hashSet[r.Hash] = i
	}

	// git log --format=%H --shortstat --all
	cmd := exec.Command("git", "log", "--format=%H", "--shortstat", "--all")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return // silently skip — stats are optional
	}

	lines := strings.Split(string(out), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if len(line) != 40 {
			continue
		}
		idx, ok := hashSet[line]
		if !ok {
			continue
		}
		// Next non-empty line should be the shortstat
		for j := i + 1; j < len(lines) && j <= i+2; j++ {
			stat := strings.TrimSpace(lines[j])
			if stat == "" {
				continue
			}
			files, ins, del := parseShortStat(stat)
			output.Results[idx].FilesChanged = files
			output.Results[idx].Insertions = ins
			output.Results[idx].Deletions = del
			break
		}
	}
}

// parseShortStat parses a git --shortstat line like:
// " 3 files changed, 12 insertions(+), 4 deletions(-)"
func parseShortStat(line string) (files, insertions, deletions int) {
	parts := strings.Split(line, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		var n int
		if _, err := fmt.Sscanf(p, "%d file", &n); err == nil {
			files = n
		} else if _, err := fmt.Sscanf(p, "%d insertion", &n); err == nil {
			insertions = n
		} else if _, err := fmt.Sscanf(p, "%d deletion", &n); err == nil {
			deletions = n
		}
	}
	return
}
