# claim

Remove Claude as a co-author from your git commit history.

When you use Claude Code (or similar AI tools), commits are automatically co-authored with Claude:

```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

`claim` finds and removes Claude's co-authorship so you can fully own your commits.

## Install

### Homebrew (macOS / Linux)

```sh
brew tap izam-mohammed/tap
brew install claim
```

### Scoop (Windows)

```sh
scoop bucket add izam-mohammed https://github.com/izam-mohammed/scoop-bucket
scoop install claim
```

### Shell script (macOS / Linux)

```sh
curl -fsSL https://raw.githubusercontent.com/izam-mohammed/claim-your-code/main/install.sh | sh
```

### Download binary

Grab the latest binary for your OS from [GitHub Releases](https://github.com/izam-mohammed/claim-your-code/releases).

### Build from source

```sh
go install github.com/izam-mohammed/claim-your-code/cmd/claim@latest
```

## Usage

### Local repositories

```sh
claim <folder>
claim .
```

### Remote GitHub repositories

```sh
# Paste any GitHub URL directly
claim https://github.com/owner/repo
claim https://github.com/owner/repo/pull/42
claim git@github.com:owner/repo.git

# Or use explicit subcommands
claim repo owner/repo
claim pr owner/repo#42
claim org my-organization
claim user some-username
```

Remote commands default to **scan-only**. Add `--apply` to rewrite and force-push.

### Example (local)

```
$ claim ./my-project

:: Scanning commits in my-project

! Found 12 co-authored commit(s) across 45 total commits

Branches:
  → main  (10 commit(s))
      Claude Opus 4.6  × 7
      Claude Sonnet 4.5  × 3
  → feature  (2 commit(s))
      Claude Opus 4.6  × 2

Commits:
  a1b2c3d4 Fix authentication bug (Claude Opus 4.6)
  e5f6g7h8 Add user profile page (Claude Sonnet 4.5)
  ... and 10 more

⚠ This will rewrite git history for 12 commit(s).
  Proceed? [y/N] y

:: Rewriting commit messages...

✓ Cleaned 12 commit(s)

Note: If you've already pushed, force-push to update remote:
  git push --force-with-lease
```

### Example (remote)

```
$ claim repo izam-mohammed/my-project --apply

:: Authenticating with GitHub...
  ✓ Authenticated as izam-mohammed

:: Fetching repo info for izam-mohammed/my-project
:: Cloning izam-mohammed/my-project...
:: Scanning commits...

! Found 5 co-authored commit(s) across 20 total commits

⚠ This will rewrite and force-push izam-mohammed/my-project to main
  Proceed? [y/N] y

:: Rewriting commit messages...
:: Pushing to izam-mohammed/my-project...

✓ Cleaned and pushed 5 commit(s) in izam-mohammed/my-project
```

### Flags

| Flag | Description |
|---|---|
| `--dry-run` | Show affected commits without modifying anything |
| `--force`, `-f` | Skip confirmation prompt |
| `--apply` | For remote repos, rewrite and force-push (default: scan only) |
| `--api-only` | Scan via GitHub API without cloning (faster, may miss old commits) |
| `--version`, `-v` | Show version |
| `--help`, `-h` | Show help |

### Reports

Every scan is tracked. View reports with:

```sh
claim report              # list all reports
claim report <id>         # show details
claim report all          # show all in detail
claim revert <id>         # revert a clean
```

## Authentication

For remote commands, `claim` needs a GitHub token. It tries these in order:

1. **Cached token** from a previous session
2. **`CLAIM_GITHUB_TOKEN`** environment variable
3. **`GITHUB_TOKEN`** environment variable
4. **`gh` CLI** — extracts token from `gh auth token`
5. **OAuth Device Flow** — opens browser for authorization
6. **Interactive prompt** — paste a Personal Access Token

Tokens are cached securely in your OS data directory.

## What it matches

Any `Co-Authored-By` line referencing Claude models at `@anthropic.com`:

- `Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>`
- `Co-Authored-By: Claude Sonnet 4.5 (1M context) <noreply@anthropic.com>`
- `Co-Authored-By: Claude Haiku 3.5 <noreply@anthropic.com>`
- Any future Claude model variant

Non-Claude co-author lines are preserved.

## How it works

1. Scans all commits using [go-git](https://github.com/go-git/go-git) (no git dependency for scanning)
2. Shows you the affected commits and asks for confirmation
3. Rewrites history using `git filter-branch` (requires git installed)
4. Cleans up backup refs automatically
5. For remote repos: clones to temp dir, rewrites, force-pushes, cleans up

## License

MIT
