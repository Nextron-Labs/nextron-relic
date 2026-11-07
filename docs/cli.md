# Command line reference

```
relic collect     collect artifacts into a ZIP archive
relic verify      verify an archive: integrity, manifest and every file hash
relic validate    validate a configuration file (also for another platform)
relic list        list built-in artifacts or profiles
relic show        show an artifact or profile definition
relic config      print the effective configuration or the JSON Schema
relic version     print version information
```

`relic <command> --help` prints the flags of each command.

## `relic collect`

Without `--config` the built-in `standard` profile is collected. If
`--artifact` or `--path` is given without `--profile` (and without
`--config`), only those are collected; add `--profile` to combine them with a
profile. Every flag that changes the configuration is recorded in the
result (`config.overrides`) and in `config.effective.yaml` in the archive.

### Selection

| Flag | Configuration | Meaning |
|---|---|---|
| `-c, --config file` | – | YAML configuration (`-` reads stdin). |
| `-p, --profile name` | `collect.profile` | `minimal`, `standard` or `full`. |
| `-a, --artifact name` | `collect.artifacts` | Add an artifact or wildcard (`windows.registry.*`). Repeatable. |
| `-x, --exclude-artifact name` | `collect.exclude_artifacts` | Remove an artifact or wildcard. Repeatable. |
| `--path pattern` | `collect.custom_artifacts` | Collect files matching a [path pattern](configuration.md#path-patterns), as artifact `cli.paths`. Relative paths are taken from the working directory. Repeatable. |
| `--exclude-path pattern` | `collect.exclude_paths` | Never collect files matching a pattern. Repeatable. |

### Output and result

| Flag | Configuration | Meaning |
|---|---|---|
| `-o, --output path` | `output.path` | Archive file or directory, see [output path](configuration.md#output-path). |
| `--overwrite` | `output.overwrite` | Replace an existing archive. |
| `--compression mode` | `output.compression` | `auto`, `deflate` or `store`. |
| `--compression-level n` | `output.compression_level` | 1 (fastest) to 9 (smallest). |
| `--dry-run` | – | Resolve patterns and list matches; read nothing, write no archive. |
| `--json` | – | Print the result JSON on stdout (nothing else goes to stdout). |
| `--status-file file` | – | Also write the result JSON to a file (atomically). |

### Job metadata

| Flag | Configuration | Meaning |
|---|---|---|
| `--job-id id` | `job.id` | Case or task ID; in the result, the manifest and the `{job_id}` placeholder. |
| `--job-name name` | `job.name` | Free text. |
| `--label key=value` | `job.labels` | Repeatable, e.g. `--label analyst=jd --label case=IR-42`. |

### Collection and limits

| Flag | Configuration | Example |
|---|---|---|
| `--method method` | `collect.method` | `auto`, `os`, `raw` |
| `--hash algorithms` | `collect.hashes` | `--hash md5,sha1,sha256` |
| `--follow-symlinks` | `collect.follow_symlinks` | |
| `--max-duration duration` | `limits.max_duration` | `30m`, `0` = unlimited |
| `--max-total-size size` | `limits.max_total_size` | `10GiB` |
| `--max-file-size size` | `limits.max_file_size` | `1GiB` |
| `--oversize mode` | `limits.oversize` | `skip`, `truncate` |
| `--max-files number` | `limits.max_files` | `50000` |
| `--min-free-space size` | `limits.min_free_space` | `5GiB`, `0` = off |
| `--read-rate rate` | `limits.read_rate` | `20MiB` (per second) |
| `--priority priority` | `limits.priority` | `low`, `normal` |

### Logging

| Flag | Default | Meaning |
|---|---|---|
| `--log-level level` | `info` | `debug`, `info`, `warn`, `error`. `debug` also applies to `relic.log` in the archive (per-file decisions); lower levels never reduce `relic.log` below `info`. |
| `-v, --verbose` | | Same as `--log-level debug`. |
| `-q, --quiet` | | Console shows errors only and no summary. Does not affect `--log-file` or `relic.log`. |
| `--log-format format` | `text` | Console format on stderr: `text` or `json`. |
| `--log-file file` | | Also write the log as JSON lines with UTC timestamps (appended). Useful when stderr is not kept, and the only log that survives a run without archive (e.g. output volume full). |
| `--progress-interval duration` | `15s` | Minimum time between progress messages (`step`, files, bytes, elapsed time). `0` disables them. |

At the start Relic logs whether it runs elevated, and warns if it does not:
protected files are then reported as unreadable.

### Examples

```bash
# Ad-hoc: only the application logs, throttled, with debug log in a file
relic collect --path '/opt/app/logs/**' --exclude-path '/opt/app/logs/**/*.gz' \
    --read-rate 20MiB --log-file /var/tmp/relic.jsonl -v

# Event logs and IIS logs of a web server, tagged with the case
relic collect -a windows.eventlogs --path 'C:\inetpub\logs\**\*.log' --job-id IR-2026-0042 -o D:\cases\

# Default triage plus all browsers, three hashes per file
relic collect -p standard -a 'browser.*' --hash md5,sha1,sha256
```

## `relic verify`

```bash
relic verify relic_HOST_20261107T215424Z.zip
relic verify out/relic.zip --result out/status.json --json
```

Checks an archive without extracting it: the ZIP structure and the CRC-32 of
every entry; that `manifest.json`, `files.jsonl`, `relic.log` and
`config.effective.yaml` exist; that `config.yaml` matches the configuration
hash in the manifest; that every stored file has a record and matches its
recorded size and hashes; and that the manifest counters match the records. With `--result` (the `--json` output or `--status-file` of the
collection) the archive size and SHA-256 must also match, which proves the
archive is the one the endpoint produced.

Exit codes: `0` verified, `2` problems found or archive unreadable, `4`
invalid usage. With `--json` the report lists every problem.

## `relic list` and `relic show`

```bash
relic list profiles
relic list artifacts --os windows --profile standard
relic list artifacts --search usb             # name, category, description or paths contain all words
relic list artifacts --category persistence --os linux
relic show artifact windows.ntfs.usnjrnl      # YAML, usable as template for custom_artifacts
relic show profile minimal                    # resolved artifacts per platform
```

## `relic validate` and `relic config`

```bash
relic validate job.yaml --os windows --json   # works on any platform
relic config show --profile minimal           # effective configuration with all defaults
relic config schema                           # JSON Schema for generators
```
