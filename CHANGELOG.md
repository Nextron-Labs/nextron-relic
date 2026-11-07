# Changelog

All notable changes to Nextron Relic are documented here. Versions follow
[Semantic Versioning](https://semver.org/). The section of a version is used
as the release notes when its tag is pushed (see `docs/releasing.md`).

## [Unreleased]

### Fixed

- **32-bit Windows binary on 64-bit Windows (WoW64):** Windows silently
  redirected file access below `%SystemRoot%\System32` to `SysWOW64`, so
  registry hives, event logs, scheduled tasks and other System32 artifacts
  were not found. Relic now reaches the real System32 directory. It also
  reads the 64-bit registry view for user profiles and the OS version, and
  resolves `%ProgramFiles%` to the native `Program Files` directory.

### Changed

- Clarified that raw NTFS access on Windows needs an elevated process
  (administrator or LocalSystem).
- **Go 1.26.10.** Binaries are built with the latest Go 1.26 patch release
  (0.1.0 used 1.26.0). The toolchain is pinned in `go.mod`, and release
  builds fail on any other version.
- **Release smoke test.** Release binaries are run on Linux, Windows
  (including 32-bit under WoW64) and macOS runners before they are
  published. The same test runs for every pull request.
- **CI actions** updated to their current major versions (Node.js 24
  runtime).

## [0.1.0]

First public version of Relic, a cross-platform collector for forensic
artifacts, shipped as a single static binary per platform.

### Collection

- **Built-in catalog** of 77 artifacts for Windows, Linux, macOS and browsers,
  with the profiles `minimal`, `standard` (default) and `full`. Custom
  artifacts use the same YAML format.
- **Path patterns** with variables (`%UserHome%`, `%SystemRoot%`, …),
  wildcards, `{a,b}` alternatives and `**`. Only the directories a pattern
  needs are read; volumes are never walked in full.
- **Windows file access.** Regular reads use backup semantics. Files locked
  by the system (registry hives, event logs, ESE databases) fall back to raw
  NTFS reads. `$MFT`, `$LogFile`, `$UsnJrnl:$J` (without its leading sparse
  region) and `$Secure:$SDS` are read directly.
- **Linux and macOS file access.** Reads never block on FIFOs or devices.
  On Linux, access times are not changed (`O_NOATIME`).
- **Archive.** One ZIP with `manifest.json` and `files.jsonl`: per-file
  status, reason, access method, hashes and timestamps, including files that
  were skipped or could not be read. It also contains the collection log and
  the original and effective configuration. The archive only gets its final
  name once it is complete.
- **Limits** for file size, total size, file count, run time, free disk
  space, memory and read rate. Collections run at low CPU and I/O priority by
  default and are stopped cleanly on Ctrl+C/SIGTERM, with a finalized archive.

### Configuration and automation

- **Configuration** is a versioned YAML schema (version 1) with strict
  validation. Every finding has its YAML path, line and column.
  `relic config schema` prints the JSON Schema.
- **Commands:** `collect`, `validate --os`, `list`, `show`, `config show`,
  `config schema`, `version`.
- **Results.** `--json` prints the result on stdout and `--status-file`
  writes it to a file. Exit codes: 0 complete, 1 partial, 2 failed,
  3 canceled, 4 invalid configuration.

### Platforms

- Binaries for Windows (amd64, 386, arm64), Linux (amd64, 386, arm64) and
  macOS (amd64, arm64).

### Known limitations

- Windows support has only been exercised in CI (GitHub Actions runners),
  not yet on production endpoints.
- NTFS metadata files are collected only from `%SystemDrive%`.
- No Volume Shadow Copy access, no encrypted output, no upload function.
- On macOS, TCC-protected locations need Full Disk Access.

See README.md for details.

[Unreleased]: https://github.com/Nextron-Labs/nextron-relic/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Nextron-Labs/nextron-relic/releases/tag/v0.1.0
