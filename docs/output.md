# Output, status and exit codes

## Archive layout

```
relic_HOST_20261106T142226Z.zip
├── files/                     collected data, original paths below the root
│   ├── C/Windows/System32/config/SYSTEM
│   ├── C/$Extend/$UsnJrnl%3A$J
│   └── var/log/auth.log
├── config.yaml                the configuration exactly as provided (its SHA-256 is in the manifest)
├── config.effective.yaml      the configuration with all defaults and command line overrides
├── files.jsonl                one record per matched file (collected, skipped, failed, ...)
├── relic.log                  collection log (JSON lines, level info and above)
└── manifest.json              the result (same as --json), written last
```

- **Writing.** The archive is written in one streaming pass to
  `<name>.partial` and renamed to `<name>` after it was closed and synced. A
  file under the final name is therefore always complete. A leftover
  `.partial` means the process was killed.
- **Path mapping.** Windows drive `C:` becomes `files/C/…`, UNC paths become
  `files/UNC/server/share/…`, Unix paths map to `files/…`. Characters that
  are invalid in Windows file names (`<>:"|?*\`), control characters, `%`
  and trailing dots or spaces are percent-encoded (`$UsnJrnl:$J` →
  `$UsnJrnl%3A$J`), so the archive extracts on every platform and the mapping
  is reversible.
- **Entries.** Each entry keeps the source modification time (ZIP extended
  timestamp, UTC); all other timestamps are in `files.jsonl`.
- **Compression.** `compression: auto` stores already compressed formats
  (`.gz`, `.zip`, `.jpg`, `.jsonlz4`, …) instead of deflating them again.
- **Size limits.** ZIP64 is used automatically for large archives and files.

## File records (`files.jsonl`)

```json
{"artifact":"windows.registry.system","pattern":"C:\\Windows\\System32\\config\\{SAM,SECURITY,SOFTWARE,SYSTEM,DEFAULT}",
 "source_path":"C:\\Windows\\System32\\config\\SYSTEM","archive_path":"files/C/Windows/System32/config/SYSTEM",
 "status":"collected","method":"ntfs_raw","fallback_reason":"open: The process cannot access the file because it is being used by another process.",
 "size":18874368,"bytes_stored":18874368,"hashes":{"sha256":"..."},
 "inode":"mft:91342-4","mode":"-r--r--r--",
 "times":{"modified":"...","accessed":"...","changed":"...","created":"...","fn_created":"..."},
 "collected_at":"2026-11-06T14:22:26.847Z"}
```

| Field | Meaning |
|---|---|
| `status` | `collected`, `truncated` (only `max_file_size` bytes stored), `skipped` (deliberately not read; see `reason`), `failed` (could not be read), `aborted` (stopped by cancel/limit while reading). |
| `reason` | Why a file was skipped, failed or truncated. |
| `method` | `os` or `ntfs_raw`. `fallback_reason` holds the OS error that triggered the raw fallback. |
| `size` / `bytes_stored` | Size at enumeration and bytes written to the archive. A `failed` or `aborted` record with `archive_path` holds the bytes read before the error. |
| `data_offset` | First stored byte in the source (non-zero when a leading sparse region was skipped, e.g. `$UsnJrnl:$J`). |
| `hashes` | Hashes of exactly the stored bytes. |
| `changed_during_read` | Size or modification time changed while reading (live system). |
| `times` | `modified`, `accessed`, `changed` (inode/MFT change), `created` (birth). Raw NTFS: `$STANDARD_INFORMATION` times plus `fn_created` from `$FILE_NAME`. |
| `uid`, `gid`, `inode`, `mode`, `symlink_target` | Source metadata where available. |

Duplicates (already collected by an earlier artifact) and paths removed by
`exclude`/`exclude_paths` produce no record; duplicates are counted in the
summary.

## Result (`--json`, `--status-file`, `manifest.json`)

The result is a stable machine-readable interface (`schema_version: 1`; fields
are only added within a version). Types: `pkg/result`.

```json
{
  "schema_version": 1,
  "tool": {"name": "relic", "version": "0.1.0", "commit": "b14afaeffcf6", "go_version": "go1.26.0"},
  "status": "partial",
  "exit_code": 1,
  "status_reason": "1 file(s) skipped by limits",
  "job": {"id": "mc-task-18342"},
  "host": {"hostname": "HR-PC-017", "os": "windows", "os_version": "Windows 11 Pro 24H2 (build 26100.2314)",
           "arch": "amd64", "user": "NT AUTHORITY\\SYSTEM", "elevated": true, "time_zone": "W. Europe Standard Time"},
  "started_at": "...", "finished_at": "...", "duration_seconds": 312.4,
  "config": {"source": "file:C:\\ProgramData\\Nextron\\Relic\\job.yaml", "sha256": "...", "profile": "standard",
             "overrides": ["output=..."]},
  "output": {"path": "C:\\ProgramData\\Nextron\\Relic\\out\\relic.zip", "format": "zip", "size": 734003200, "sha256": "..."},
  "summary": {"artifacts_selected": 33, "files_matched": 5214, "files_collected": 5190, "files_truncated": 0,
              "files_skipped": 23, "files_failed": 1, "files_duplicate": 12, "bytes_collected": 2147483648,
              "enumeration_errors": 0},
  "limits": [{"limit": "max_total_size", "value": 17179869184, "message": "..."}],
  "artifacts": [{"name": "windows.eventlogs", "status": "collected", "selected_by": "profile:standard",
                 "files_matched": 412, "files_collected": 412, "bytes_collected": 734003200}],
  "issues": [],
  "errors": []
}
```

`output.size` and `output.sha256` are only known after the archive is
closed. They are present in `--json` and `--status-file`, but not in the
archive's own `manifest.json`.

### Overall status and exit codes

| Exit | Status | Meaning |
|---|---|---|
| 0 | `complete` | Every selected artifact was processed; every matching file was collected. Artifacts without matches (`not_found`) are normal. |
| 1 | `partial` | The archive is usable but incomplete. Causes: read or enumeration errors, files skipped or truncated by limits, or a limit (`max_duration`, `max_total_size`, `max_files`, `min_free_space`) ended the collection early. `status_reason` and `limits` explain. |
| 2 | `failed` | No usable archive: the output could not be created, a fatal write error occurred, or no file could be collected at all. |
| 3 | `canceled` | Interrupted by SIGINT/SIGTERM (Ctrl+C, Ctrl+Break, service stop). The archive is finalized with everything collected so far; the file being read is recorded as `aborted`. A second signal exits immediately and leaves `.partial`. |
| 4 | `failed` | Invalid command line or configuration; nothing was collected. With `--json` a result with `issues` is still printed. |

Per-artifact status: `collected`, `partial`, `failed`, `not_found`, `skipped`
(not processed because the run stopped), `not_applicable` (explicitly
requested but no source for this platform). Up to 25 enumeration and pattern
errors are kept per artifact (`errors`, `errors_dropped`).

### Console output

Logs go to stderr (`--log-format text|json`, `--log-level`, `-q`). With
`--json` stdout contains only the result object. Without `--json` a short
summary is printed to stderr. Progress is logged every 15 seconds.
