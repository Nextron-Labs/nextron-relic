# ASGARD integration

Relic is designed to run unchanged from an ASGARD Management Center playbook
and standalone. The ASGARD agent already handles distribution, execution and
transport, so Relic has no network functions of its own.

## Playbook pattern

ASGARD custom playbooks consist of up to 16 sequential steps: *Download File
from ASGARD*, *Run Command Line on Endsystem*, *Upload File to ASGARD*.
stdout/stderr of commands are reported back.

| Step | Type | Value (Windows example) |
|---|---|---|
| 1 | Download file | `relic.exe` → `C:\ProgramData\Nextron\Relic\relic.exe` |
| 2 | Download file | generated `job.yaml` → `C:\ProgramData\Nextron\Relic\job.yaml` |
| 3 | Run command | `C:\ProgramData\Nextron\Relic\relic.exe collect -c C:\ProgramData\Nextron\Relic\job.yaml --json -q --status-file C:\ProgramData\Nextron\Relic\out\status.json` |
| 4 | Upload file | `C:\ProgramData\Nextron\Relic\out\relic.zip` |
| 5 | Upload file (optional) | `C:\ProgramData\Nextron\Relic\out\status.json` |
| 6 | Run command (optional) | cleanup of `C:\ProgramData\Nextron\Relic\out` |

See `examples/asgard-windows-triage.yaml` for a matching configuration.

### Rules for the generated configuration

- Use a **fixed `output.path`** (no `{timestamp}`) with `overwrite: true`, so
  the upload step can reference the archive. The archive appears under its
  final name only once it is complete. A failed or killed run leaves no file
  there, only a `.partial`, so the upload step fails visibly instead of
  transferring a broken archive.
- Put the ASGARD task ID into `job.id` and anything useful into `job.labels`.
  Both are echoed into the result and the manifest, which links evidence and
  task.
- Set `limits.max_duration` below the playbook timeout. Relic then finalizes a
  usable (`partial`) archive instead of being killed.
- Keep `min_free_space` so that a collection never fills the system drive.
- Validate generated files on the server with `relic validate job.yaml --os
  windows --json` (works on Linux) or against `relic config schema`.

### Evaluating the run

- **stdout** (with `--json`, `-q`) is exactly one JSON result object. Use it
  to show status, counters, limits and errors in the UI.
- **The exit code** distinguishes `0` complete, `1` partial, `2` failed, `3`
  canceled and `4` invalid configuration. Treat 0 and 1 as success with a
  usable archive.
- The archive's `manifest.json` and `files.jsonl` carry the full provenance
  (per-file hashes, timestamps, method, status) for later processing.

## LocalSystem considerations

- Relic enables `SeBackupPrivilege` and opens files with backup semantics, so
  ACLs do not block collection. Locked files fall back to raw NTFS reads.
- `%UserHome%` covers all user profiles via the registry ProfileList. It does
  not use the environment of the SYSTEM account.
- The process runs with below-normal CPU and low I/O priority by default
  (`limits.priority: low`).
- Relic is a console program without UI. It reacts to Ctrl+C/Ctrl+Break and
  console close events by finalizing the archive. A hard `TerminateProcess`
  cannot be handled; in that case only the `.partial` file remains.

## Open points for the ASGARD side

- How ASGARD's own encrypted evidence collection should be combined with
  Relic archives (Relic currently writes unencrypted ZIP; see the roadmap in
  `docs/design.md`).
- Whether the agent can deliver the configuration via stdin
  (`relic collect -c -` is supported) to avoid a separate download step.
