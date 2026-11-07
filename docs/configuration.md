# Configuration

Relic is controlled by a single YAML file (schema version 1). Without a file,
`relic collect` uses the built-in defaults with the `standard` profile.

```bash
relic config show                 # effective defaults; a good starting template
relic config show -c job.yaml     # effective configuration of a file
relic config schema               # JSON Schema (draft 2020-12) for generators
relic validate job.yaml --os windows
```

## Design rules

- **Versioned.** `version: 1` is required. A newer version is rejected with a
  clear message instead of being half-understood.
- **Strict.** Unknown keys, wrong types and invalid values are errors. Every
  finding has a YAML path, line and column, and all problems are reported at
  once (with suggestions for typos):

  ```
  error (line 3, column 3): collect.profil: unknown field "profil" (did you mean "profile"?)
  error (line 12, column 18): limits.max_file_size: invalid size "12 apples": unknown unit " apples" (...)
  ```
- **Defaults are explicit.** Omitted values take the defaults shown by
  `relic config show`. An empty section (`limits:`) keeps the defaults.
- **Machine friendly.** Sizes and durations accept integers (bytes, seconds),
  so generators never need to format units. `relic config schema` provides
  the structure; `relic validate --json` the semantic checks (artifact names,
  pattern syntax, platform restrictions) without touching the local system.

## Reference

```yaml
version: 1                          # required

job:                                # optional, copied into the result
  id: mc-task-18342             # also usable as {job_id} in output.path
  name: Ransomware triage
  labels: {case: IR-2026-0042}

collect:
  profile: standard                 # minimal | standard | full (see docs/artifacts.md)
  artifacts: [windows.rdp_bitmap_cache, "browser.*"]   # add by name or wildcard
  exclude_artifacts: [browser.brave]                   # remove by name or wildcard
  exclude_paths: ['%UserHome%\AppData\Local\Temp\**']  # never collect these paths
  custom_artifacts: []              # see below; always collected unless excluded
  method: auto                      # auto | os | raw (see "Access methods")
  hashes: [sha256]                  # md5, sha1, sha256, sha512
  follow_symlinks: false            # follow symlinks matched by wildcards
  max_depth: 32                     # recursion limit for "**"

limits:                             # 0 disables a limit
  max_file_size: 4GiB               # per file
  oversize: skip                    # skip | truncate (store the first max_file_size bytes)
  max_total_size: 32GiB             # sum of collected source bytes
  max_files: 0                      # number of files
  max_duration: 4h                  # wall clock; then the archive is finalized
  min_free_space: 1GiB              # stop before the output volume runs full
  max_memory: 512MiB                # Go soft memory limit
  read_rate: 0                      # bytes per second (0 = unthrottled)
  priority: low                     # low | normal (CPU and I/O priority)

output:
  path: relic_{hostname}_{timestamp}.zip   # file, or directory ending in / or \
  compression: auto                 # auto (store already compressed files) | deflate | store
  compression_level: 5              # 1 (fastest) .. 9 (smallest)
  overwrite: false
```

Sizes: integer bytes or a number with unit. `KB/MB/GB/TB` are powers of
1000, `KiB/MiB/GiB/TiB` and `K/M/G/T` are powers of 1024.
Durations: integer seconds or `30s`, `90m`, `2h`, `1h30m`.

Output placeholders: `{hostname}`, `{timestamp}` (UTC, `20261106T142226Z`) and
`{job_id}` (`nojob` if unset). If `--output` names a directory, the file name
pattern of the configuration is kept.

How the output path is interpreted:

| Path | Archive |
|---|---|
| ends with `/` (or `\` on Windows) | `<dir>/relic_{hostname}_{timestamp}.zip`; the directory is created |
| an existing directory | `<dir>/relic_{hostname}_{timestamp}.zip` |
| ends with `.zip` (any case) | used as is |
| anything else, e.g. `C:\relic-out` | `.zip` is appended (`C:\relic-out.zip`) and a warning is logged |

## Artifact selection

The selection is built in this order, and this order is also the collection
order (what is collected first is kept when a size or time limit is hit):

1. the artifacts of `profile` (profiles include each other: `full` ⊃ `standard` ⊃ `minimal`),
2. `artifacts` (names or wildcards such as `windows.registry.*`),
3. `custom_artifacts`,
4. minus `exclude_artifacts`.

Only artifacts with a source for the running platform are collected, so one
configuration can serve Windows, Linux and macOS endpoints. An explicitly
listed artifact without a source for the platform is reported as
`not_applicable`.

## Custom artifacts

Custom artifacts use the same format as the built-in catalog
(`relic show artifact <name>` prints any built-in one as a template):

```yaml
collect:
  custom_artifacts:
    - name: custom.app_logs          # lower case, digits, '.', '_', '-'; must not clash with built-ins
      description: Logs of the line-of-business application
      category: logs
      sources:
        - os: [windows]
          paths:
            - '%ProgramData%\Contoso\App\logs\**\*.log'
          exclude:
            - '%ProgramData%\Contoso\App\logs\debug\**'
        - os: [linux, macos]
          paths: ['/opt/contoso/app/log/*.log*']
```

Source fields:

| Field | Meaning |
|---|---|
| `type` | `file` (default; the only type in version 1). Other values are rejected as unsupported. |
| `os` | `windows`, `linux`, `darwin`/`macos`. Without `os` the paths must be valid on all platforms. |
| `paths` | Path patterns (below). |
| `exclude` | Patterns removed from this source's matches. |
| `method` | Overrides `collect.method` for this source. `raw` requires `os: [windows]`. |
| `trim_sparse` | Raw NTFS only: skip the leading sparse region (used for `$UsnJrnl:$J`). |

## Path patterns

Patterns are absolute paths with wildcards:

| Syntax | Meaning |
|---|---|
| `*` | any characters within one path component (on Windows not `:`, so alternate data streams are only matched when named) |
| `?` | one character |
| `[abc]`, `[a-z]`, `[!abc]` | character class |
| `{a,b,c}` | alternatives, may be nested and may contain `/` |
| `**` | zero or more directories (a whole component); at the end: all files below |
| `%Name%` | variable (below); `%%` is a literal `%` |
| `file:stream` | NTFS alternate data stream (Windows) |

Windows patterns accept `\` and `/` and are case-insensitive; macOS patterns
are case-insensitive; Linux patterns are case-sensitive. `..` is rejected.

Variables (case-insensitive):

| Variable | Windows | Linux | macOS |
|---|---|---|---|
| `%UserHome%` | every profile from the registry ProfileList (including LocalSystem, LocalService, NetworkService) and every directory below `%SystemDrive%\Users` | `/root`, every directory below `/home`, every existing home in `/etc/passwd` (service accounts such as `www-data` included) | every directory below `/Users`, `/var/root` |
| `%SystemDrive%`, `%SystemRoot%`, `%WinDir%`, `%ProgramData%`, `%ProgramFiles%`, `%ProgramFiles(x86)%` | from the environment with sensible fallbacks; `%ProgramFiles(x86)%` expands to nothing on 32-bit Windows | – | – |
| any other `%NAME%` | environment variable of the collector process (note: under LocalSystem, `%APPDATA%` etc. point to the system profile) | not available | not available |

Variable values are literal: a profile called `bob [admin]` is not treated as
a character class. A multi-valued variable expands a pattern once per value.

Matching rules:

- Only the directories a pattern needs are read; volumes are never walked in
  full.
- Files matched by several patterns or artifacts are collected once, by the
  first artifact (`files_duplicate` counts the rest).
- Explicitly named files (literal names or `{a,b}` alternatives) that are
  symbolic links are followed; symlinks matched by wildcards are recorded as
  skipped unless `follow_symlinks` is set. Recursion never enters symlinked
  directories or Windows junctions unless `follow_symlinks` is set.
- Directories, FIFOs, sockets and devices are never read (recorded as skipped).

## Access methods

| Method | Behavior |
|---|---|
| `os` | Regular file API. Windows: `CreateFile` with full sharing and backup semantics (`SeBackupPrivilege` is enabled, so ACLs do not block LocalSystem/administrators). Linux: `O_NOATIME` (access times are not changed), non-blocking open. |
| `raw` | Windows only. Parses NTFS directly from the volume (`\\.\C:`) with Velocidex go-ntfs. Reads locked files and NTFS metadata files (`$MFT`, `$LogFile`, `$UsnJrnl:$J`, `$Secure:$SDS`). Requires administrative rights. |
| `auto` (default) | `os`; on Windows, if opening fails with a sharing, lock or access violation, falls back to `raw`. The record keeps the original error as `fallback_reason`. |

The method actually used is recorded per file (`os` or `ntfs_raw`).
