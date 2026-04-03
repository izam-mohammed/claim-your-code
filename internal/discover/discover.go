package discover

import (
	"os"
	"path/filepath"
	"strings"
)

// skipDirs are directories we never descend into during search.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".cache":       true,
	".npm":         true,
	".yarn":        true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	".tox":         true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".nuxt":        true,
	".output":      true,
	"target":       true, // rust/java
	".gradle":      true,
	".m2":          true,
	".cargo":       true,
	"Pods":         true,
	".terraform":   true,
}

// Repo represents a discovered git repository.
type Repo struct {
	Path string // absolute path to the repo root (parent of .git)
	Name string // last path component
}

// IsGitRepo checks if the given path contains a .git directory.
func IsGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

// FindRepos searches for git repositories under root.
// It uses breadth-first search and stops descending into a directory
// once a .git folder is found there (won't look deeper inside that repo).
// maxDepth limits how deep the search goes (0 = root only).
func FindRepos(root string, maxDepth int) []Repo {
	var repos []Repo
	type entry struct {
		path  string
		depth int
	}

	queue := []entry{{path: root, depth: 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if IsGitRepo(current.path) {
			name := filepath.Base(current.path)
			repos = append(repos, Repo{Path: current.path, Name: name})
			// Don't descend into this repo's subdirectories
			continue
		}

		if current.depth >= maxDepth {
			continue
		}

		entries, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			// Skip hidden dirs (except we already checked .git above)
			if strings.HasPrefix(name, ".") {
				continue
			}
			if skipDirs[name] {
				continue
			}
			queue = append(queue, entry{
				path:  filepath.Join(current.path, name),
				depth: current.depth + 1,
			})
		}
	}

	return repos
}

// FindNestedRepos searches for git repositories nested inside a repo
// (e.g. submodules or nested projects). Searches up to 3 levels deep
// and skips the repo's own .git directory.
func FindNestedRepos(repoPath string) []Repo {
	type entry struct {
		path  string
		depth int
	}

	var nested []Repo
	queue := []entry{{path: repoPath, depth: 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		entries, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				continue
			}
			subPath := filepath.Join(current.path, name)
			if IsGitRepo(subPath) {
				relName, _ := filepath.Rel(repoPath, subPath)
				nested = append(nested, Repo{Path: subPath, Name: relName})
				// Don't descend further into this nested repo
				continue
			}
			if current.depth < 3 {
				queue = append(queue, entry{path: subPath, depth: current.depth + 1})
			}
		}
	}

	return nested
}
