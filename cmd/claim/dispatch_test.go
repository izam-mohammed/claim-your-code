package main

import (
	"strings"
	"testing"
)

func TestDispatchNoArgumentsPrintsUsageAndFails(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim"}, dispatch)
	if !exited || code != 1 {
		t.Errorf("exit = (%d, %v), want a failure exit", code, exited)
	}
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "Commands:") {
		t.Errorf("no-argument run did not print usage:\n%s", out)
	}
}

func TestDispatchHelpForms(t *testing.T) {
	for _, args := range [][]string{
		{"claim", "--help"},
		{"claim", "-h"},
		{"claim", "help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, _, exited := runMain(t, args, dispatch)
			if exited {
				t.Error("an explicit help request should exit successfully")
			}
			if !strings.Contains(out, "Remove Claude as co-author") {
				t.Errorf("help output = %q", out)
			}
		})
	}
}

func TestDispatchPerCommandHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"repo --help", []string{"claim", "repo", "--help"}, "claim repo <owner/repo or URL>"},
		{"pr -h", []string{"claim", "pr", "-h"}, "claim pr <owner/repo#N or PR URL>"},
		{"help org", []string{"claim", "help", "org"}, "claim org <org-name or URL>"},
		{"help user", []string{"claim", "help", "user"}, "claim user <username or URL>"},
		{"report --help", []string{"claim", "report", "--help"}, "claim report [folder] [id|all]"},
		{"revert --help", []string{"claim", "revert", "--help"}, "claim revert <id>"},
		{"logout --help", []string{"claim", "logout", "--help"}, "claim logout [username]"},
		{"help after an argument", []string{"claim", "repo", "owner/repo", "--help"}, "claim repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, exited := runMain(t, tt.args, dispatch)
			if exited {
				t.Error("a help request should not exit with a failure")
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("help output is missing %q:\n%s", tt.want, out)
			}
		})
	}
}

func TestDispatchHelpForALocalFolder(t *testing.T) {
	out, _, exited := runMain(t, []string{"claim", ".", "--help"}, dispatch)
	if exited {
		t.Error("`claim . --help` should not fail")
	}
	if !strings.Contains(out, "Scan and clean local git repositories") {
		t.Errorf("`claim . --help` did not show the local help:\n%s", out)
	}
}

func TestDispatchHelpForAURL(t *testing.T) {
	out, _, exited := runMain(t, []string{"claim", "https://github.com/o/r", "--help"}, dispatch)
	if exited {
		t.Error("`claim <url> --help` should not fail")
	}
	if !strings.Contains(out, "Auto-detect") {
		t.Errorf("`claim <url> --help` did not show the URL help:\n%s", out)
	}
}

func TestDispatchHelpForAnUnknownSubcommandFallsBackToTheOverview(t *testing.T) {
	out, _, _ := runMain(t, []string{"claim", "help", "not-a-command"}, dispatch)
	if !strings.Contains(out, "Commands:") {
		t.Errorf("`claim help not-a-command` should fall back to the overview:\n%s", out)
	}
}

func TestDispatchVersion(t *testing.T) {
	for _, flag := range []string{"--version", "-v"} {
		out, _, exited := runMain(t, []string{"claim", flag}, dispatch)
		if exited {
			t.Errorf("%s should exit successfully", flag)
		}
		if !strings.Contains(out, "claim "+version) {
			t.Errorf("%s printed %q, want the version", flag, out)
		}
	}
}

func TestDispatchUnknownFlagIsRejected(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim", "--nonsense"}, dispatch)
	if !exited || code != 1 {
		t.Errorf("an unknown flag should fail, got exit (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "unknown flag") || !strings.Contains(out, "claim --help") {
		t.Errorf("the error should name the flag and point at help:\n%s", out)
	}
}

func TestDispatchFilterMsg(t *testing.T) {
	// __filter-msg is the internal git filter; it reads stdin and strips
	// Claude trailers. Exercised directly in TestRunFilterMsg.
	if _, ok := findCommand("__filter-msg"); ok {
		t.Error("__filter-msg is internal and should not appear in help")
	}
}

func TestDispatchRejectsAMalformedURL(t *testing.T) {
	_, code, exited := runMain(t, []string{"claim", "https://gitlab.com/o/r"}, dispatch)
	if !exited || code != 1 {
		t.Errorf("a non-github URL should fail, got exit (%d, %v)", code, exited)
	}
}

func TestDispatchLocalPathThatIsNotADirectory(t *testing.T) {
	out, code, exited := runMain(t, []string{"claim", "/definitely/not/a/real/path"}, dispatch)
	if !exited || code != 1 {
		t.Errorf("a missing directory should fail, got exit (%d, %v)", code, exited)
	}
	if !strings.Contains(out, "not a valid directory") {
		t.Errorf("error = %q", out)
	}
}
