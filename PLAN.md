# claim: what is left to build, in order

The tool itself is built and released (`docs/built/BUILT-2026-10-22-1826.md`).
What is left is the repository standards: the checks first, so the README
and site changes after them run through the same gate.

### Phase 1: one check on every pull request

- [ ] `plans/01-quality-gates.md` - `make check` with lint, tests, a 90 %
  coverage floor and the security tools, run on every PR.

### Phase 2: say what it is

- [ ] `plans/02-readme-banner.md` - a light and dark banner on top, and a
  minimal README.
- [ ] `plans/03-site.md` - a one-page site on GitHub Pages.
