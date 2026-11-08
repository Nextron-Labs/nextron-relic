# Releasing

Releases are built and published by GitHub Actions
(`.github/workflows/release.yml`) when a version tag is pushed.

## Steps

1. Make sure `main` contains everything for the release and CI is green.
2. Move the entries under `## [Unreleased]` in `CHANGELOG.md` to a new
   section `## [X.Y.Z]` and add the link at the bottom. Merge that to `main`.
3. Tag the commit on `main` and push the tag:

   ```bash
   git switch main && git pull
   git tag -a v0.1.0 -m "Relic 0.1.0"
   git push origin v0.1.0
   ```

## What the workflow does

Each step only runs if the previous one succeeded; nothing is published
otherwise.

1. **ci:** runs the test suite on the tagged commit on Linux, Windows and
   macOS, including the live raw NTFS tests on the elevated Windows runner.
2. **build:**
   - Checks the tag format: `v<major>.<minor>[.<patch>][-<pre-release>]`,
     e.g. `v0.1`, `v0.1.0` or `v1.2.0-rc.1`.
   - Checks that the Go toolchain matches the `toolchain` line in `go.mod`
     (`make check-go`). Patch releases contain security fixes, so binaries
     are never built with an older one.
   - Builds static binaries for all 8 platforms with the version and commit
     stamped in (`relic version`), and writes `SHA256SUMS`.
3. **smoke:** runs exactly these binaries on GitHub runners
   (`.github/workflows/smoke.yml`, `scripts/smoke-test.sh`):
   - Linux amd64 and 386, as root.
   - Windows amd64 and 386, elevated; the 386 binary runs on 64-bit Windows
     under WoW64.
   - macOS arm64, unprivileged.

   Each run checks the checksum and the version, validates the example
   configurations, resolves the `full` profile in a dry run (a pattern error
   fails the test) and collects the `minimal` profile. The archive and every
   file hash in it are verified by `scripts/smoke-check.py` and by
   `relic verify`. On Windows the locked SYSTEM registry hive must have been
   read via raw NTFS.
4. **publish:** publishes the GitHub release with the binaries and
   `SHA256SUMS` (`scripts/publish-release.sh`):
   - If a release or draft for the tag already exists, its title and notes
     are kept, the binaries are attached and a draft is published. This lets
     you prepare the notes in a draft release before tagging.
   - Otherwise the release is created with the version's `CHANGELOG.md`
     section as notes (`scripts/release-notes.sh`), or with GitHub's generated
     notes if there is no section.
   - Tags with a pre-release suffix (`-rc.1`, `-beta`) are marked as
     pre-releases.

Re-running the workflow for the same tag replaces the attached binaries.

## Testing an existing release

The smoke test can also download and test the assets of a published
release:

```bash
gh workflow run smoke.yml -f tag=v0.1.0
gh run watch "$(gh run list --workflow smoke.yml --limit 1 --json databaseId --jq '.[0].databaseId')"
```

Releases before 0.3.0 have no `relic verify`; the test skips that step for
them. Locally, with the release assets in `dist/`:

```bash
EXPECTED_VERSION=0.1.0 scripts/smoke-test.sh dist darwin-arm64
```

## Updating Go

The `toolchain` line in `go.mod` pins the Go release used for builds. The
`go` command downloads it automatically for local builds, and
`actions/setup-go` installs it in CI. To move to a new patch release:

```bash
go mod edit -toolchain=go1.26.11
```

## Assets

| File | Platform |
|---|---|
| `relic-<version>-windows-amd64.exe` | Windows x64 |
| `relic-<version>-windows-386.exe` | Windows x86 (32-bit) |
| `relic-<version>-windows-arm64.exe` | Windows on ARM |
| `relic-<version>-linux-amd64` | Linux x86-64 |
| `relic-<version>-linux-386` | Linux x86 (32-bit) |
| `relic-<version>-linux-arm64` | Linux ARM64 |
| `relic-<version>-darwin-amd64` | macOS Intel |
| `relic-<version>-darwin-arm64` | macOS Apple Silicon |
| `SHA256SUMS` | checksums of all binaries |

Verify a download with `sha256sum -c SHA256SUMS --ignore-missing` (Linux)
or `shasum -a 256 -c SHA256SUMS --ignore-missing` (macOS).

## Not covered yet

- **Code signing.** Windows Authenticode and macOS signing/notarization are
  not part of the workflow. Unsigned binaries may trigger SmartScreen or
  Gatekeeper warnings, and some EDR products block unsigned tools. Signing
  needs Nextron's certificates as repository secrets.
- **Release from `main` only.** The workflow does not check that the tagged
  commit is on `main`; that is left to the person tagging.
