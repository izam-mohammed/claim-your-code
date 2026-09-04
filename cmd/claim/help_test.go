package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestEveryCommandIsDocumented(t *testing.T) {
	// Every command dispatch handles must have a help page, so an agent
	// reading `claim --help` can discover all of them.
	dispatched := []string{"repo", "pr", "org", "user", "logout", "report", "revert"}
	for _, name := range dispatched {
		if _, ok := findCommand(name); !ok {
			t.Errorf("command %q is dispatched but has no help entry", name)
		}
	}
	if _, ok := findCommand("<folder>"); !ok {
		t.Error("the local folder form has no help entry")
	}
	if _, ok := findCommand("<github-url>"); !ok {
		t.Error("the URL form has no help entry")
	}
}

func TestCommandDocsAreComplete(t *testing.T) {
	for _, c := range commands {
		t.Run(c.name, func(t *testing.T) {
			if c.synopsis == "" {
				t.Error("synopsis is empty")
			}
			if strings.HasSuffix(c.synopsis, ".") {
				t.Errorf("synopsis %q should not end in a full stop — the overview lists them as labels", c.synopsis)
			}
			if c.detail == "" {
				t.Error("detail is empty")
			}
			if len(c.examples) == 0 {
				t.Error("no examples given")
			}
			for _, ex := range c.examples {
				if !strings.HasPrefix(ex, "claim") {
					t.Errorf("example %q should start with `claim`", ex)
				}
			}
			for _, f := range c.flags {
				if f.desc == "" {
					t.Errorf("flag %q has no description", f.name)
				}
			}
		})
	}
}

func TestParsedFlagsAreDocumented(t *testing.T) {
	// Each flag parseRemoteFlags understands must appear in the help of at
	// least one command, or callers cannot discover it.
	documented := map[string]bool{}
	for _, c := range commands {
		for _, f := range c.flags {
			for _, part := range strings.Split(f.name, ",") {
				documented[strings.TrimSpace(part)] = true
			}
		}
	}
	for _, f := range globalFlags {
		for _, part := range strings.Split(f.name, ",") {
			documented[strings.TrimSpace(part)] = true
		}
	}

	for _, flag := range []string{"--dry-run", "--force", "-f", "--apply", "--api-only", "--help", "-h", "--version", "-v"} {
		if !documented[flag] {
			t.Errorf("flag %q is accepted but never documented", flag)
		}
	}
}

func TestCommandsDocumentOnlyFlagsTheyHonour(t *testing.T) {
	// org and user never rewrite, so advertising --apply there would mislead.
	for _, name := range []string{"org", "user"} {
		c, ok := findCommand(name)
		if !ok {
			t.Fatalf("no help entry for %q", name)
		}
		for _, f := range c.flags {
			if strings.Contains(f.name, "--apply") || strings.Contains(f.name, "--dry-run") {
				t.Errorf("%s documents %q, but it never rewrites anything", name, f.name)
			}
		}
	}
}

func TestWriteUsageListsEveryCommandAndFlag(t *testing.T) {
	var buf bytes.Buffer
	writeUsage(&buf)
	out := buf.String()

	for _, want := range []string{
		"claim <folder>", "claim <github-url>",
		"repo", "pr", "org", "user", "report", "revert", "logout",
		"--dry-run", "--force", "--apply", "--api-only", "--help", "--version",
		"Authentication:", "Exit status:", "Examples:",
		"CLAIM_GITHUB_TOKEN", "GITHUB_TOKEN",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the overview is missing %q", want)
		}
	}
}

func TestWriteUsageDoesNotListThePlaceholderNames(t *testing.T) {
	var buf bytes.Buffer
	writeUsage(&buf)
	// The <folder> and <github-url> forms belong under Usage, not in the
	// Commands table where they would read as literal subcommand names.
	commandsSection := buf.String()
	if i := strings.Index(commandsSection, "Commands:"); i >= 0 {
		j := strings.Index(commandsSection[i:], "Flags:")
		section := commandsSection[i : i+j]
		if strings.Contains(section, "<folder>") || strings.Contains(section, "<github-url>") {
			t.Errorf("the Commands table should not repeat the bare forms:\n%s", section)
		}
	}
}

func TestWriteCommandHelpRendersEveryPart(t *testing.T) {
	c, ok := findCommand("repo")
	if !ok {
		t.Fatal("no repo command")
	}
	var buf bytes.Buffer
	writeCommandHelp(&buf, c)
	out := buf.String()

	for _, want := range []string{
		"claim repo <owner/repo or URL>",
		"Usage:", "Description:", "Flags:", "Examples:",
		"--apply", "--api-only", "--help",
		"scan-only by default",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the repo help page is missing %q\n---\n%s", want, out)
		}
	}
}

func TestWriteCommandHelpWithoutExamplesOrFlags(t *testing.T) {
	var buf bytes.Buffer
	writeCommandHelp(&buf, commandDoc{name: "bare", synopsis: "does little"})
	out := buf.String()
	if !strings.Contains(out, "claim bare") || !strings.Contains(out, "does little") {
		t.Errorf("minimal help page = %q", out)
	}
	if strings.Contains(out, "Examples:") {
		t.Error("a command with no examples should not print an Examples heading")
	}
	if !strings.Contains(out, "--help") {
		t.Error("global flags should appear even on a command with none of its own")
	}
}

func TestEveryCommandPageRenders(t *testing.T) {
	for _, c := range commands {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeCommandHelp(&buf, c)
			out := buf.String()
			if !strings.Contains(out, c.synopsis) {
				t.Errorf("page is missing its synopsis")
			}
			if !strings.Contains(out, "Usage:") {
				t.Error("page is missing a Usage section")
			}
		})
	}
}

func TestFindCommandUnknown(t *testing.T) {
	if _, ok := findCommand("no-such-command"); ok {
		t.Error("findCommand matched a command that does not exist")
	}
}

func TestIsHelpFlag(t *testing.T) {
	for _, arg := range []string{"--help", "-h", "help"} {
		if !isHelpFlag(arg) {
			t.Errorf("isHelpFlag(%q) = false, want true", arg)
		}
	}
	for _, arg := range []string{"", "--h", "-help", "repo", "--force"} {
		if isHelpFlag(arg) {
			t.Errorf("isHelpFlag(%q) = true, want false", arg)
		}
	}
}

func TestWantsHelp(t *testing.T) {
	if !wantsHelp([]string{"owner/repo", "--help"}) {
		t.Error("wantsHelp missed --help after an argument")
	}
	if !wantsHelp([]string{"-h"}) {
		t.Error("wantsHelp missed -h")
	}
	if wantsHelp([]string{"owner/repo", "--apply"}) {
		t.Error("wantsHelp matched a run with no help flag")
	}
	if wantsHelp(nil) {
		t.Error("wantsHelp(nil) = true")
	}
}

func TestShowCommandHelpUnknown(t *testing.T) {
	if showCommandHelp("nope") {
		t.Error("showCommandHelp reported success for an unknown command")
	}
}
