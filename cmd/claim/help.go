// The help system: one overview and a page per command.
//
// The tables below are the single source of truth for what claim accepts, so
// `claim --help` and `claim <command> --help` can never drift from each other.

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// flagDoc documents a single flag.
type flagDoc struct {
	name string
	desc string
}

// commandDoc documents one subcommand.
type commandDoc struct {
	name     string
	args     string // argument spec shown after the command name
	synopsis string // one line, shown in the overview
	detail   string // longer description, shown on the command's own page
	flags    []flagDoc
	examples []string
}

// Flags shared by the remote commands that can rewrite and push.
var (
	flagDryRun  = flagDoc{"--dry-run", "Report what would change and exit without modifying anything."}
	flagForce   = flagDoc{"--force, -f", "Skip confirmation prompts and accept the default answer."}
	flagApply   = flagDoc{"--apply", "Rewrite and force-push. Without it, remote commands only scan."}
	flagAPIOnly = flagDoc{"--api-only", "Scan the 100 most recent commits over the API instead of cloning. Faster, but misses older history."}
)

// globalFlags apply to every invocation.
var globalFlags = []flagDoc{
	{"--help, -h", "Show help. Use `claim <command> --help` for one command."},
	{"--version, -v", "Print the claim version and exit."},
}

// commands is every user-facing command, in the order help lists them.
var commands = []commandDoc{
	{
		name:     "<folder>",
		args:     "",
		synopsis: "Scan and clean local git repositories",
		detail: "Scans a local repository for commits co-authored by Claude and rewrites\n" +
			"their messages to remove the trailer.\n\n" +
			"If <folder> is not itself a git repository, claim searches up to 4 levels\n" +
			"deep and asks which of the repositories it finds to work on. If it is a\n" +
			"repository that contains others, claim offers to include the nested ones.\n\n" +
			"You are asked whether to scan the default branch only or every branch, then\n" +
			"shown what was found and asked to confirm before any history is rewritten.\n" +
			"Nothing is pushed unless you confirm a second, explicit prompt.",
		flags: []flagDoc{flagDryRun, flagForce},
		examples: []string{
			"claim .                      # the repository in the current directory",
			"claim ~/code/my-project      # a specific repository",
			"claim ~/code                 # search ~/code for repositories",
			"claim . --dry-run            # report only, change nothing",
			"claim . --force              # take every default, no prompts",
		},
	},
	{
		name:     "<github-url>",
		args:     "",
		synopsis: "Auto-detect a repo, PR, or user from a GitHub URL",
		detail: "Accepts any GitHub URL and routes it to the matching command:\n" +
			"a /pull/N URL runs `claim pr`, a profile or /orgs/ URL runs `claim user`,\n" +
			"and anything else runs `claim repo`. SSH and scheme-less forms work too.\n\n" +
			"Like the other remote commands, this only scans unless you pass --apply.",
		flags: []flagDoc{flagDryRun, flagForce, flagApply, flagAPIOnly},
		examples: []string{
			"claim https://github.com/owner/repo",
			"claim https://github.com/owner/repo/pull/42",
			"claim https://github.com/owner/repo/tree/some-branch",
			"claim git@github.com:owner/repo.git",
			"claim github.com/owner",
		},
	},
	{
		name:     "repo",
		args:     "<owner/repo or URL>",
		synopsis: "Scan a remote GitHub repository",
		detail: "Clones the repository to a temporary directory, scans its full history,\n" +
			"and reports what it finds. The clone is deleted when the command exits.\n\n" +
			"This is scan-only by default. Pass --apply to rewrite the history and be\n" +
			"offered a force-push; the push itself needs a further typed confirmation.\n\n" +
			"Public repositories work without authentication. Private ones, and any run\n" +
			"with --apply, will prompt you to authenticate.",
		flags: []flagDoc{flagDryRun, flagForce, flagApply, flagAPIOnly},
		examples: []string{
			"claim repo owner/repo                  # scan only",
			"claim repo owner/repo --api-only       # quick scan, no clone",
			"claim repo owner/repo --apply          # rewrite, then offer to push",
			"claim repo https://github.com/o/r/tree/dev",
		},
	},
	{
		name:     "pr",
		args:     "<owner/repo#N or PR URL>",
		synopsis: "Scan a pull request",
		detail: "Clones the repository, checks out the pull request's head branch, and scans\n" +
			"it. With --apply the head branch is rewritten and you are offered a\n" +
			"force-push of that branch only.\n\n" +
			"Force-pushing a pull request branch rewrites the commits contributors see.",
		flags: []flagDoc{flagDryRun, flagForce, flagApply, flagAPIOnly},
		examples: []string{
			"claim pr owner/repo#42",
			"claim pr https://github.com/owner/repo/pull/42",
			"claim pr owner/repo#42 --apply",
		},
	},
	{
		name:     "org",
		args:     "<org-name or URL>",
		synopsis: "Scan every repository in an organization",
		detail: "Lists the organization's repositories, excluding archived ones and forks,\n" +
			"and asks which to scan and whether to scan over the API (fast, recent\n" +
			"commits only) or by cloning each one (slower, complete history).\n\n" +
			"Multi-repo scans never rewrite anything: they report and record a scan.\n" +
			"Clean an individual repository with `claim repo <owner/repo> --apply`.",
		flags: []flagDoc{flagForce},
		examples: []string{
			"claim org my-organization",
			"claim org my-organization --force    # select all repos without prompting",
			"claim org https://github.com/orgs/my-organization",
		},
	},
	{
		name:     "user",
		args:     "<username or URL>",
		synopsis: "Scan every repository owned by a user",
		detail: "Lists the repositories the user owns, excluding archived ones and forks,\n" +
			"and asks which to scan and how, exactly as `claim org` does.\n\n" +
			"Multi-repo scans never rewrite anything: they report and record a scan.",
		flags: []flagDoc{flagForce},
		examples: []string{
			"claim user izam-mohammed",
			"claim user izam-mohammed --force",
			"claim user https://github.com/izam-mohammed",
		},
	},
	{
		name:     "report",
		args:     "[folder] [id|all]",
		synopsis: "List or show past claim reports",
		detail: "Every scan and clean is recorded, encrypted, under claim's data directory.\n" +
			"With no arguments you get a picker of every report. Give an id to show one\n" +
			"in detail, `all` to show every report in detail, or a folder to limit the\n" +
			"listing to that repository.\n\n" +
			"An id may be abbreviated as long as the prefix is unambiguous.",
		examples: []string{
			"claim report                  # pick from every report",
			"claim report clm_a3f8b1       # show one report",
			"claim report all              # show every report in detail",
			"claim report ~/code/my-repo   # only reports for that repository",
		},
	},
	{
		name:     "revert",
		args:     "<id>",
		synopsis: "Restore the branches a clean rewrote",
		detail: "Restores every branch a recorded clean touched to the commit it pointed at\n" +
			"beforehand, putting the Claude co-author trailers back.\n\n" +
			"This only touches local branches. If you already force-pushed the cleaned\n" +
			"history, push again afterwards to restore the remote.",
		examples: []string{
			"claim revert clm_a3f8b1",
		},
	},
	{
		name:     "logout",
		args:     "[username]",
		synopsis: "Remove saved GitHub accounts",
		detail: "Saved accounts are stored encrypted under a key derived from this machine.\n" +
			"Naming a username removes that account; with no argument you are asked which\n" +
			"account to remove, or offered to remove all of them.",
		examples: []string{
			"claim logout",
			"claim logout izam-mohammed",
		},
	},
}

// authNote is appended to the overview: agents need to know when a run may block.
const authNote = `Authentication:
  Public repositories are read without a token. Private repositories, and any
  run with --apply, prompt you to pick or add a GitHub account. Set
  CLAIM_GITHUB_TOKEN or GITHUB_TOKEN, or sign in with the gh CLI, to avoid the
  prompt.

Exit status:
  0  success, including scans that found nothing
  1  usage error, or the command could not complete`

// findCommand returns the doc for a named command.
func findCommand(name string) (commandDoc, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return commandDoc{}, false
}

// isHelpFlag reports whether an argument asks for help.
func isHelpFlag(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

// wantsHelp reports whether any argument after the command name asks for help.
func wantsHelp(args []string) bool {
	for _, a := range args {
		if isHelpFlag(a) {
			return true
		}
	}
	return false
}

// writeUsage renders the overview.
func writeUsage(w io.Writer) {
	fmt.Fprintf(w, "%s - Remove Claude as co-author from your git commits\n\n", bold("claim"))

	fmt.Fprintf(w, "%s\n", bold("Usage:"))
	fmt.Fprintf(w, "  claim <folder>                     Scan and clean local git repositories\n")
	fmt.Fprintf(w, "  claim <github-url>                 Auto-detect a repo, PR, or user from a URL\n")
	fmt.Fprintf(w, "  claim <command> [arguments]\n\n")

	fmt.Fprintf(w, "%s\n", bold("Commands:"))
	for _, c := range commands {
		if strings.HasPrefix(c.name, "<") {
			continue // the two forms above, already shown under Usage
		}
		label := c.name
		if c.args != "" {
			label += " " + c.args
		}
		fmt.Fprintf(w, "  %-34s %s\n", label, c.synopsis)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "%s\n", bold("Flags:"))
	for _, f := range []flagDoc{flagDryRun, flagForce, flagApply, flagAPIOnly} {
		fmt.Fprintf(w, "  %-34s %s\n", f.name, f.desc)
	}
	for _, f := range globalFlags {
		fmt.Fprintf(w, "  %-34s %s\n", f.name, f.desc)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "%s\n", bold("Examples:"))
	for _, ex := range []string{
		"claim .                                Clean the current repository",
		"claim . --dry-run                      Report only, change nothing",
		"claim repo owner/repo                  Scan a remote repository",
		"claim pr owner/repo#42 --apply         Clean and push a pull request branch",
		"claim report                           Browse past reports",
	} {
		fmt.Fprintf(w, "  %s\n", ex)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "%s\n\n", authNote)
	fmt.Fprintf(w, "Run %s for detail on one command.\n", cyan("claim <command> --help"))
}

// writeCommandHelp renders one command's page.
func writeCommandHelp(w io.Writer, c commandDoc) {
	label := "claim " + c.name
	if c.args != "" {
		label += " " + c.args
	}

	fmt.Fprintf(w, "%s - %s\n\n", bold(label), c.synopsis)

	fmt.Fprintf(w, "%s\n  %s\n\n", bold("Usage:"), label)

	if c.detail != "" {
		fmt.Fprintf(w, "%s\n", bold("Description:"))
		for _, line := range strings.Split(c.detail, "\n") {
			if line == "" {
				fmt.Fprintln(w)
				continue
			}
			fmt.Fprintf(w, "  %s\n", line)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "%s\n", bold("Flags:"))
	for _, f := range c.flags {
		fmt.Fprintf(w, "  %-34s %s\n", f.name, f.desc)
	}
	for _, f := range globalFlags {
		fmt.Fprintf(w, "  %-34s %s\n", f.name, f.desc)
	}
	fmt.Fprintln(w)

	if len(c.examples) > 0 {
		fmt.Fprintf(w, "%s\n", bold("Examples:"))
		for _, ex := range c.examples {
			fmt.Fprintf(w, "  %s\n", ex)
		}
		fmt.Fprintln(w)
	}
}

// printUsage writes the overview to stderr, for the no-arguments error case.
func printUsage() { writeUsage(os.Stderr) }

// showHelp writes the overview to stdout, for an explicit --help.
func showHelp() { writeUsage(os.Stdout) }

// showCommandHelp writes one command's page to stdout. It reports whether the
// command was known.
func showCommandHelp(name string) bool {
	c, ok := findCommand(name)
	if !ok {
		return false
	}
	writeCommandHelp(os.Stdout, c)
	return true
}
