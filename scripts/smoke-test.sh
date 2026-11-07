#!/usr/bin/env bash
# Smoke test of a built Relic binary on the current machine (used by
# .github/workflows/smoke.yml, also usable by hand):
#
#   scripts/smoke-test.sh <dist-dir> <platform-suffix>
#   scripts/smoke-test.sh dist windows-amd64.exe
#
# Environment:
#   EXPECTED_VERSION  version the binary must report (e.g. 0.1.1); optional
#   SMOKE_SUDO=true   run the collection with sudo (Linux/macOS)
#   SMOKE_RAW=true    require the locked SYSTEM hive to be read via raw NTFS
#
# Checks: checksum from SHA256SUMS, version stamp, info commands, example
# configurations, a dry run of the "full" profile without pattern errors, a
# "minimal" collection (exit code 0 or 1) and the archive
# it produced (scripts/smoke-check.py independently, and "relic verify").
set -euo pipefail

dist="${1:?usage: smoke-test.sh <dist-dir> <platform-suffix>}"
suffix="${2:?usage: smoke-test.sh <dist-dir> <platform-suffix>}"
here="$(cd "$(dirname "$0")" && pwd)"

shopt -s nullglob
bins=("$dist"/relic-*-"$suffix")
if [ "${#bins[@]}" -ne 1 ]; then
	echo "expected exactly one binary for $suffix in $dist, found: ${bins[*]:-none}" >&2
	exit 1
fi
bin="${bins[0]}"
name="$(basename "$bin")"
echo "== $name"

# Checksum of exactly this file.
line="$(grep -E "[ *]${name}\$" "$dist/SHA256SUMS" || true)"
[ -n "$line" ] || { echo "$name is not listed in SHA256SUMS" >&2; exit 1; }
want="${line%% *}"
if command -v sha256sum >/dev/null; then
	have="$(sha256sum "$bin" | cut -d' ' -f1)"
else
	have="$(shasum -a 256 "$bin" | cut -d' ' -f1)"
fi
[ "$want" = "$have" ] || { echo "checksum mismatch for $name: $have != $want" >&2; exit 1; }
echo "checksum ok"

chmod +x "$bin"
out="$("$bin" version)"
echo "$out"
if [ -n "${EXPECTED_VERSION:-}" ]; then
	case "$out" in
	"relic ${EXPECTED_VERSION} "*) ;;
	*) echo "binary reports '$out', expected version ${EXPECTED_VERSION}" >&2; exit 1 ;;
	esac
fi

"$bin" list profiles >/dev/null
"$bin" list artifacts --json >/dev/null
"$bin" config schema >/dev/null
for f in "$here"/../examples/*.yaml; do
	os=linux
	case "$(basename "$f")" in *windows*) os=windows ;; *macos*) os=darwin ;; esac
	"$bin" validate "$f" --os "$os"
done

# Resolve every pattern of the full catalog (dry run: nothing is read). Any
# run-level error (pattern or variable error) fails the smoke test.
dry=0
"$bin" collect --profile full --dry-run --json -q --max-duration 10m > dryrun.json || dry=$?
echo "full dry run exit code: $dry"
if [ "$dry" -gt 1 ]; then
	head -c 4000 dryrun.json >&2 || true
	exit 1
fi
py="$(command -v python3 || command -v python)"
"$py" - dryrun.json <<'PY'
import json, sys
r = json.load(open(sys.argv[1], encoding="utf-8"))
errs = r.get("errors") or []
if errs:
    print("full dry run reported errors:", *errs, sep="\n  ", file=sys.stderr)
    sys.exit(1)
s = r["summary"]
print(f"full dry run ok: {s['artifacts_selected']} artifacts, {s['files_matched']} files matched, "
      f"{s['enumeration_errors']} enumeration errors")
PY

rm -rf smoke-out result.json
cmd=("$bin" collect --profile minimal --output smoke-out/ --json -q --max-duration 15m)
code=0
if [ "${SMOKE_SUDO:-}" = true ]; then
	sudo "${cmd[@]}" > result.json || code=$?
	sudo chown -R "$(id -u):$(id -g)" smoke-out result.json
else
	"${cmd[@]}" > result.json || code=$?
fi
echo "collect exit code: $code"
if [ "$code" -gt 1 ]; then
	head -c 4000 result.json >&2 || true
	exit 1
fi

args=(result.json --exit-code "$code")
[ -n "${EXPECTED_VERSION:-}" ] && args+=(--expect-version "$EXPECTED_VERSION")
[ "${SMOKE_RAW:-}" = true ] && args+=(--require-raw-hive)
"$py" "$here/smoke-check.py" "${args[@]}"

archive="$("$py" -c 'import json,sys; print(json.load(open(sys.argv[1]))["output"]["path"])' result.json)"
"$bin" verify "$archive" --result result.json
