# claim: what every change passes

Recorded on 22/10/2026 from what the repository already runs. There is no
single check command yet; `plans/01-quality-gates.md` adds `make check` and
a coverage floor, and updates this file when it lands. Until then a change
is done when the commands below pass locally and the Security workflow is
green on its pull request.

## Toolchain

| What | Version | Pinned in |
| --- | --- | --- |
| Go | 1.26 | the `go` line in `go.mod` |
| Modules | Go modules | `go.sum`, committed |
| GoReleaser | v2 (`~> v2`) | `.github/workflows/release.yml`, `.goreleaser.yml` |
| pre-commit hooks | gitleaks v8.30.1, pre-commit-hooks v6.0.0 | `.pre-commit-config.yaml` |

## Checks

| Check | Tool | Config | Command | Passing means |
| --- | --- | --- | --- | --- |
| Format | gofmt | none | `gofmt -l .` | prints nothing (not enforced by any script) |
| Lint | go vet | none | `make lint` (`go vet ./...`) | no findings |
| Types | the Go compiler | `go.mod` | `make build` | builds |
| Unit tests | `go test` | none | `make test` (`go test ./...`) | all pass |
| Coverage | `go test -coverprofile` | none | `go test -coverprofile=cover.out ./... && go tool cover -func=cover.out` | no floor yet; 96.1 % on 22/10/2026 |
| Dependency audit | osv-scanner and govulncheck, in the shared gate | `.github/workflows/security.yml` | on every PR | reports, does not block |
| Static analysis | semgrep (`p/golang` and the default packs), in the shared gate | same | on every PR | reports, does not block |
| Workflow audit | zizmor, in the shared gate | same | on every PR | reports, does not block |
| Secret scan | gitleaks | `.gitleaksignore`, `.pre-commit-config.yaml` | pre-commit hook; the shared gate on every PR | no findings; blocks the merge |
| Build | `go build` | none | `make build` | builds `bin/claim` |
| Release config | GoReleaser | `.goreleaser.yml` | `make snapshot` (`goreleaser build --snapshot --clean`) | builds every target |

## Tests

Tests sit beside the code as `<file>_test.go`, with extra cases in
`<file>_more_test.go`. Git repositories are built in temporary directories
by the tests; GitHub is replaced by `net/http/httptest` servers. Prompts,
hosts and the exit path are injectable (commit `33d29ac`), so the command
layer in `cmd/claim/` is tested too. One test:
`go test -run TestName ./internal/<pkg>`.

## Constraints

- A tool that rewrites history and force-pushes: remote commands are
  scan-only unless `--apply` is given, and the rewrite asks first unless
  `--force`.
- Credentials (`CLAIM_GITHUB_TOKEN`, `GITHUB_TOKEN`, the `gh` session) are
  read, never printed; saved accounts are encrypted at rest.
- Releases publish to `izam-mohammed/homebrew-tap` and
  `izam-mohammed/scoop-bucket` through `TAP_GITHUB_TOKEN` on a `v*` tag.

## Conventions

- Conventional Commits with a scope where it helps (`fix(auth):`,
  `ci(deps):`), lowercase subject, a body saying why.
- Dependabot commits use `deps` and `ci(deps)` prefixes
  (`.github/dependabot.yml`).
- Every change reaches `main` through a pull request.

## CI

- `.github/workflows/security.yml`: the shared gate on every pull request
  and weekly; stage 1 (secrets) blocks.
- `.github/workflows/ci.yml`: test, vet and build on ubuntu, macos and
  windows, run by hand only (`gh workflow run ci.yml --ref <branch>`).
- `.github/workflows/release.yml`: goreleaser on a `v*` tag.

## Decided

- The shared security gate over a repository-local scan: added in commit
  `d9a12eb` from `izam-mohammed/repo-template`.
- The three-OS test matrix manual only: commit `34b4750`.
- A Homebrew formula over a cask, so Linux installs too: `.goreleaser.yml`.
