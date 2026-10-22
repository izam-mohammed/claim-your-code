# Security policy

## Supported versions

Only the latest release of `claim` on
[GitHub Releases](https://github.com/izam-mohammed/claim-your-code/releases)
gets security fixes. Please upgrade (`brew upgrade claim`, `scoop update claim`,
the install script, or `go install ...@latest`) before reporting.

| Version | Supported |
|---|---|
| latest release | yes |
| older | no |

## Reporting a vulnerability

Please do not open a public issue, pull request or discussion for a security problem.

Report it privately through GitHub:
[Report a vulnerability](https://github.com/izam-mohammed/claim-your-code/security/advisories/new)
(the "Security" tab, then "Report a vulnerability").

Helpful details:

- the version and how you installed it (Homebrew, Scoop, install script, binary, `go install`)
- what an attacker could do, and what they need first (for example a crafted repository or a malicious API response)
- the smallest steps or script that show it
- any fix you have in mind

Please leave out real tokens and private repository contents.

## What happens next

This is a project with one maintainer, so there are no guaranteed response times. The aim is to:

- acknowledge the report
- agree on whether it is a vulnerability and how serious it is
- prepare a fix, release it, and publish a GitHub security advisory that credits you (unless you would rather
  not be named)

Please give a reasonable chance to release a fix before you share details publicly.

## Scope

In scope: the `claim` CLI (local scanning, history rewriting, remote clone and force-push), its GitHub sign-in
and token cache, `install.sh`, and this repository's release workflow.
Problems in git, go-git or GitHub itself should go to those projects; tell us too if `claim` makes them worse.
