package report

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/izam-mohammed/claim-your-code/internal/scanner"
)

const (
	DirName   = ".claim"
	IDPrefix  = "clm"
	gitIgnore = ".claim/"
)

// Report is the top-level structure persisted to disk.
type Report struct {
	ID           string            `json:"id"`
	Version      string            `json:"version"`
	Repo         string            `json:"repo"`
	CreatedAt    time.Time         `json:"created_at"`
	Summary      Summary           `json:"summary"`
	Branches     []Branch          `json:"branches"`
	Commits      []Commit          `json:"commits"`
	OriginalRefs map[string]string `json:"original_refs,omitempty"` // branch name -> commit hash before rewrite
	Result       *Result           `json:"result,omitempty"`
	Reverted     *Reverted         `json:"reverted,omitempty"`
}

type Summary struct {
	TotalCommits    int            `json:"total_commits"`
	AffectedCommits int            `json:"affected_commits"`
	Models          map[string]int `json:"models"`
}

type Branch struct {
	Name   string         `json:"name"`
	Count  int            `json:"count"`
	Models map[string]int `json:"models"`
}

type Commit struct {
	Hash     string   `json:"hash"`
	Subject  string   `json:"subject"`
	Model    string   `json:"model"`
	Branches []string `json:"branches"`
}

type Result struct {
	Status      string    `json:"status"` // "cleaned", "dry_run", "aborted"
	CompletedAt time.Time `json:"completed_at"`
	Cleaned     int       `json:"cleaned"`
}

type Reverted struct {
	RevertedAt time.Time `json:"reverted_at"`
}

// generateID creates a short unique ID like "clm_a3f8b1".
func generateID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", IDPrefix, hex.EncodeToString(b))
}

// Build creates a Report from scan results.
func Build(version, repoPath string, totalCommits int, results []scanner.Result, branchSummaries []scanner.BranchSummary) *Report {
	models := map[string]int{}
	commits := make([]Commit, len(results))
	for i, r := range results {
		if r.Model != "" {
			models[r.Model]++
		}
		commits[i] = Commit{
			Hash:     r.Hash,
			Subject:  r.Subject,
			Model:    r.Model,
			Branches: r.Branches,
		}
	}

	branches := make([]Branch, len(branchSummaries))
	for i, bs := range branchSummaries {
		branches[i] = Branch{
			Name:   bs.Branch,
			Count:  bs.Count,
			Models: bs.Models,
		}
	}

	return &Report{
		ID:        generateID(),
		Version:   version,
		Repo:      repoPath,
		CreatedAt: time.Now().UTC(),
		Summary: Summary{
			TotalCommits:    totalCommits,
			AffectedCommits: len(results),
			Models:          models,
		},
		Branches: branches,
		Commits:  commits,
	}
}

// SetOriginalRefs stores branch -> hash mapping before rewrite.
func (r *Report) SetOriginalRefs(refs map[string]string) {
	r.OriginalRefs = refs
}

// SetResult records the outcome of the rewrite operation.
func (r *Report) SetResult(status string, cleaned int) {
	r.Result = &Result{
		Status:      status,
		CompletedAt: time.Now().UTC(),
		Cleaned:     cleaned,
	}
}

// SetReverted marks this report as reverted.
func (r *Report) SetReverted() {
	r.Reverted = &Reverted{
		RevertedAt: time.Now().UTC(),
	}
}

// IsRevertible returns true if this report was a successful clean that hasn't been reverted.
func (r *Report) IsRevertible() bool {
	return r.Result != nil && r.Result.Status == "cleaned" && r.Reverted == nil && len(r.OriginalRefs) > 0
}

// Save writes the report as JSON to .claim/<id>.json in the repo root.
func (r *Report) Save(repoPath string) error {
	if err := ensureGitignore(repoPath); err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}

	dir := filepath.Join(repoPath, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, r.ID+".json"), data, 0o644)
}

// List reads all reports from .claim/ directory, sorted newest first.
func List(repoPath string) ([]*Report, error) {
	dir := filepath.Join(repoPath, DirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var reports []*Report
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		rpt, err := loadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		reports = append(reports, rpt)
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})

	return reports, nil
}

// Load reads a single report by ID from .claim/<id>.json.
func Load(repoPath, id string) (*Report, error) {
	return loadFile(filepath.Join(repoPath, DirName, id+".json"))
}

// FindByPrefix finds a report whose ID starts with the given prefix.
func FindByPrefix(repoPath, prefix string) (*Report, error) {
	reports, err := List(repoPath)
	if err != nil {
		return nil, err
	}

	var matches []*Report
	for _, r := range reports {
		if strings.HasPrefix(r.ID, prefix) {
			matches = append(matches, r)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no report found matching '%s'", prefix)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("ambiguous prefix '%s': matches %d reports", prefix, len(matches))
	}
}

func loadFile(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ensureGitignore adds .claim/ to .gitignore if not already present.
func ensureGitignore(repoPath string) error {
	gitignorePath := filepath.Join(repoPath, ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if already ignored
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == gitIgnore || trimmed == ".claim" {
			return nil
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	_, err = f.WriteString(gitIgnore + "\n")
	return err
}
