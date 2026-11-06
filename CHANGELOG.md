# Changelog

All notable changes to Nextron Relic are documented here. Versions follow
[Semantic Versioning](https://semver.org/). The section of a version is used
as the release notes when its tag is pushed (see `docs/releasing.md`).

## [Unreleased]

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
