package rewriter

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Rewrite uses git filter-branch to strip Claude co-author lines from all commits.
// It invokes the claim binary's hidden __filter-msg subcommand as the msg-filter.
func Rewrite(repoPath string) error {
	claimBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find claim binary path: %w", err)
	}

	cmd := exec.Command("git", "filter-branch", "--force",
		"--msg-filter", fmt.Sprintf("%s __filter-msg", claimBinary),
		"--", "--all",
	)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "FILTER_BRANCH_SQUELCH_WARNING=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git filter-branch failed: %w", err)
	}

	// Clean up backup refs created by filter-branch
	cleanCmd := exec.Command("git", "for-each-ref", "--format=%(refname)", "refs/original/")
	cleanCmd.Dir = repoPath
	out, err := cleanCmd.Output()
	if err == nil && len(out) > 0 {
		for _, ref := range splitLines(string(out)) {
			if ref == "" {
				continue
			}
			delCmd := exec.Command("git", "update-ref", "-d", ref)
			delCmd.Dir = repoPath
			_ = delCmd.Run()
		}
	}

	return nil
}

// GetBranchRefs returns a map of branch name -> commit hash for all local branches.
func GetBranchRefs(repoPath string) (map[string]string, error) {
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read branch refs: %w", err)
	}

	refs := map[string]string{}
	for _, line := range splitLines(string(out)) {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			refs[parts[0]] = parts[1]
		}
	}
	return refs, nil
}

// Revert restores branches to the given original refs.
func Revert(repoPath string, originalRefs map[string]string) error {
	for branch, hash := range originalRefs {
		cmd := exec.Command("git", "update-ref", fmt.Sprintf("refs/heads/%s", branch), hash)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to restore %s to %s: %w\n%s", branch, hash[:8], err, out)
		}
	}
	return nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
