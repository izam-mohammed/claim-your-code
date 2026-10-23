# claim has a documentation site with a page for every command, generated from the help it prints

## The ask

A tool that needs detailed documentation gets it, planned in its own
repository.

Built so far: `docs/built/BUILT-2026-10-22-1826.md`.

ragrank is the model: a Sphinx site on Read the Docs with getting started,
guides and an API reference (`ragrank/docs/docs/`, `ragrank/docs/api_reference/`,
`.github/workflows/build-docs.yml`, which pings two Read the Docs webhooks on
a release).

What it means here: `claim` is a public CLI people install through
Homebrew, Scoop, `install.sh` or `go install`. `claim --help` lists nine
entries (`<folder>`, `<github-url>`, `repo`, `pr`, `org`, `user`,
`report`, `revert`, `logout`), the README's flag table has six flags, and
there is an authentication flow with three credential
sources, encrypted reports and a revert. Today all of it sits in a
216-line README. One README
cannot carry a page per command, and a hand-copied flag table drifts from
the binary. The binary already has the source of truth:
`cmd/claim/help.go` keeps every command, argument, flag and example in the
`commands` and `globalFlags` tables that `claim --help` prints. Detailed
docs are required: a CLI other people install, with many commands, flags
and an auth flow.

`plans/03-site.md` makes `site/index.html`, a one-page landing page on
GitHub Pages. The site is the landing page; this plan's docs are the
reference, served from the same Pages deploy under `/docs/`.

## Alternatives

### Decision 1: what builds the docs? (two-way door)

The pages are Markdown; moving to another generator later means a new
config and theme, not rewritten content. The docs need a sidebar, search,
light and dark themes, a way to include generated files, and static output
Pages can serve.

Searched 2026-10-22: "MkDocs Material vs Starlight vs Sphinx vs mdBook vs
Docusaurus 2026", "Material for MkDocs maintenance mode Zensical 2026",
"Zensical release status", "mdBook 0.5 release", "Hugo Hextra theme
FlexSearch hugo module", "mdBook vs Starlight vs Hugo Hextra page weight";
the Material for MkDocs announcement of maintenance mode (9.7.0,
11/11/2025, critical fixes for at least 12 months), Zensical's own docs
(alpha, 0.0.x versions), the mdBook 0.5 changelog, the Hextra theme page,
and ragrank's own Sphinx setup. No published like-for-like benchmark of
page weight or build time across these was found.

| Must | mdBook | Hugo with the Hextra theme | Starlight (Astro) | Sphinx with MyST (ragrank's setup) | Docusaurus | MkDocs Material | Zensical | pkg.go.dev |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Static output Pages can serve | yes | yes | yes | yes | yes | yes | yes | no (hosted by Google) |
| Documents commands for a user, not Go packages | yes | yes | yes | yes | yes | yes | yes | no (`main` and `internal/` only show as packages) |
| Still maintained, with a stable release | yes | yes | yes | yes | yes | no (maintenance mode since 11/2025) | no (alpha) | yes |
| **Survives** | **yes** | **yes** | **yes** | **yes** | **yes** | no | no | no |

| Criterion | Weight | mdBook | Hugo with Hextra | Starlight | Sphinx with MyST (ragrank's setup) | Docusaurus |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Performance efficiency | 35 | 4 | 4 | 4 | 3 | 2 |
| Fit to the ask | 25 | 4 | 5 | 5 | 4 | 5 |
| Security | 15 | 5 | 3 | 2 | 3 | 2 |
| Maintainability of the result | 15 | 5 | 3 | 3 | 3 | 2 |
| Compatibility | 10 | 4 | 5 | 2 | 2 | 2 |
| **Weighted total** | **100** | **4.30** | **4.05** | **3.60** | **3.15** | **2.75** |

- Efficiency: mdBook, Hugo and Starlight all write static HTML from one
  build with no runtime; each page still loads a search script (mdBook's
  built-in index, Hextra's FlexSearch, Starlight's Pagefind), so none is
  scored 5. Sphinx 3: a Python build and a theme's scripts. Docusaurus 2:
  a React app that hydrates on every page. No byte counts were found to
  separate the first three; Phase 1 records the built size of the first
  page as the baseline.
- Fit: Hextra, Starlight and Docusaurus have richer components (tabs,
  callouts); mdBook has the sidebar, search, themes and
  `{{#include}}` for generated files, which is all the ask needs (4).
  Sphinx's strength is Python autodoc, which a Go CLI cannot use (4).
- Security: mdBook is one release binary pinned by version and checksum,
  with no package tree (5). Hugo adds a third-party theme fetched as a
  module (3). Starlight and Docusaurus add Node and an npm tree (2). Sphinx
  adds a pip tree (3).
- Maintainability: mdBook is `book.toml` and `SUMMARY.md` (5); the others
  add a theme pin, a lockfile or a `conf.py` (3, Docusaurus 2 for its
  React config).
- Compatibility: Hugo is a Go binary like the repository's other tools
  (5); mdBook is a pinned binary like gitleaks (4); Node or Python in a Go
  repository (2).

**Chosen: mdBook.** It meets the ask with the fewest moving parts and the
smallest dependency surface. Sphinx, ragrank's setup, scored lower here
because its main gain (Python API autodoc) does not apply to a Go CLI; see
`## Open decisions`. Revisit if the docs need versioned pages per release,
which mdBook does not do.

### Decision 2: where are the docs served? (two-way door)

They have to be reachable at a URL, built on `main`, with no person
deploying them, and they should sit next to the landing page from
`plans/03-site.md`.

Searched 2026-10-22: "Read the Docs vs GitHub Pages hosting project
documentation private repository free plan 2026", "GitHub Pages
availability private repositories GitHub Free"; the Read the Docs pricing
page (Community is free for open source and ad-supported; private docs
start at a paid plan), GitHub's Pages limits page (free for public
repositories), and `plans/03-site.md` decision 1.

| Must | Baseline: GitHub Pages, the `pages.yml` from `plans/03-site.md`, under `/docs/` | Read the Docs Community (ragrank's setup) | Cloudflare Pages | Vercel |
| --- | --- | --- | --- | --- |
| No new account, service or secret | yes | no (a Read the Docs account; ragrank keeps webhook secrets) | no (account and API token) | no (account and token) |
| No ads on the docs | yes | no (Community is ad-supported) | yes | yes |
| **Survives** | **yes** | no | no | no |

Only the baseline survives, so there is nothing to score against it.

**Chosen: GitHub Pages, from the same `pages.yml`, at
`https://izam-mohammed.github.io/claim-your-code/docs/`.** The landing page
stays at the root and links to `/docs/`; one deploy publishes both. Revisit
if the docs need versions per release or pull request previews.

### Decision 3: how do the command pages stay true to the binary? (two-way door)

A page per command must not drift from `claim <command> --help`.

Searched 2026-10-22: "generate CLI reference markdown from help text go
golden file test -update"; the repository's own `cmd/claim/help.go` and
`help_test.go`, and penpot-mcp's `TestSkillListsEveryTool`, which writes a
Markdown file from the server with `-update` and fails when it differs.

| Criterion | Baseline: hand-written pages | A Go test writes the pages from `commands`, fails when stale | A script runs the built `claim <cmd> --help` at docs build time |
| --- | --- | --- | --- |
| Performance efficiency | nothing to run | one test in `go test` | a Go build before every docs build |
| Fit to the ask | drifts | cannot drift; `make check` fails | cannot drift, but only plain help text |
| Reliability | silent drift | the diff shows in review | output changes only seen after deploy |

**Chosen: a Go test writes the pages.** `TestDocsMatchHelp` (expected in
`cmd/claim/docs_test.go`) renders each `commandDoc` and `globalFlags` to
`docs/book/src/reference/<command>.md`; with `-update` it writes them,
without it it fails on any difference. Revisit if commands move to a
library that generates its own docs.

### Decision 4: what checks the built docs? (two-way door)

A broken link or a missing chapter should fail `make check`, offline.

Searched 2026-10-22: "lychee link checker offline mode local html vs
mdbook-linkcheck mdbook 0.5 compatibility"; lychee's docs (`--offline`
checks local files and HTML), mdBook's `build.create-missing` option. No
statement was found on whether `mdbook-linkcheck` supports mdBook 0.5.

| Criterion | Baseline: `mdbook build` only | `mdbook-linkcheck` backend | lychee `--offline` on the built HTML |
| --- | --- | --- | --- |
| Performance efficiency | fastest | runs inside the build | one extra pass over local files, no network |
| Fit to the ask | missing chapters only | links and anchors | links, local files; anchors to be confirmed in Phase 1 |
| Reliability | misses broken links | `?` on mdBook 0.5 | independent of the generator |

**Chosen: `mdbook build` with `create-missing = false`, any warning
treated as a failure, then lychee `--offline` on the output.** Revisit if
lychee misses broken anchors in Phase 1's planted-link test.

## The decision

The docs are an mdBook book in `docs/book/` (`book.toml`,
`src/SUMMARY.md`, decision 1), built into `site/docs/` by the Pages
workflow from `plans/03-site.md` and published at `/docs/` beside the
landing page (decision 2). The command reference is generated from
`cmd/claim/help.go` by `TestDocsMatchHelp` (decision 3). `make docs` builds
the book and runs lychee offline; `make check` runs it (decision 4). The
book takes over the README's usage sections: they become the
getting-started and guide chapters, and the README keeps the pitch and
install with a link to the published docs, so there is one copy.

Chapters: getting started (install on each platform, first local scan,
first remote scan), guides (scanning many repositories, authentication and
saved accounts, reports and revert, force-pushing safely, what it
matches), the generated command reference, and troubleshooting.

New dependencies: `mdbook` and `lychee`, each a release binary pinned by
version and SHA-256 in a `make tools` target (new, or the one
`plans/01-quality-gates.md` adds) and in CI. No new
secret or permission; the Pages permissions are `plans/03-site.md`'s.

Standards: `docs/standards.md` (`make check` once
`plans/01-quality-gates.md` lands); this plan adds `make docs` to it.

## Where it lands

| Path | New or edit | Phase | What changes | Firm? |
| --- | --- | --- | --- | --- |
| `docs/book/book.toml`, `docs/book/src/SUMMARY.md` | new | 1 | the book | expected |
| `docs/book/src/getting-started.md` | new | 1 | install and first scan | expected |
| `docs/book/src/reference/*.md` | new | 2 | generated command pages | fixed |
| `cmd/claim/docs_test.go` | new | 2 | `TestDocsMatchHelp` | expected |
| `docs/book/src/guides/*.md` | new | 3 | guides, from the README's usage sections | expected |
| `README.md` | edit | 3 | usage sections moved into the book, a link left | expected |
| `Makefile` | edit | 1, 4 | `make docs`, tools, in `make check` | reuse |
| `.github/workflows/pages.yml` | edit | 4 | builds the book into `site/docs/` (the file `plans/03-site.md` lands) | expected |
| `site/index.html`, `README.md` | edit | 4 | link to `/docs/` (the page `plans/03-site.md` lands) | expected |
| `docs/standards.md` | edit | 4 | the docs check | reuse |

*Reused, not rewritten:*

- `cmd/claim/help.go` `commands`, `globalFlags`, `commandDoc`: the source
  of every reference page.
- The README's usage sections: the guide text, split into chapters, not
  rewritten.
- `.github/workflows/pages.yml` from `plans/03-site.md`: one deploy for
  both.

## Security

The docs are static and public. The risk is the two new binaries in the
build and the deploy path.

| Threat | Here | Handled by | Proved by |
| --- | --- | --- | --- |
| Tampering: a replaced `mdbook` or `lychee` download | the `tools` target and CI | pinned version and SHA-256, the build stops on a mismatch | a wrong checksum in a scratch copy fails `make tools` |
| Elevation of privilege: a PR deploys the docs | `pages.yml` | `plans/03-site.md`'s triggers and job permissions, unchanged | the PR shows no Pages run |
| Information disclosure: a token in an example | guide pages | examples use `<token>` placeholders; gitleaks in `make check` | gitleaks clean |
| Information disclosure: a third-party request from a page | theme assets | mdBook's theme is served from the same origin; no analytics | the network panel in the try shows only the Pages host |

No data collected, no cookies. The checklist followed: GitHub's Pages
security notes for custom workflows and the shared gate's workflow rules.

## Open decisions

- ragrank's stack (Sphinx on Read the Docs) was the model named in the
  ask and lost here (decisions 1 and 2): default, mdBook on GitHub Pages,
  unless the user wants ragrank's stack kept for consistency.

## Declined

- MkDocs Material: failed the must (maintenance mode since 11/2025).
- Zensical: failed the must (alpha).
- pkg.go.dev: failed the musts (Google-hosted; documents packages, not
  commands).
- Hugo with Hextra: lost on security and maintainability (a third-party
  theme fetched each build).
- Starlight, Docusaurus: lost on security and compatibility (Node and an
  npm tree in a Go repository); Docusaurus also on efficiency.
- Sphinx with MyST: lost on efficiency and compatibility; its autodoc
  does not apply to Go.
- Read the Docs, Cloudflare Pages, Vercel: failed the musts (a new
  account; Read the Docs Community shows ads).
- Hand-written command pages: drift from the binary.
- `mdbook-linkcheck`: support for mdBook 0.5 unconfirmed.

## What the tests hold

`TestDocsMatchHelp` holds that every command and flag `claim --help` knows
has a page and that each page says what the help says. `make docs` holds
that the book builds without warnings and every local link resolves.
Together they cover drift and breakage; the prose of the guides is checked
by reading in the try.

## Repository knowledge

Read root `KNOWLEDGE.md` when present before each phase. Follow the global and
repository instructions for recording useful development findings.

## Phases

### Phase 1: the rough cut: the book builds locally (needs you: try it)

- [ ] `docs/book/` with `book.toml` (`create-missing = false`),
  `SUMMARY.md`, an introduction and `getting-started.md` (install on each
  platform, `claim .`, `claim repo owner/repo`), copied from the README's
  install and usage text.
- [ ] `make docs` builds the book with a pinned `mdbook`, failing on any
  warning, and `make tools` installs it by version and checksum; the
  built size of `getting-started.html` and its assets recorded in the
  phase's commit message as the baseline.
- [ ] Check: `make check` passes, and `make docs` writes
  `docs/book/book/index.html`.
- [ ] You try it: run `make docs` and `mdbook serve docs/book`, open the
  getting-started page in light and dark, search for "revert", and say
  whether this is the shape you want before the rest is written.

### Phase 2: the command reference, generated

- [ ] `TestDocsMatchHelp` (expected in `cmd/claim/docs_test.go`) writes
  `docs/book/src/reference/<command>.md` and a global flags page from
  `commands` and `globalFlags` with `-update`, and fails when a page
  differs; `SUMMARY.md` lists them.
- [ ] Check: `make check` passes, and adding a fake flag to `help.go` in a
  scratch copy fails `go test ./cmd/claim` until `-update` is run.

### Phase 3: guides and examples

- [ ] Guides for many repositories (`org`, `user`, API or clone),
  authentication and saved accounts, reports and `revert`, pushing after a
  clean, and what it matches, moved from the README's usage sections into
  `docs/book/src/guides/`, and those sections shrink to a link to the book.
- [ ] A troubleshooting page from the errors `claim` prints today, each
  with what to do, and every example output copied from a real run.
- [ ] Check: `make check` passes, and `make docs` builds with no warning.

### Phase 4: the docs check and publishing

- [ ] `make docs` also runs a pinned lychee `--offline` on the built book,
  pinned by checksum, and `make check` runs `make docs`; a planted broken
  link and a planted broken anchor each fail it.
- [ ] `pages.yml` builds the book into `site/docs/` before uploading
  `site/`, so one deploy serves both; `site/index.html` and `README.md`
  link `/docs/`.
- [ ] `docs/standards.md` names `make docs`, mdBook and lychee under
  checks and decided.
- [ ] Check: `make check` passes, and after the workflow runs on `main`,
  `https://izam-mohammed.github.io/claim-your-code/docs/` answers 200.

### Phase 5: open the PR

- [ ] The branch pushed and its PR opened against the branch it was cut from
  (or, if one is already open for this branch, bring the open PR's body up
  to date) with a Conventional Commit title for the whole change, a body
  naming this plan, what changed and why, and the checks run with their
  result. The PR's URL printed as the evidence. Opened, never merged.
