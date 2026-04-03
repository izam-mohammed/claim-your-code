# claim

Strip `Co-Authored-By: Claude` lines from your git commit history.

When you use Claude Code (or similar AI tools), commits get tagged with co-author lines like:

```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

`claim` removes these lines so you can claim your code as your own.

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

```sh
claim <folder>
```

### Example

```
$ claim ./my-project

Scanning commits in './my-project'...

Found 12 commit(s) with Co-Authored-By: Claude lines:

  a1b2c3d4  Fix authentication bug
  e5f6g7h8  Add user profile page
  i9j0k1l2  Refactor database queries
  ... and 9 more

This will rewrite git history for 12 commit(s).
Proceed? [y/N] y

Rewriting commit messages...

Done! Cleaned 12 commit(s).

Note: If you have already pushed these commits, you will need to force-push:
  git push --force-with-lease
```

### Flags

| Flag | Description |
|---|---|
| `--dry-run` | Show affected commits without modifying anything |
| `--force`, `-f` | Skip confirmation prompt |
| `--version`, `-v` | Show version |
| `--help`, `-h` | Show help |

## Updating GitHub remote

After running `claim`, your local history has been rewritten — but your GitHub remote still has the old commits. You need to force-push to update it:

```sh
# Push all rewritten branches
git push --force-with-lease --all

# If you also have tags pointing to rewritten commits
git push --force-with-lease --tags
```

> **Warning:** Force-pushing rewrites history for anyone who has cloned or forked your repo. If others are collaborating, coordinate with them first — they will need to re-clone or run `git pull --rebase` after the push.

For a single branch:

```sh
git push --force-with-lease origin main
```

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

## License

MIT
