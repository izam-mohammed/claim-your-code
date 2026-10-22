# the readme opens with a banner that says what claim does, in either theme

## The ask

Every repository should have a proper minimal README with either a banner
(a light and a dark SVG, made with the `github-banner` skill) or a UI
screenshot near the top.

Built so far: `docs/built/BUILT-2026-10-22-1826.md`.

What it means here: the README opens with a meme PNG
(`assets/who-wrote-the-code.png`, put there on purpose in commit
`0399d97`) and runs to 216 lines, most of it reference material (every
flag, the auth flow, reports, how the rewrite works). Neither a banner nor
a picture of the tool is there.

## Alternatives

### Decision 1: what opens the README? (two-way door)

Whatever sits at the top is the first thing a visitor sees; the ask wants a
banner or a screenshot.

Searched 2026-10-22: "terminal screenshot README charmbracelet vhs vs
freeze vs asciinema svg-term"; the freeze and VHS project descriptions,
asciinema's recording model; the `github-banner` skill's rules
(`<picture>` with `prefers-color-scheme`, no animation, self-contained).

| Must | Baseline: the meme PNG (user's pick, `0399d97`) | Light and dark banner SVGs | Terminal screenshot SVG of a scan |
| --- | --- | --- | --- |
| A banner or a screenshot of the tool | no (a meme about the tool, not either) | yes | yes |
| **Survives** | no | **yes** | **yes** |

| Criterion | Weight | Light and dark banner SVGs | Terminal screenshot SVG |
| --- | ---: | ---: | ---: |
| Performance efficiency | 40 | 5 | 4 |
| Fit to the ask | 35 | 5 | 4 |
| Reliability | 25 | 4 | 3 |
| **Weighted total** | **100** | **4.75** | **3.75** |

- Banner efficiency 5: two hand-written SVGs, a few KB each, no raster;
  the PNG today is a full-width bitmap.
- Banner fit 5: follows the viewer's GitHub theme through `<picture>`;
  a terminal capture has one background in both themes (fit 4).
- Screenshot reliability 3: it goes stale whenever the output wording
  changes and has to be regenerated (VHS or freeze); the banner says what
  the tool does, which does not change.

**Chosen: light and dark banner SVGs** (the meme, the user's earlier
pick, failed the must; the user is asked, see `## Open decisions`).
Revisit if the user wants a screenshot instead; the example output in
the README already shows a run.

### Decision 2: how much stays in the README? (two-way door)

Minimal means a visitor gets what it does, how to install it and the first
command without scrolling through every flag.

Searched 2026-10-22: "minimal README structure CLI tool install usage
docs folder"; GitHub's docs on READMEs (relative links to files in the
repository render on GitHub).

| Criterion | Baseline: all 216 lines | README short, details in `docs/usage.md` | README with `<details>` blocks |
| --- | --- | --- | --- |
| Performance efficiency | one long page | short page, one click for the rest | same bytes, folded |
| Fit to the ask | not minimal | minimal | minimal on screen, long in source |
| Maintainability of the result | one file | two files, each short | one file, HTML mixed in |

**Chosen: README short, details in `docs/usage.md`.** The README keeps
the pitch, install, the three commonest commands and a link. Revisit if
`docs/usage.md` stops being read: issues asking for a flag it documents.

## The decision

A banner pair made with the `github-banner` skill at
`assets/banner-dark.svg` and `assets/banner-light.svg`, shown at the top of
`README.md` with `<picture>` (decision 1), on a fresh concept checked
against the skill's `concepts-used.md` and logged there after. The README
is cut to the pitch, install, the commonest commands and a link to
`docs/usage.md`, which takes the flags, remote commands, reports,
authentication, what it matches and how it works, unchanged in substance
(decision 2). The README's own voice (plain, second person) is kept.

No new dependency or permission; the banner build script lives in the
skill, not in this repository.

Standards: `docs/standards.md` (`make check` once `plans/01-quality-gates.md`
lands).

Security: no new input, data, secret, dependency or permission; docs and
two static SVGs with no scripts or external URLs.

## Where it lands

| Path | New or edit | Phase | What changes | Firm? |
| --- | --- | --- | --- | --- |
| `assets/banner-dark.svg` | new | 1 | dark banner | fixed |
| `assets/banner-light.svg` | new | 1 | light banner | fixed |
| `README.md` | edit | 1 | `<picture>` at the top | reuse |
| `docs/usage.md` | new | 2 | the reference half of today's README | expected |

*Reused, not rewritten:*

- `README.md`: its install section and examples move as they are, no
  rewrite of what already works (PR `#10` made every command in it run).
- The `github-banner` skill's `scripts/build.py` and `scripts/preview.sh`:
  the build and lint of both SVGs.

## Open decisions

- The meme PNG was the user's pick for the top (`0399d97`) and fails the
  must: default, the banner goes on top and the meme moves to the foot of
  the README under its own line, unless the user says to drop it or keep
  it on top.

## Declined

- The meme PNG on top: failed the must (neither a banner nor a picture of
  the tool); pending the user's answer.
- Terminal screenshot SVG: lost on efficiency, fit across themes and going
  stale.
- One README with `<details>` blocks: still long in source and mixes HTML
  into the prose.

## What the tests hold

The skill's `build.py` lint holds the banner rules (no overlap, contrast at
least 4.5:1, no animation, no external URL). `make check` holds that no
code changed. Every link in the new README resolves on GitHub, checked by
hand in the try.

## Repository knowledge

Read root `KNOWLEDGE.md` when present before each phase. Follow the global and
repository instructions for recording useful development findings.

## Phases

### Phase 1: the rough cut: a banner on top (needs you: try it)

- [ ] Three concepts for what `claim` does, checked against the skill's
  `concepts-used.md`, the strongest built into
  `assets/banner-dark.svg` and `assets/banner-light.svg` by the skill's
  `build.py`, its lint passing.
- [ ] `README.md` opens with the `<picture>` pair and a one-line alt text
  saying the joke, the meme moved to the foot per `## Open decisions`.
- [ ] Check: `make check` passes, and `sh scripts/preview.sh` from the
  skill renders both themes with nothing overlapping.
- [ ] You try it: open the branch's README on GitHub, switch GitHub between
  light and dark, and say whether the banner reads and whether the meme
  stays, moves or goes.

### Phase 2: the minimal README

- [ ] Flags, remote examples, reports, authentication, what it matches and
  how it works move to `docs/usage.md` (expected), and `README.md` keeps
  the pitch, install, `claim .`, `claim repo owner/repo`, `--dry-run` and a
  link to `docs/usage.md`.
- [ ] The skill's `concepts-used.md` gains a line for this banner.
- [ ] Check: `make check` passes, and every relative link in `README.md`
  and `docs/usage.md` resolves on the pushed branch.

### Phase 3: open the PR

- [ ] The branch pushed and its PR opened against the branch it was cut from
  (or, if one is already open for this branch, its body brought up to date)
  with a Conventional Commit title for the whole change, a body naming this
  plan, what changed and why, and the checks run with their result. The PR's
  URL printed as the evidence. Opened, never merged.
