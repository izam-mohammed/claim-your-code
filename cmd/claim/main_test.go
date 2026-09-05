// Shared test harness for the command layer: stdout capture, prompt stubs,
// a stubbed exit, and helpers for building throwaway git repositories.

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/izam-mohammed/claim-your-code/internal/report"
	"github.com/izam-mohammed/claim-your-code/internal/rewriter"
)

func TestMain(m *testing.M) {
	color.NoColor = true // assert on plain text, not escape codes
	// No test in this package may draw the report picker. Nothing here can
	// answer it, and where a console is attached -- the Windows runner --
	// huh blocks on a keypress until the package times out.
	report.SetSelectReport(func([]huh.Option[string]) (string, error) {
		return "", fmt.Errorf("no terminal")
	})
	os.Exit(m.Run())
}

// errExit is what the stubbed exit panics with, so a fatal path unwinds the
// way os.Exit would end the process instead of falling through.
type errExit struct{ code int }

// runMain runs fn with os.Args set, stdout and stderr captured, and exit
// stubbed. It returns everything printed and the exit code, if one was raised.
func runMain(t *testing.T, args []string, fn func()) (output string, exitCode int, exited bool) {
	t.Helper()

	prevArgs, prevExit := os.Args, exit
	os.Args = args
	exit = func(code int) { panic(errExit{code}) }
	t.Cleanup(func() { os.Args, exit = prevArgs, prevExit })

	origOut, origErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				e, ok := rec.(errExit)
				if !ok {
					w.Close()
					os.Stdout, os.Stderr = origOut, origErr
					panic(rec)
				}
				exitCode, exited = e.code, true
			}
		}()
		fn()
	}()

	w.Close()
	os.Stdout, os.Stderr = origOut, origErr
	return <-done, exitCode, exited
}

// stubPrompts replaces every interactive prompt for the duration of a test.
// Unset fields answer in the least destructive way: no, and cancelled.
type prompts struct {
	confirm    func(string) bool
	dangerous  func(string) bool
	selectOne  func(string, []huh.Option[string], int) (string, error)
	selectMany func(string, []huh.Option[string], int) ([]string, error)
}

func stubPrompts(t *testing.T, p prompts) {
	t.Helper()
	prevC, prevD, prevS, prevM := confirm, confirmDangerous, selectOne, selectMany

	confirm = func(title string) bool {
		if p.confirm == nil {
			return false
		}
		return p.confirm(title)
	}
	confirmDangerous = func(title string) bool {
		if p.dangerous == nil {
			return false
		}
		return p.dangerous(title)
	}
	selectOne = func(title string, opts []huh.Option[string], h int) (string, error) {
		if p.selectOne == nil {
			return "", fmt.Errorf("no terminal")
		}
		return p.selectOne(title, opts, h)
	}
	selectMany = func(title string, opts []huh.Option[string], h int) ([]string, error) {
		if p.selectMany == nil {
			return nil, fmt.Errorf("no terminal")
		}
		return p.selectMany(title, opts, h)
	}

	t.Cleanup(func() {
		confirm, confirmDangerous, selectOne, selectMany = prevC, prevD, prevS, prevM
	})
}

// answer builds a selectOne stub that always returns value.
func answer(value string) func(string, []huh.Option[string], int) (string, error) {
	return func(string, []huh.Option[string], int) (string, error) { return value, nil }
}

// isolateData points the report and account stores at a temp directory.
func isolateData(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", tmp)
	case "windows":
		t.Setenv("LocalAppData", tmp)
	default:
		t.Setenv("XDG_DATA_HOME", tmp)
	}
}

// breakDataDir makes report.DataDir fail for the rest of the test, so the
// paths that warn about a report they could not save can be exercised. Each
// OS derives the directory from a different variable, so clearing one is not
// enough -- on Windows HOME means nothing and %LocalAppData% decides.
func breakDataDir(t *testing.T) {
	t.Helper()
	for _, key := range dataDirVars {
		t.Setenv(key, "")
	}
}

// breakDataDirMidRun is breakDataDir for a flow that has to load a report
// before the directory goes away, so it cannot be broken up front. The
// returned function puts the environment back.
func breakDataDirMidRun() (restore func()) {
	prev := make(map[string]string, len(dataDirVars))
	for _, key := range dataDirVars {
		prev[key] = os.Getenv(key)
		os.Setenv(key, "")
	}
	return func() {
		for key, value := range prev {
			if value == "" {
				os.Unsetenv(key)
				continue
			}
			os.Setenv(key, value)
		}
	}
}

// dataDirVars are every variable report.DataDir consults, across platforms.
var dataDirVars = []string{"HOME", "XDG_DATA_HOME", "LocalAppData"}

// stubFilter points the rewriter at a shell stub that strips Claude trailers,
// standing in for the real claim binary.
func stubFilter(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub filter is a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "claim-stub")
	body := "#!/bin/sh\nsed '/[Cc]o-[Aa]uthored-[Bb]y: Claude/d'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rewriter.SetExecutable(script))
}

const claudeTrailer = "Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"

// git runs a git command in dir.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo creates an initialised git repository at dir.
func newRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	return dir
}

// commit adds a file and commits it with the given message.
func commit(t *testing.T, repo, file, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, file), []byte(file+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", file)
	git(t, repo, "commit", "-m", message)
}

// dirtyRepo creates a repository with one clean and one Claude-co-authored commit.
func dirtyRepo(t *testing.T, dir string) string {
	t.Helper()
	repo := newRepo(t, dir)
	commit(t, repo, "clean.txt", "feat: a clean commit")
	commit(t, repo, "dirty.txt", "feat: a claimed commit\n\n"+claudeTrailer)
	return repo
}
