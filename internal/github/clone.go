package github

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Clone performs a full git clone of a repo to a temp directory.
// Returns the path to the cloned repo.
func Clone(cloneURL, parentDir string) (string, error) {
	// Extract repo name from URL for the directory name
	repoName := filepath.Base(cloneURL)
	if len(repoName) > 4 && repoName[len(repoName)-4:] == ".git" {
		repoName = repoName[:len(repoName)-4]
	}

	destPath := filepath.Join(parentDir, repoName)

	cmd := exec.Command("git", "clone", "--quiet", cloneURL, destPath)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone failed: %w\n%s", err, out)
	}

	return destPath, nil
}

// CloneBare performs a bare git clone (no working tree, faster for scanning).
func CloneBare(cloneURL, parentDir string) (string, error) {
	repoName := filepath.Base(cloneURL)
	if len(repoName) > 4 && repoName[len(repoName)-4:] == ".git" {
		repoName = repoName[:len(repoName)-4]
	}

	destPath := filepath.Join(parentDir, repoName+".git")

	cmd := exec.Command("git", "clone", "--bare", "--quiet", cloneURL, destPath)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone --bare failed: %w\n%s", err, out)
	}

	return destPath, nil
}

// Checkout switches to a specific branch in a cloned repo.
func Checkout(repoPath, branch string) error {
	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s failed: %w\n%s", branch, err, out)
	}
	return nil
}

// PushForce force-pushes a branch to the origin remote.
func PushForce(repoPath, branch string) error {
	cmd := exec.Command("git", "push", "--force-with-lease", "origin", branch)
	cmd.Dir = repoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push --force-with-lease failed: %w", err)
	}
	return nil
}

// CreateTempDir creates a temporary directory for cloning.
func CreateTempDir() (string, error) {
	return os.MkdirTemp("", "claim-*")
}

// CleanupTempDir removes a temporary clone directory.
func CleanupTempDir(dir string) {
	_ = os.RemoveAll(dir)
}
