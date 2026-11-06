#!/usr/bin/env python3
"""Generate docs/artifacts.md from the built-in catalog.

Usage: go run ./cmd/relic list artifacts --json | python3 scripts/gen-artifact-docs.py > docs/artifacts.md
"""
import json
import sys

rows = json.load(sys.stdin)
osname = {"windows": "Windows", "linux": "Linux", "darwin": "macOS"}
print("# Built-in artifacts\n")
print("Generated from the embedded catalog (`internal/catalog/data/*.yaml`) with")
print("`relic list artifacts --json | python3 scripts/gen-artifact-docs.py`. Use")
print("`relic show artifact <name>` for the exact path patterns.\n")
print("Profiles are cumulative: `standard` contains `minimal`, `full` contains everything.\n")
groups = [("Windows", lambda r: r["name"].startswith("windows.")),
          ("Linux", lambda r: r["name"].startswith("linux.")),
          ("Linux and macOS", lambda r: r["name"].startswith("unix.")),
          ("macOS", lambda r: r["name"].startswith("macos.")),
          ("Browsers (all platforms)", lambda r: r["name"].startswith("browser."))]
for title, pred in groups:
    sel = [r for r in rows if pred(r)]
    if not sel:
        continue
    print(f"## {title}\n")
    print("| Artifact | Platforms | Smallest profile | Description |")
    print("|---|---|---|---|")
    for r in sel:
        prof = r["profiles"][0] if r["profiles"] else "-"
        plats = ", ".join(osname[o] for o in r["os"])
        print(f"| `{r['name']}` | {plats} | {prof} | {r['description']} |")
    print()
