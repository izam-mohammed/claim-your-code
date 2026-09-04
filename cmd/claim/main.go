// Command claim removes Claude co-authorship from git commit history,
// locally or on GitHub.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/izam-mohammed/claim-your-code/internal/remote"
)

var version = "dev"

func main() { dispatch() }

// dispatch routes os.Args to the matching command.
func dispatch() {
	if len(os.Args) < 2 {
		printUsage()
		exit(1)
		return
	}

	cmd := os.Args[1]

	// `claim help [command]` and `claim <command> --help`.
	if isHelpFlag(cmd) {
		if len(os.Args) >= 3 && showCommandHelp(os.Args[2]) {
			return
		}
		showHelp()
		return
	}
	if wantsHelp(os.Args[2:]) {
		if showCommandHelp(cmd) {
			return
		}
		// Not a subcommand, so it is a folder or a URL.
		if remote.IsURL(cmd) {
			showCommandHelp("<github-url>")
		} else {
			showCommandHelp("<folder>")
		}
		return
	}

	switch cmd {
	case "__filter-msg":
		runFilterMsg()
	case "repo":
		runRemoteRepo()
	case "pr":
		runRemotePR()
	case "org":
		runRemoteOrg()
	case "user":
		runRemoteUser()
	case "logout":
		runLogout()
	case "report":
		runReport()
	case "revert":
		runRevert()
	case "--version", "-v":
		fmt.Printf("claim %s\n", version)
	default:
		arg := cmd
		if strings.HasPrefix(arg, "-") {
			fatalf("unknown flag %q — run `claim --help` to see the available commands", arg)
			return
		}
		if remote.IsURL(arg) {
			target, err := remote.ParseURL(arg)
			if err != nil {
				fatal(err)
			}
			switch target.Kind {
			case "pr":
				runRemotePRWithTarget(target)
			case "user":
				runRemoteMultiRepoByName(target.Owner)
			default:
				runRemoteRepoWithTarget(target)
			}
		} else {
			runClaim(arg)
		}
	}
}
