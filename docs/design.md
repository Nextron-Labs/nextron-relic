# Design notes

## Starting point (inventory, 2026-11-06)

- The repository contained only a README, a Go `.gitignore` template and the
  GPL-3.0 license, with no code, tests or project instructions.
- **THOR (`thorx`, proprietary)** contains a small "Artifact-Collector"
  module and a raw NTFS reader:
  - `lib/modules/collector`: YAML `FILE`/`GLOB` sources, a full-volume MFT
    walk, a ZIP writer with a free-space guard, SHA-256 only in the log.
  - `lib/util/mftread`: about 270 lines on Velocidex go-ntfs.
  - Concepts adopted from it: cached per-volume NTFS contexts, a paged
    sector reader, converting go-ntfs panics into errors, OS read first with
    raw fallback, and a minimum free space check.
- **CyLR (GPL-3.0, C#, dormant since 2022)** is the functional reference.
  Comparison below.

## Language and packaging: Go

- The repository was prepared for Go (`.gitignore`).
- THOR, Aurora and the ASGARD tooling are Go, and the relevant libraries
  exist as Go modules: go-ntfs, and Nextron's go-priority, go-ionice and
  jsonlog.
- With `CGO_ENABLED=0` every target is one static binary of about 6 MB. No
  runtime and no installer are needed. Built targets: windows/amd64, 386,
  arm64; linux/amd64, 386, arm64; darwin/amd64, arm64.
- Rust would give the same deployment properties but would not reuse any of
  the existing components.

Go ≥ 1.21 no longer supports Windows 7/8 or Server 2008 R2/2012. The minimum
is Windows 10 / Server 2016 (see limitations in the README).

## Architecture

```
cmd/relic                 entry point
internal/cli              commands: collect, validate, list, show, config, version
pkg/config                configuration types, defaults, strict parser, validator, JSON Schema (public)
pkg/result                result, status, exit codes, file records (public)
internal/catalog          embedded artifact catalog + profiles (YAML), selection logic
internal/pathspec         variables, brace/wildcard/** patterns, minimal-walk resolver, path styles
internal/source           access methods: os (per-OS), ntfs_raw, auto fallback
internal/ntfsraw          NTFS volume/stream access on go-ntfs (portable; device open is Windows-only)
internal/archive          streaming ZIP writer, path mapping, atomic finalize
internal/collector        orchestration: selection → resolve → open → stream+hash → records; limits, cancel, status
internal/hostinfo         host identity, %UserHome% & friends, free space, process priority
```

The OS-specific code sits behind small interfaces:

- `pathspec.FS` is the view used for resolution (OS file system or raw NTFS).
- `source.Accessor` opens files with a method.
- `hostinfo` provides variables, free space and priority per platform.

The collection logic itself is shared by all platforms. Because path styles
are explicit (`pathspec.Style`), Windows patterns and the raw NTFS pipeline
are also unit tested on macOS and Linux against an NTFS image.

## Declarative vs. custom logic

| Declarative (catalog YAML / custom artifacts) | Needs code |
|---|---|
| file and directory artifacts with wildcards, per-OS sources, exclusions | user profile discovery (ProfileList, /etc/passwd, /Users) |
| access method per source (`raw` for NTFS metadata files) | raw NTFS parsing, locked-file fallback |
| sparse handling flag (`trim_sparse` for `$UsnJrnl:$J`) | sparse range detection |
| profiles as ordered artifact lists | limits, cancellation, status semantics |

Planned source types such as `command` (volatile data), `registry` (key
exports) and `live` (processes, network) need code. Schema version 1 rejects
them explicitly ("unsupported source type") instead of ignoring them.

## Comparison with CyLR

| Topic | CyLR | Relic |
|---|---|---|
| Artifact selection | flat path list, no per-OS sections, replace (`-c`) or add (`-d`) | named artifacts with per-OS sources, profiles, include/exclude by wildcard, custom artifacts in the same format |
| Matching | walks every file of every volume and matches each one | reads only the directories a pattern needs; literal paths are a single `stat` |
| Locked files | raw read via DiscUtils for all files, no per-file fallback | OS read first, raw NTFS fallback per file (recorded); `method: raw` for metadata files |
| NTFS metadata | `$MFT`, `$LogFile`, optional `$J` copied as a full stream | same plus `$Boot` and `$Secure:$SDS`; `$J` without the leading sparse region, offset recorded |
| Symlinks/junctions | skips reparse-point directories; follows file symlinks | never recurses into symlinks/junctions unless configured; explicit symlinks followed, others recorded |
| Archive | ZIP, local-time DOS timestamps, no manifest or hashes | ZIP with UTC timestamps, manifest, per-file records with hashes and all timestamps, atomic finalize |
| Errors | a read error during copy aborts the run; several failures still exit 0 | every file has a status and reason; distinct exit codes 0–4; JSON result |
| Limits | none | file size, total size, file count, duration, free space, memory, read rate, priority |
| Transport | built-in SFTP (password on the command line) | none on purpose: ASGARD transports; standalone users copy the archive |
| Default list errors | `autorized_keys`, `/etc/issues`, `/etc/systemd.d` | corrected; covered by tests that load and validate the catalog |

Adopted from CyLR: zero-configuration defaults per OS, ProfileList-based user
profiles, raw collection of NTFS metadata, dry run, optional zip
compression level.

Not adopted:
- The SFTP upload. It is out of scope while ASGARD handles transport, and
  CyLR's version passes the password on the command line.
- The ZIP password. Plain ZipCrypto is weak; see the roadmap.

## Licensing

- Relic is GPL-3.0 (existing `LICENSE`).
- **No THOR code was copied.** THOR is proprietary; including its code would
  mean publishing it under the GPL, which is a decision for Nextron. The raw
  NTFS layer was re-implemented on the public go-ntfs API, applying the
  concepts listed above.
- **Dependencies** (all GPL-3.0 compatible):
  - go-ntfs: Apache-2.0
  - yaml.v3: MIT/Apache-2.0
  - cobra and pflag: Apache-2.0/BSD-3
  - klauspost/compress: BSD-3/Apache-2.0
  - x/sys: BSD-3
- **CyLR** is GPL-3.0, so its code could be reused, but none was. Only its
  behavior was studied.
- **Test fixture.** The NTFS test image comes from go-ntfs (Apache-2.0), see
  `internal/ntfsraw/testdata/README.md`.

## Assumptions

- The ASGARD agent provides transport and executes Relic as LocalSystem.
  Relic needs no network access.
- An unencrypted ZIP is acceptable for the first iteration, because ASGARD
  transports evidence itself.
- Cookies, saved passwords and private keys are not collected by default.
  Password hashes (`/etc/shadow`, macOS dslocal user records) are part of
  `standard`, because account changes are a core triage question.
- `%UserHome%` on Linux includes service account homes from `/etc/passwd`,
  e.g. `/var/www` for webshell shell histories.
- Default limits are conservative for endpoints: 4 GiB per file, 32 GiB
  total, 4 h, 1 GiB minimum free space, low priority.

## Roadmap

1. **Windows validation on real systems.** CI on GitHub Windows runners runs
   the live raw NTFS tests. Next: Windows Server with roles (DC, IIS) and
   locked/large `$J`.
2. **Volume Shadow Copies** as an additional source, for consistent registry
   hives and historic versions.
3. **Encrypted output**, preferably public-key (e.g. age/X25519), so endpoints
   never hold a decryption key. Must be aligned with ASGARD's evidence
   encryption.
4. **Additional source types:**
   - `command`: volatile data such as process lists and network connections,
     with bounded output.
   - `registry`: exports of keys from live hives.
   - macOS `log collect` for unified logs.
5. **All fixed NTFS volumes** (not only `%SystemDrive%`) for `$MFT`/`$J`
   artifacts, via a `%FixedDrives%` variable.
6. **Parallel compression** for faster large collections. The current
   pipeline is single-threaded on purpose to keep the endpoint load low.
7. **Import of ForensicArtifacts/KAPE target definitions** into the catalog.
