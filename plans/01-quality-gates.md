# every pull request runs one check, with a 90 % coverage floor

## The ask

Every repository should have proper lint and format, tests, a test
coverage floor of 90 % (target 95 %) and security checks (dependency audit,
secret scan, static analysis), all run by one local check command and by CI
on every pull request, reusing the shared security gate in
`izam-mohammed/.github` rather than a parallel one.

Built so far: `docs/built/BUILT-2026-10-22-1826.md`.

What it means here: the tests and the coverage are already good (96.1 % of
statements on 22/10/2026, every package passing), and the shared gate
already runs on every PR. What is missing is the one command that runs
everything, a floor that stops coverage dropping, a linter past `go vet`,
and any test run on a pull request: `ci.yml` was made manual-only on
05/09/2026 (commit `34b4750`) because the three-OS matrix fired on every
push.

## Alternatives

### Decision 1: which linter runs past `go vet`? (two-way door)

`go vet` catches a narrow set; errors left unchecked, shadowing and unused
code go unseen.

Searched 2026-10-22: "golangci-lint v2 vs staticcheck vs go vet
comparison"; the golangci-lint README (default linters govet, errcheck,
staticcheck, unused), the staticcheck description, and
`izam-mohammed/penpot-mcp`'s `.golangci.yml`, which already runs v2.

| Criterion | Weight | Baseline: `go vet` | golangci-lint v2 | staticcheck alone |
| --- | ---: | ---: | ---: | ---: |
| Performance efficiency | 40 | 5 | 4 | 4 |
| Fit to the ask | 30 | 2 | 5 | 4 |
| Compatibility | 30 | 4 | 5 | 3 |
| **Weighted total** | **100** | **3.80** | **4.60** | **3.70** |

- `go vet` efficiency 5: part of the toolchain, no extra binary; fit 2: no
  errcheck, no unused, no formatter check.
- golangci-lint efficiency 4: one process for every linter with a shared
  cache, but one more binary to install; fit 5: lint and `fmt --diff` in one
  config; compatibility 5: the same tool and pinned version
  (`GOLANGCI_LINT_VERSION := v2.14.0`) as penpot-mcp's `Makefile`.
- staticcheck compatibility 3: a second config style beside penpot-mcp's.

**Chosen: golangci-lint v2.** One config for lint and format, matching
the other Go repository. Revisit if a full run passes 30 s.

### Decision 2: how is the coverage floor held? (two-way door)

The floor must fail `make check` locally and in CI, without sending code
or coverage anywhere.

Searched 2026-10-22: "go test coverage threshold enforce CI
go-test-coverage vladopajic vs codecov"; the go-test-coverage project page
(server-less, GPL-3.0, per-package thresholds), Codecov's Go guide (needs an
account and org approval).

| Must | `go tool cover` total, read by the Makefile | go-test-coverage | Codecov |
| --- | --- | --- | --- |
| Runs locally with no account | yes | yes | no (hosted service) |
| **Survives** | **yes** | **yes** | no |

| Criterion | Weight | `go tool cover` total | go-test-coverage |
| --- | ---: | ---: | ---: |
| Performance efficiency | 40 | 5 | 4 |
| Reliability | 30 | 4 | 5 |
| Security | 30 | 5 | 4 |
| **Weighted total** | **100** | **4.70** | **4.30** |

- `go tool cover` efficiency 5: reads the profile `go test` already wrote;
  reliability 4: one total, so one weak package can hide behind strong ones
  (the per-package table is printed so it is seen).
- go-test-coverage reliability 5: per-file and per-package thresholds;
  security 4: one more dependency in the toolchain.

**Chosen: `go tool cover` total, read by the Makefile.** No new tool, and
the floor lives in one file. Revisit if a package falls under 85 % while
the total holds.

### Decision 3: what runs on a pull request? (two-way door)

The ask wants the check on every PR; the user turned the three-OS matrix
off for PRs on 05/09/2026.

Searched 2026-10-22: "github actions run matrix only on workflow_dispatch
and single job on pull_request"; the GitHub Actions docs on `on:` events
and matrix jobs.

| Must | Baseline: manual only | One ubuntu `make check` per PR, matrix kept manual | Full matrix on every PR |
| --- | --- | --- | --- |
| Tests run on every PR | no | yes | yes |
| **Survives** | no | **yes** | **yes** |

| Criterion | Weight | Ubuntu check per PR | Full matrix per PR |
| --- | ---: | ---: | ---: |
| Performance efficiency | 40 | 5 | 2 |
| Reliability | 30 | 3 | 5 |
| Compatibility | 30 | 5 | 2 |
| **Weighted total** | **100** | **4.40** | **2.90** |

- Ubuntu efficiency 5: one runner per PR instead of three, and macos
  minutes are the dearest; compatibility 5: keeps the user's decision in
  `34b4750` for the matrix.
- Ubuntu reliability 3: a windows-only bug (as fixed in `3c48b89`) is
  caught only when the matrix is run by hand or by the release.

**Chosen: one ubuntu `make check` per PR, the matrix kept manual.**
Revisit if a windows or macos bug reaches a release.

### Decision 4: where do the security checks live? (two-way door)

The shared gate already runs on every PR here (`security.yml`, pinned to
`ff76c48`).

Searched 2026-10-22: "GitHub reusable workflow security scan gitleaks
osv-scanner semgrep zizmor vs CodeQL default setup"; the shared gate's
`README.md`, a Cloud Foundry Stratos PR adopting the same tools and
deferring CodeQL.

| Criterion | Baseline: the shared gate, plus govulncheck and gitleaks in `make check` | CodeQL default setup added | A repository-local scan workflow |
| --- | --- | --- | --- |
| Performance efficiency | four parallel jobs, minutes | a CodeQL build per PR, slower | same tools, duplicated |
| Fit to the ask | secrets, deps, SAST, workflows | SAST only | as baseline |
| Compatibility | the gate every repo shares | a second SAST beside semgrep | a parallel gate, which the ask rules out |

**Chosen: the shared gate, plus govulncheck and gitleaks in `make
check`.** It is already wired; the local half lets a change fail before it
is pushed. Revisit if the gate gains a blocking SAST stage.

## The decision

`make check` runs, in order and stopping at the first failure: golangci-lint
format check and lint (decision 1), `go test -race` writing a coverage
profile, the coverage floor from `scripts/coverage-floor` (decision 2, 90 %),
the build, govulncheck and gitleaks (decision 4). `.github/workflows/check.yml`
runs it on ubuntu for every pull request and push to `main` (decision 3);
`ci.yml` stays manual for the three-OS matrix. The floor starts at 90 %, the
ask's floor, with 95 % as the target to raise it to once the lowest package
(`internal/crypt`, 85.1 %) is lifted. `docs/standards.md` is updated in
the phase that changes each check.

New dependencies: golangci-lint v2.14.0 and govulncheck as `go install`
tools (`make tools`), gitleaks v8.30.1 (already the pre-commit hook's
version). No runtime dependency, no new permission beyond
`contents: read` for the new workflow.

Standards: `docs/standards.md` (format, lint, tests with the coverage
floor, build, govulncheck and gitleaks through `make check` once Phase 1
lands; today `make test` and `make lint`).

## Where it lands

| Path | New or edit | Phase | What changes | Firm? |
| --- | --- | --- | --- | --- |
| `Makefile` | edit | 1 | `check`, `cover`, `tools` targets | expected |
| `scripts/coverage-floor` | new | 1 | the floor, one number | fixed |
| `.github/workflows/check.yml` | new | 1 | `make check` on PR and push to `main` | expected |
| `.golangci.yml` | new | 2 | linters and formatters | expected |
| `docs/standards.md` | edit | 1 | one command, the floor, CI | reuse |
| `.github/workflows/security.yml` | edit | 2 | gate pin moved to the current `.github` commit if it moved | reuse |

*Reused, not rewritten:*

- `.github/workflows/security.yml`: the shared gate stays the security
  check on PRs; nothing parallel is added.
- `Makefile` `test`, `lint`, `build`: `check` calls them rather than
  repeating their commands.
- `izam-mohammed/penpot-mcp`'s `Makefile` `tools` target and
  `.golangci.yml`: the pattern copied, not invented.

## Security

The change adds a CI workflow and two developer tools; it adds nothing to
the binary.

| Threat | Here | Handled by | Proved by |
| --- | --- | --- | --- |
| Tampering: an action replaced under its tag | `check.yml` uses third-party actions | every action pinned to a full commit SHA, version in a comment | the shared gate's zizmor stage reports no unpinned action |
| Elevation of privilege: a PR workflow with write access | `check.yml` runs on fork PRs | top-level `permissions: {}`, the job gets `contents: read`, `persist-credentials: false` | zizmor clean on the PR |
| Tampering: a compromised linter or scanner | `make tools` installs binaries | exact versions pinned in the `Makefile` | `make tools` installs the pinned versions |
| Information disclosure: a secret committed | any change | gitleaks in `make check` and in the gate | `make check` fails on a planted fake key in a scratch copy |

No secret is read; the tap token stays in `release.yml` only. The
checklist followed: the CIS-style rules in the shared gate's README
(per-job permissions, SHA pins, no `${{ }}` in `run:`), and SLSA build
level 1 for the release, unchanged.

## Edge cases

- A package with no test files: counted at 0 % in the total; none today.
- `-race` on windows needs cgo: `make check` runs on ubuntu and macos; the
  manual matrix keeps `go test ./...` without `-race` on windows.

## Declined

- `go vet` alone: lost on fit (no errcheck, unused or format check).
- staticcheck alone: lost on compatibility with penpot-mcp's setup.
- go-test-coverage: lost on efficiency and on adding a dependency.
- Codecov: failed the must; coverage would leave the machine.
- The full matrix on every PR: lost on efficiency, and undoes the user's
  `34b4750`.
- CodeQL default setup and a local scan workflow: a second SAST and a
  parallel gate.

## What the tests hold

The existing suite (every package under `cmd/` and `internal/`) is the
test; this plan adds a floor under it. The floor is proved by lowering
nothing and checking `make check` fails when `scripts/coverage-floor` is
set above the measured total in a scratch copy.

## Repository knowledge

Read root `KNOWLEDGE.md` when present before each phase. Follow the global and
repository instructions for recording useful development findings.

## Phases

### Phase 1: the rough cut: one command, run on every PR (needs you: try it)

- [ ] `make check` runs the format check (`gofmt -l`), `go vet`,
  `go test -race -coverprofile`, the floor in `scripts/coverage-floor`
  (90) with the per-package table printed lowest first, and the build
  (expected in `Makefile`), pinned by the floor failing when set to 100 in
  a scratch copy.
- [ ] `.github/workflows/check.yml` runs `make check` on ubuntu for every
  pull request and push to `main`, `permissions: {}` at the top and
  `contents: read` on the job, actions pinned by SHA.
- [ ] `docs/standards.md` names `make check` as the one command, the floor
  and the new workflow.
- [ ] Check: `make check` passes, and prints a total at or over 90 %.
- [ ] You try it: run `make check` on your Mac, then open a throwaway PR
  and say whether the `check` job runs and goes green.

### Phase 2: lint, audit and secrets in the same command

- [ ] golangci-lint v2 lint and `fmt --diff` replace `gofmt -l` and
  `go vet` in `make check`, configured from the rules the code already
  passes (expected in `.golangci.yml`), and `make tools` installs the
  pinned golangci-lint and govulncheck (expected in `Makefile`).
- [ ] govulncheck and gitleaks run at the end of `make check`; the
  `security.yml` pin moves to the current `izam-mohammed/.github` commit
  if it moved.
- [ ] `docs/standards.md`'s Checks and Decided sections updated.
- [ ] Check: `make check` passes, and the PR's Security and check
  workflows are both green.

### Phase 3: open the PR

- [ ] The branch pushed and its PR opened against the branch it was cut from
  (or, if one is already open for this branch, its body brought up to date)
  with a Conventional Commit title for the whole change, a body naming this
  plan, what changed and why, and the checks run with their result. The PR's
  URL printed as the evidence. Opened, never merged.
