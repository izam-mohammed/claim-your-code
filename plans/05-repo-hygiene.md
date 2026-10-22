# the repo keeps its dependencies current and tells people how to report a vulnerability

## The ask

The owner asked, in their own words: "have dependabot in all of my repos",
"have proper security.md file for all of my repos", and "proper licence for
all of the repos ... which is a product in nature ... even for private ones".

In this repository that means: Dependabot already covers `gomod` and
`github-actions`, but not the `pre-commit` hooks in
`.pre-commit-config.yaml`; there is no `SECURITY.md`; the MIT `LICENSE` and
the README's `## License` line already exist and stay as they are. The
repository had no GitHub topics.

No built snapshot is written: this change touches only repository metadata
(Dependabot config, a policy file, topics), not the `claim` CLI.

## Alternatives

### Decision 1: which bot keeps dependencies current (two-way door)

The owner named Dependabot. The repository already runs it, and open
Dependabot PRs exist, so the baseline is Dependabot.

Searched 22/10/2026: "Renovate vs Dependabot GitHub app private
repositories minimumReleaseAge", "dependabot pre-commit ecosystem"; read
the Dependabot options reference (docs.github.com), the 10/03/2026 GitHub
changelog on pre-commit support, and Renovate's minimum release age docs
(docs.renovatebot.com).

| Must | Baseline: Dependabot (user's pick) | Renovate |
| --- | --- | --- |
| Updates Go modules, Actions and pre-commit hooks | yes | yes |
| Runs without a third-party app on the account | yes | no (needs the Mend app or a self-hosted runner) |
| **Survives** | **yes** | no |

| Criterion | Weight | Baseline: Dependabot |
| --- | ---: | ---: |
| Performance efficiency | 40 | 4 |
| Reliability | 30 | 4 |
| Compatibility | 30 | 5 |
| **Weighted total** | **100** | **4.30** |

- Dependabot efficiency 4: runs on GitHub's side with no CI minutes; a
  weekly schedule and one group per ecosystem keep it to a few PRs a week.
- Dependabot compatibility 5: already configured here, and `cooldown`
  `default-days` is documented for every ecosystem in use.

Sensitivity: only a need for a feature Dependabot lacks (cross-ecosystem
grouping) would reopen this.
Risk: a bad release is proposed; the 7-day cooldown limits it.

**Chosen: Dependabot (user's pick).** It is already in place and needs no
outside app. Revisit if grouping across ecosystems becomes necessary.

## The decision

Topics are set as a repository setting. `.github/dependabot.yml` keeps its
`gomod` and `github-actions` entries untouched and gains a `pre-commit`
entry modelled on the owner's standard (weekly, 7-day cooldown, one group,
`chore(deps)` prefix) (Decision 1). A new `SECURITY.md` points reporters to
GitHub private vulnerability reporting (enabled on this public repository),
supports only the latest release, and scopes the CLI, its token cache and
the release workflow. No licence change: MIT is already present.

New dependencies or permissions: none.

Security: the change adds no code, input or secret; it adds the policy that
tells reporters how to reach the owner privately.

## Where it lands

| Path | New or edit | Phase | What changes | Firm? |
| --- | --- | --- | --- | --- |
| `.github/dependabot.yml` | edit | 2 | adds the `pre-commit` ecosystem | fixed |
| `SECURITY.md` | new | 3 | the security policy | fixed |

*Reused, not rewritten:*

- `.github/dependabot.yml`: the existing `gomod` and `github-actions`
  entries stay as they are; only the missing ecosystem is added.

## Declined

- Renovate: fails the must of running without a third-party app.
- Rewriting the existing Dependabot entries to the newer standard: the
  brief is to extend only where an ecosystem is missing.

## Phases

### Phase 1: topics

- [ ] Topics set to `go`, `cli`, `git`, `git-history`, `commit-rewrite`,
  `claude-code`, `github`; check: `gh repo view --json repositoryTopics`
  lists them.

### Phase 2: Dependabot

- [ ] The `pre-commit` ecosystem added to `.github/dependabot.yml` (weekly,
  7-day cooldown, one group, `chore(deps)`); check: the YAML parses.

### Phase 3: security policy

- [ ] `SECURITY.md` added: supported versions, private reporting link,
  what happens next, scope; check: no email address or response time is
  invented in it.

### Phase 4: open the PR

- [ ] The branch pushed and its PR opened against `main` (or, if one is
  already open for this branch, its body brought up to date) with a
  Conventional Commit title for the whole change, a body naming this plan,
  what changed and why, and the checks run with their result. The PR's URL
  printed as the evidence. Opened, never merged.
