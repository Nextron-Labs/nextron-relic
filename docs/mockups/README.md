# Mockups of the Artefact Collection component

Source of the screenshots in
[`docs/images/management-center`](../images/management-center), used by the
[Management Center integration guide](../management-center-integration.md). They
show a design proposal, not an existing UI.

```bash
python3 docs/mockups/build.py                   # all screens
python3 docs/mockups/build.py --only 03,08      # some screens
python3 docs/mockups/build.py --html-only out/  # write HTML to open in a browser
```

Needs Google Chrome (or `CHROME=/path/to/chrome`), Node 22 or newer, Go (or
`--relic path/to/relic`) and network access for the web fonts and icons.
ImageMagick is optional and reduces the file size.

| File | Purpose |
|---|---|
| `build.py` | Screens and sample data, rendered to `docs/images/management-center` |
| `mc.css` | Look of the Management Center, dark theme |
| `shot.mjs` | Takes the screenshots through the Chrome DevTools protocol |

**Real data:** artefact names, categories, profile membership, path patterns and
the counts derived from them come from the Relic build that is passed in, so
re-render after catalog changes. The configuration on the review screen is
[`examples/management-center-windows-triage.yaml`](../../examples/management-center-windows-triage.yaml).

**Sample data:** hosts, sizes, hashes and timestamps. The sample result has the
same structure as `pkg/result` and its counters add up. `--html-only` also writes it
as `sample-result.json`.

The `large` and `sensitive` tags come from two sets in `build.py`, because the
catalog has no such fields yet.
