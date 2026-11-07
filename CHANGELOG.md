# Changelog

All notable changes to Nextron Relic are documented here. Versions follow
[Semantic Versioning](https://semver.org/). The section of a version is used
as the release notes when its tag is pushed (see `docs/releasing.md`).

## [Unreleased]

### Added

- **Artifact catalog: 171 artifacts (was 77).** Based on a comparison with
  KAPE, ForensicArtifacts, UAC, Velociraptor, Aftermath, mac_apt and current
  DFIR research. See `docs/artifacts.md`.
  - **Windows:**
    - RMM and remote access tools: ScreenConnect (client and server),
      Splashtop, Atera, NinjaOne, Datto, N-able, Kaseya, Action1,
      MeshCentral/Tactical RMM, Level, ImmyBot, RustDesk, Quick Assist,
      SimpleHelp, LogMeIn, Zoho Assist, UltraViewer and others.
    - Antivirus/EDR logs: Sophos, Symantec, Trellix/McAfee, ESET,
      Bitdefender, Trend Micro, Malwarebytes, Kaspersky.
    - PowerShell transcripts and profiles, WER reports, CryptnetUrlCache
      (certutil downloads), Notepad tab state, Impacket remnants, custom
      shims, Office persistence, RDP client files, VPN client logs,
      network scanners, Capability Access Manager, StartupInfo, netlogon
      and debug logs.
    - Server roles: SYSVOL/NETLOGON, IIS configuration and history,
      Exchange webshell locations, SQL Server, DHCP, Veeam, Tomcat.
    - `full` only: Exchange logs, DNS debug log, `$TxfLog`, Syscache,
      staging folders, ETL traces, update logs, OneDrive, Windows.old,
      Sticky Notes.
  - **Linux:**
    - Persistence: PAM modules, package-manager hooks, boot loader and
      initramfs, network hooks, polkit/D-Bus, Python startup hooks, dpkg
      maintainer scripts and md5sums.
    - Kernel and network state from `/proc` (modules, taint, sockets,
      `core_pattern`, binfmt_misc).
    - Security configuration (auditd, logrotate, SELinux/AppArmor,
      firewall), containers (Docker, containerd, Podman, Kubernetes, k3s),
      cloud agents (cloud-init, SSM, Azure run-command).
    - Web application configuration (`.htaccess`, `.user.ini`, PHP,
      compiled JSP), application logs outside `/var/log`, vCenter appliance
      logs, DHCP leases, directory services, root mail, locate database,
      desktop activity, remote access tool logs.
    - `full` only: container logs, `/tmp`, `/var/tmp` and `/dev/shm`, local
      binaries and kernel modules, boot images.
  - **macOS:**
    - Gatekeeper/syspolicyd databases, XProtect behavior service database,
      CoreAnalytics, Terminal and iTerm2 saved state.
    - Persistence: authorization and PAM plugins, plugin load points,
      privileged helpers, system extensions, user application settings.
    - Managed preferences, `/Library/Preferences`, network usage
      per process, remote access tools, login records, Bluetooth and iOS
      devices, application inventory, Spotlight shortcuts, Biome.
    - `full` only: all Biome streams, PowerLog, Screen Time, iCloud Drive,
      internet accounts, receipt BOMs.

- **`relic verify <archive>`** checks an archive without extracting it: ZIP
  structure and CRC-32 of every entry, the metadata files, every stored file
  against the size and hashes in `files.jsonl`, and the manifest counters.
  `--result status.json` also checks the archive size and SHA-256 against the
  collection result. Exit code 0 verified, 2 problems found. The release
  smoke test runs it on every built binary.
- **Ad-hoc paths:** `--path` collects files matching a path pattern (artifact
  `cli.paths`, repeatable); `--exclude-path` excludes paths.
- **Job metadata flags:** `--job-id`, `--job-name`, `--label key=value`.
- **Flags for configuration values** that previously needed a file: `--hash`,
  `--follow-symlinks`, `--compression`, `--compression-level`, `--oversize`,
  `--max-files`, `--min-free-space`, `--read-rate`, `--priority`.
- **Logging:**
  - `-v`/`--verbose` (same as `--log-level debug`).
  - `--log-file` writes the log as JSON lines with UTC timestamps (appended,
    not affected by `-q`); it also survives runs that produce no archive.
  - `--log-level debug` now also applies to `relic.log` in the archive.
  - `--progress-interval` (default 15s, `0` disables); progress messages show
    the artifact step (`5/34`) and the elapsed time.
  - A warning at the start when Relic does not run elevated (root,
    administrator/LocalSystem), and the start message records user and
    elevation.
- **`relic list artifacts --search/--category`** to find artifacts by words in
  their name, description or paths, or by category.
- **CLI reference** in `docs/cli.md`; flag help names the expected value
  (`--max-file-size size`).

### Changed

- **Profiles.** `standard` now also contains the systemd journal (the only
  log on Debian 12 and later, which have no `auth.log`) and the vendor systemd
  units. Existing artifacts collect more:
  - Linux: account backup copies, wtmpdb and lastlog2, utmp, cron logs,
    logind sessions, transient and generated systemd units, lingering users,
    OpenRC, xinetd, system-wide zsh/csh/fish files.
  - SSH (Linux and macOS): `sshrc`, `~/.ssh/rc`, host public keys,
    `authorized_keys*` backups.
  - macOS: at jobs, launchd overrides, login hooks, rc and periodic scripts.
  - Windows: AnyDesk and TeamViewer incoming connection records, Defender
    `MpCmdRun.log` and quarantine metadata, the Group Policy Preferences
    cache, PowerShell scheduled jobs, Desktop and Start Menu shortcuts.
- **`%UserHome%` on Windows** also covers the 32-bit LocalSystem profile
  (`%SystemRoot%\SysWOW64\config\systemprofile`), used by 32-bit services
  and RMM agents.
- The release smoke test also resolves the `full` profile in a dry run on
  every platform and fails on any pattern error.
- **Selection without a profile:** `relic collect -a <artifact>` or
  `--path` without `--profile` and without `--config` now collects only what
  was named. Before, the artifacts were added to the default `standard`
  profile. Add `--profile standard` for the old behavior.
- `relic.log` timestamps are UTC, like all other timestamps in the archive.

### Fixed

- **`config.yaml` in every archive** ([#6](https://github.com/Nextron-Labs/nextron-relic/issues/6)):
  runs without `--config` wrote no `config.yaml` and recorded an empty
  `config.sha256`, although the archive layout documented the file as always
  present. Such runs now archive the built-in defaults (`standard` profile) as
  `config.yaml`, and `config.sha256` is its hash; command line flags are
  listed in `config.overrides` (`profile=` when `--artifact`/`--path`
  replaced the default profile) and the result is `config.effective.yaml`.
  `relic verify` and the release smoke test check that `config.yaml` matches
  the hash in the manifest.
- **AnyDesk `service.conf` is no longer collected.** It holds the password
  hash and the client's private key, which contradicts the rule that the
  built-in catalog collects no credentials or private keys.

## [0.2.0]

Maintenance release: archive naming fix, 32-bit Windows under WoW64, and a
release pipeline that runs every binary before it is published. A development
build was also exercised on a Windows 11 Enterprise 25H2 test system (raw NTFS
fallback, NTFS metadata files, `$J` sparse trimming, hash provenance, exit
codes, `oversize: truncate`, 32-bit build under WoW64).

### Fixed

- **Archive without `.zip` extension** ([#3](https://github.com/Nextron-Labs/nextron-relic/issues/3)):
  `--output C:\relic-out` (a path that does not exist, without a trailing
  separator) wrote the archive as an extension-less file `C:\relic-out`,
  which `*.zip` wildcards, Explorer and `Expand-Archive` ignore. Relic now
  appends `.zip` to any archive name that does not end in `.zip`
  (`C:\relic-out.zip`) and logs a warning. Existing directories and paths
  ending with a separator still receive the default file name. This also
  applies to `output.path` in configuration files.
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

[Unreleased]: https://github.com/Nextron-Labs/nextron-relic/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/Nextron-Labs/nextron-relic/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Nextron-Labs/nextron-relic/releases/tag/v0.1.0
