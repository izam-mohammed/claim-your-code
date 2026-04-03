package scanner

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/izam-mohammed/claim-your-code/internal/pattern"
)

// Result represents a commit that contains a Claude co-author line.
type Result struct {
	Hash     string
	Subject  string // First line of the commit message
	Model    string // e.g. "Claude Opus 4.6"
	Branches []string
}

// BranchSummary holds per-branch stats.
type BranchSummary struct {
	Branch string
	Count  int
	Models map[string]int // model name -> count
}

// Scan opens a git repository at repoPath and returns all commits
// that contain a Claude Co-Authored-By line, along with branch info.
func Scan(repoPath string) ([]Result, []BranchSummary, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, nil, err
	}

	// Build a map of commit hash -> branch names
	commitBranches := map[string][]string{}
	branchTips := map[string]plumbing.Hash{}

	refs, err := repo.References()
	if err != nil {
		return nil, nil, err
	}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() {
			branchName := ref.Name().Short()
			branchTips[branchName] = ref.Hash()
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// For each branch, walk its commits and record membership
	for branchName, tipHash := range branchTips {
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

	// Scan all commits for Claude co-author
	iter, err := repo.Log(&git.LogOptions{All: true})
	if err != nil {
		return nil, nil, err
	}

	seen := map[string]bool{}
	var results []Result
	err = iter.ForEach(func(c *object.Commit) error {
		h := c.Hash.String()
		if seen[h] {
			return nil
		}
		seen[h] = true

		if pattern.ContainsClaudeCoAuthor(c.Message) {
			model := pattern.ExtractModelName(c.Message)
			branches := commitBranches[h]
			if len(branches) == 0 {
				branches = []string{"(detached)"}
			}
			results = append(results, Result{
				Hash:     h,
				Subject:  firstLine(c.Message),
				Model:    model,
				Branches: branches,
			})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// Build per-branch summary
	branchStats := map[string]*BranchSummary{}
	for _, r := range results {
		for _, b := range r.Branches {
			bs, ok := branchStats[b]
			if !ok {
				bs = &BranchSummary{Branch: b, Models: map[string]int{}}
				branchStats[b] = bs
			}
			bs.Count++
			if r.Model != "" {
				bs.Models[r.Model]++
			}
		}
	}

	var summaries []BranchSummary
	for _, bs := range branchStats {
		summaries = append(summaries, *bs)
	}

	return results, summaries, nil
}

func firstLine(s string) string {
	for i, ch := range s {
		if ch == '\n' {
			return s[:i]
		}
	}
	return s
}
