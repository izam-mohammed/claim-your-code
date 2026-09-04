// Command claim removes Claude co-authorship from git commit history,
// locally or on GitHub.

package main

import (
	"fmt"
	"os"

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

	switch os.Args[1] {
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
	case "--help", "-h":
		printUsage()
	default:
		arg := os.Args[1]
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

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s - Remove Claude as co-author from git commits

%s
  claim <folder>                Scan and clean local git repositories
  claim <github-url>            Auto-detect repo or PR from URL

%s
  claim repo <owner/repo>       Scan a remote GitHub repo
  claim pr <owner/repo#N>       Scan a pull request
  claim org <name>              Scan all repos in an organization
  claim user <name>             Scan all repos of a user

%s
  claim report [id|all]         List or show claim reports
  claim revert <id>             Revert a specific clean
  claim logout [username]        Remove saved GitHub account(s)

%s
  --dry-run          Show what would be changed without modifying anything
  --force            Skip confirmation prompt
  --apply            For remote repos, rewrite and force-push (default: scan only)
  --api-only         Scan via GitHub API without cloning (faster, may miss old commits)
`, bold("claim"), bold("Local:"), bold("Remote:"), bold("Reports:"), bold("Flags:"))
}
