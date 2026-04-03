package rewriter

import (
	"fmt"
	"os"
	"os/exec"
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
