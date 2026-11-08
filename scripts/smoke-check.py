#!/usr/bin/env python3
"""Verify the result and archive of a Relic smoke collection.

    smoke-check.py result.json --exit-code 0 [--expect-version 0.1.1] [--require-raw-hive]

Fails (exit 1) unless:
- the result is complete or partial and matches the process exit code,
- the archive exists and its size and SHA-256 match the result,
- the archive contains manifest.json, files.jsonl, relic.log and config files,
  and config.yaml matches the configuration hash in the manifest (both are
  absent for built-in runs of 0.2.0 and earlier),
- every collected file in the archive matches its recorded SHA-256 and size,
- at least one file was collected,
- (optional) the tool version matches,
- (optional, Windows) the locked SYSTEM registry hive was collected through
  raw NTFS access and starts with the "regf" signature.
"""
import argparse
import hashlib
import json
import os
import sys
import zipfile


def fail(msg):
    print(f"SMOKE CHECK FAILED: {msg}", file=sys.stderr)
    sys.exit(1)


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("result")
    ap.add_argument("--exit-code", type=int, required=True)
    ap.add_argument("--expect-version")
    ap.add_argument("--require-raw-hive", action="store_true")
    a = ap.parse_args()

    with open(a.result, encoding="utf-8") as f:
        r = json.load(f)

    status, code = r.get("status"), r.get("exit_code")
    if status not in ("complete", "partial"):
        fail(f"status {status!r}: {r.get('status_reason')}")
    if code != a.exit_code or code != {"complete": 0, "partial": 1}[status]:
        fail(f"exit code mismatch: process {a.exit_code}, result {code}, status {status}")
    if a.expect_version and r["tool"]["version"] != a.expect_version:
        fail(f"tool version {r['tool']['version']!r} != {a.expect_version!r}")

    out = r.get("output") or {}
    path = out.get("path")
    if not path or not os.path.isfile(path):
        fail(f"archive {path!r} does not exist")
    if os.path.exists(path + ".partial"):
        fail("a .partial file was left behind")
    if os.path.getsize(path) != out.get("size") or sha256_file(path) != out.get("sha256"):
        fail("archive size/sha256 differ from the result")

    with zipfile.ZipFile(path) as z:
        names = set(z.namelist())
        for required in ("manifest.json", "files.jsonl", "relic.log", "config.effective.yaml"):
            if required not in names:
                fail(f"archive misses {required}")
        manifest = json.loads(z.read("manifest.json"))
        # Since 0.3.0 every run records the hash of its input configuration and
        # archives it as config.yaml; 0.2.0 and earlier have neither for
        # built-in runs (re-testing published releases must still pass).
        want_sha = (manifest.get("config") or {}).get("sha256") or ""
        if want_sha or "config.yaml" in names:
            if "config.yaml" not in names:
                fail("archive misses config.yaml although the manifest records its sha256")
            config_sha = hashlib.sha256(z.read("config.yaml")).hexdigest()
            if config_sha != want_sha:
                fail(f"config.yaml sha256 {config_sha} does not match the manifest ({want_sha!r})")
        if manifest.get("status") != status:
            fail(f"manifest status {manifest.get('status')!r} != {status!r}")
        records = [json.loads(line) for line in z.read("files.jsonl").splitlines() if line.strip()]
        collected = [x for x in records if x["status"] == "collected"]
        if not collected:
            fail("no file was collected")
        for x in collected:
            data = z.read(x["archive_path"])
            if len(data) != x["bytes_stored"] or hashlib.sha256(data).hexdigest() != x["hashes"]["sha256"]:
                fail(f"hash/size mismatch for {x['source_path']}")

        hive = None
        if a.require_raw_hive:
            for x in records:
                if x["source_path"].lower().endswith("\\system32\\config\\system"):
                    hive = x
                    break
            if hive is None:
                fail("no record for the SYSTEM registry hive")
            if hive["status"] != "collected" or hive.get("method") != "ntfs_raw":
                fail(f"SYSTEM hive not collected via raw NTFS: {hive['status']} {hive.get('method')} {hive.get('reason')}")
            if not z.read(hive["archive_path"]).startswith(b"regf"):
                fail("SYSTEM hive does not start with 'regf'")

    s = r["summary"]
    print(f"smoke check ok: {r['tool']['version']} ({r['tool']['commit']}, {r['tool']['go_version']}) "
          f"on {r['host']['os_version']} {r['host']['arch']} elevated={r['host']['elevated']}")
    print(f"  status={status} files={s['files_collected']} bytes={s['bytes_collected']} "
          f"failed={s['files_failed']} skipped={s['files_skipped']} reason={r.get('status_reason', '')!r}")
    for art in r["artifacts"]:
        print(f"  {art['name']:<34} {art['status']:<14} {art['files_collected']}")
    if hive:
        print(f"  SYSTEM hive: {hive['method']}, {hive['bytes_stored']} bytes, fallback: {hive.get('fallback_reason', '')}")


if __name__ == "__main__":
    main()
