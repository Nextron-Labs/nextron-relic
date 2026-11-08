#!/usr/bin/env python3
"""Render the Artefact Collection mockups for docs/management-center-integration.md.

The artefact names, categories, profile membership, path patterns and the
counts derived from them come from a real Relic build. Hosts, sizes, hashes
and timestamps are sample data. The sample result is shaped like pkg/result.

    python3 docs/mockups/build.py                  # all screens
    python3 docs/mockups/build.py --only 03,08     # some screens
    python3 docs/mockups/build.py --html-only DIR  # write HTML, render nothing

Needs Google Chrome (or CHROME=/path/to/chrome), Node 22 or newer, Go or
--relic PATH, and network access for the web fonts and icons. ImageMagick is
optional and reduces the PNG size.
"""
import argparse
import hashlib
import html
import json
import os
import pathlib
import random
import shutil
import subprocess
import sys
import tempfile

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parent.parent
OUT = ROOT / "docs" / "images" / "management-center"
EXAMPLE = ROOT / "examples" / "management-center-windows-triage.yaml"

GIB, MIB, KIB = 1024**3, 1024**2, 1024

# Display order and labels. Relic only has the `category` keys.
CATEGORIES = [
    ("registry", "Registry", "System and per-user hives"),
    ("execution", "Execution evidence", "Prefetch, Amcache, SRUM, shell history"),
    ("logs", "Logs", "Event logs, /var/log, journal"),
    ("persistence", "Persistence", "Tasks, WMI, cron, systemd, PAM"),
    ("file_system", "File system", "$MFT, $LogFile, $UsnJrnl, Recycle Bin"),
    ("accounts", "Accounts", "Users, groups, password hashes"),
    ("user_activity", "User activity", "LNK, jump lists, timeline, notifications"),
    ("browser", "Browsers", "History, downloads, extensions"),
    ("network", "Network", "Hosts, firewall logs, BITS, VPN clients"),
    ("remote_access", "Remote access", "RDP, SSH, RMM tools"),
    ("security", "Security products", "Defender and AV logs"),
    ("devices", "Devices", "USB and device history"),
    ("server", "Server roles", "IIS, Exchange, SQL Server, Veeam"),
    ("containers", "Containers", "Docker, containerd, Kubernetes"),
    ("system", "System", "Packages, kernel, configuration"),
]
CAT_LABEL = {k: v for k, v, _ in CATEGORIES}
CAT_LABEL["custom"] = "Custom"

# Not in the catalog yet: see "What Relic should add" in the guide.
SENSITIVE = {"windows.registry.system", "linux.accounts.shadow", "macos.accounts", "windows.rmm.screenconnect",
             "browser.chrome", "browser.edge", "browser.brave", "browser.firefox", "windows.webcache"}
LARGE = {"windows.ntfs.mft", "windows.ntfs.usnjrnl", "windows.iis_logs", "windows.exchange.logs", "windows.etl", "windows.staging",
         "windows.old", "windows.thumbcache", "windows.rdp_bitmap_cache", "linux.logs.journal", "linux.packages.rpmdb",
         "linux.staging_dirs", "linux.containers.logs", "linux.binaries.local", "linux.boot.images"}

TARGETS = [("HR-PC-017", "windows"), ("HR-PC-021", "windows"), ("FS-02", "windows"), ("WEB-LNX-04", "linux")]
TARGET_OS = ["windows", "linux"]
ADDED = {"windows.rdp_bitmap_cache"}
REMOVED = {"browser.brave"}


def esc(s):
    return html.escape(str(s), quote=False)


def fmt_bytes(n):
    if n >= GIB:
        return f"{n / GIB:.1f} GiB"
    if n >= MIB:
        return f"{n / MIB:.0f} MiB"
    if n >= KIB:
        return f"{n / KIB:.0f} KiB"
    return f"{n} B"


class Relic:
    def __init__(self, path):
        self.cmd = [path] if path else ["go", "run", "./cmd/relic"]

    def json(self, *args):
        out = subprocess.run(self.cmd + list(args), cwd=ROOT, check=False, capture_output=True, text=True)
        return json.loads(out.stdout)


# ---------------------------------------------------------------- components

def icon(name, cls="fa-solid"):
    return f'<i class="{cls} {name}"></i>'


OS_ICON = {"windows": ("fa-windows", "Windows"), "linux": ("fa-linux", "Linux"), "darwin": ("fa-apple", "macOS")}


def os_badge(goos, small=False):
    ic, label = OS_ICON[goos]
    if small:
        return f'<span class="os s">{icon(ic, "fa-brands")}</span>'
    return f'<span class="os">{icon(ic, "fa-brands")} {label}</span>'


def cb(state="off"):
    mark = icon("fa-check") if state.startswith("on") else ""
    return f'<span class="cb {state}">{mark}</span>'


def n(number):
    return f'<span class="n">{number}</span>'


def pill(text, color):
    return f'<span class="pill p-{color}">{text}</span>'


STATUS = {"complete": ("Complete", "green"), "partial": ("Partial", "amber"), "failed": ("Failed", "red"),
          "canceled": ("Canceled", "grey"), "running": ("Running", "blue"), "queued": ("Queued", "dark"),
          "uploading": ("Uploading", "blue")}
ART_STATUS = {"collected": ("Collected", "green"), "partial": ("Partial", "amber"), "failed": ("Failed", "red"),
              "not_found": ("Not found", "dark"), "skipped": ("Skipped", "grey"), "not_applicable": ("Not applicable", "dark")}
FILE_STATUS = {"collected": ("Collected", "green"), "truncated": ("Truncated", "amber"), "skipped": ("Skipped", "grey"),
               "failed": ("Failed", "red"), "aborted": ("Aborted", "red")}


def status_pill(s, table=STATUS):
    text, color = table[s]
    return pill(text, color)


NAV = [("fa-gauge-high", "System Status"), ("fa-server", "Asset Management"), ("fa-table-cells-large", "Scan Control"),
       ("fa-play", "Response Control"), ("fa-bullseye", "Service Control"), ("fa-file-pen", "IOC Management"),
       ("fa-bolt", "Thunderstorm"), ("fa-cubes-stacked", "Evidence Collection"), ("fa-box-archive", "Artefact Collection"),
       ("fa-download", "Downloads"), ("fa-id-card", "Licensing"), ("fa-rotate", "Updates"), ("fa-wrench", "Settings"),
       ("fa-code", "API Documentation")]
SUBS = {"Asset Management": ["Assets", "Asset Requests", "Broker Network"],
        "Artefact Collection": ["Collections", "Group Collections", "Collection Templates"]}


def page(section, sub, crumbs, content, overlay=""):
    nav = []
    for ic, name in NAV:
        is_open = name == section
        new = ' <span class="new">NEW</span>' if name == "Artefact Collection" else ""
        chev = icon("fa-chevron-down" if is_open else "fa-chevron-right") .replace('<i class="', '<i class="ch ')
        nav.append(f'<div class="i{" open" if is_open else ""}">{icon(ic)}<span>{name}{new}</span>{chev}</div>')
        if is_open:
            items = "".join(f'<div class="{"on" if s == sub else ""}">{s}</div>' for s in SUBS[name])
            nav.append(f'<div class="sub">{items}</div>')
    trail = ' <span class="sep">&gt;</span> '.join(crumbs[:-1])
    trail = (trail + ' <span class="sep">&gt;</span> ') if trail else ""
    return f"""<!doctype html><html><head><meta charset="utf-8">
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Roboto+Condensed:wght@300;400;500;700&family=Roboto+Mono:wght@400;500&display=block">
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.2/css/all.min.css">
<link rel="stylesheet" href="mc.css"></head><body>
<aside class="side"><div class="brand"><span class="logo"></span><span class="brand-t"><b>NEXTRON</b><br>MANAGEMENT CENTER</span></div>
<div class="nav">{"".join(nav)}</div></aside>
<header class="top">{icon("fa-bars")}<span class="crumb">{trail}<b>{crumbs[-1]}</b></span>
<span class="r"><span class="mock">MOCKUP &middot; SAMPLE DATA</span><span class="green">{icon("fa-server")}</span><span>{icon("fa-clock-rotate-left")} 60m</span><span>{icon("fa-user")} mwagner</span><span>{icon("fa-moon")}</span></span></header>
<main class="content">{content}</main>{overlay}</body></html>"""


# ---------------------------------------------------------------- selection

class Model:
    """The dialogue's state, derived from the real catalog."""

    def __init__(self, relic):
        self.relic = relic
        self.catalog = relic.json("list", "artifacts", "--json")
        self.by_name = {a["name"]: a for a in self.catalog}
        self.profiles = {p["name"]: p for p in relic.json("list", "profiles", "--json")}
        resolved = relic.json("show", "profile", "standard", "--json")["resolved"]
        self.standard = {o: list(resolved[o]) for o in resolved}
        self.items = [a for a in self.catalog if set(a["os"]) & set(TARGET_OS)]
        base = set(self.standard["windows"]) | set(self.standard["linux"])
        self.selected = (base | ADDED) - REMOVED
        self.base = base

    def count(self, goos, custom=True):
        names = [a for a in self.items if goos in a["os"] and a["name"] in self.selected]
        extra = 1 if custom and goos == "windows" else 0
        return len(names) + extra

    def cat_items(self, key):
        return [a for a in self.items if a["category"] == key]

    def selection_windows(self):
        names = [x for x in self.standard["windows"] if x not in REMOVED]
        return names + sorted(ADDED) + ["custom.ransom_notes"]


def tree_item(m, a, detail=False, marks=None):
    marks = marks or {}
    name = a["name"]
    on = name in m.selected
    tags = []
    if name in ADDED:
        tags.append('<span class="tag add">added</span>')
    if name in REMOVED:
        tags.append('<span class="tag rem">removed</span>')
    if name in LARGE:
        tags.append('<span class="tag large">large</span>')
    if name in SENSITIVE:
        tags.append('<span class="tag sens">sensitive</span>')
    smallest = "Minimal" if "minimal" in a["profiles"] else "Standard" if "standard" in a["profiles"] else "Full"
    tags.append(f'<span class="tag prof">{smallest}</span>')
    oss = "".join(os_badge(o, True) for o in ["windows", "linux", "darwin"] if o in a["os"] and o in TARGET_OS)
    mark = marks.get(name, {})
    row = (f'<div class="tr item">{cb("on" if on else "off")}<code>{esc(name)}</code>{mark.get("name", "")}'
           f'<span class="d ell">{esc(a["description"])}</span>{mark.get("tags", "")}<span class="tags">{"".join(tags)}</span>'
           f'<span class="row" style="gap:4px;flex:none;width:44px;justify-content:flex-end">{oss}</span>'
           f'<span class="ti muted">{icon("fa-circle-info" if not detail else "fa-circle-xmark")}</span></div>')
    if detail:
        d = m.relic.json("show", "artifact", name, "--json")
        src = d["sources"][0]
        pats = "<br>".join(esc(p) for p in src["paths"])
        extra = []
        if src.get("method"):
            extra.append(f'Access method <b>{esc(src["method"])}</b>')
        if src.get("trim_sparse"):
            extra.append("leading sparse region skipped")
        extra = (" &middot; " + " &middot; ".join(extra)) if extra else ""
        row += (f'<div class="tr detail"><div>{mark.get("detail", "")}<span class="muted">Path patterns:</span> <span class="pat">{pats}</span></div>'
                f'<div class="muted" style="margin-top:4px">Selected through template <b>Standard triage</b>{extra}</div></div>')
    return row


def tree(m, expanded=(), detail=None, numbers=None, marks=None, start_at=None):
    numbers = numbers or {}
    rows = []
    started = start_at is None
    for key, label, desc in CATEGORIES:
        started = started or key == start_at
        items = m.cat_items(key)
        if not items or not started:
            continue
        sel = [a for a in items if a["name"] in m.selected]
        state = "on" if len(sel) == len(items) else "ind" if sel else "off"
        oss = []
        for o in TARGET_OS:
            tot = [a for a in items if o in a["os"]]
            if tot:
                s = [a for a in tot if a["name"] in m.selected]
                oss.append(f'<span class="os s">{icon(OS_ICON[o][0], "fa-brands")} {len(s)}/{len(tot)}</span>')
        is_open = key in expanded
        chev = icon("fa-chevron-down" if is_open else "fa-chevron-right")
        num = numbers.get(key, {})
        rows.append(f'<div class="tr"><span class="chev">{chev}</span>{cb(state)}<span class="cname">{label}</span>{num.get("name", "")}'
                    f'<span class="cdesc grow ell">{esc(desc)}</span>{num.get("os", "")}<span class="oss">{"".join(oss)}</span>'
                    f'<span class="cnt">{len(sel)} / {len(items)}</span></div>')
        if is_open:
            rows.extend(tree_item(m, a, a["name"] == detail, marks) for a in items)
    return f'<div class="tree">{"".join(rows)}</div>'


def assets_table(selected=True):
    rows = [("HR-PC-017", "windows", True, "2026-11-02"), ("HR-PC-021", "windows", True, "2026-11-02"),
            ("FS-02", "windows", True, "2026-11-01"), ("WEB-LNX-04", "linux", True, "2026-11-01"),
            ("DC-01", "windows", False, "2026-11-01"), ("APP-SRV-03", "windows", False, "2026-10-29"),
            ("MKT-MAC-02", "darwin", False, "2026-10-30"), ("QA-LNX-11", "linux", False, "2026-10-28"),
            ("HR-PC-030", "windows", False, "2026-10-31"), ("HR-PC-044", "windows", False, "2026-11-02"),
            ("DEV-LNX-07", "linux", False, "2026-10-27"), ("FIN-PC-009", "windows", False, "2026-11-02")]
    body = []
    for host, goos, sel, scan in rows:
        act = (f'<span class="actions">{icon("fa-table-cells-large")}{icon("fa-play")}{icon("fa-box-archive")}'
               f'{icon("fa-terminal")}{icon("fa-folder-tree")}</span>')
        body.append(f'<tr><td>{icon("fa-arrow-right")}</td><td>{cb("on" if sel and selected else "off")}</td><td>{host}</td>'
                    f'<td>{pill("Online", "green")}</td><td>5 days</td><td>2024-03-11</td><td>{os_badge(goos)}</td><td>{scan}</td><td>{act}</td></tr>')
    return ('<div class="card"><div class="h">Assets<span class="grow"></span><span class="btn light">Add Scan</span>'
            f'<span class="btn light">{icon("fa-play")} Add Task</span></div><div class="b">'
            '<table class="t"><colgroup><col style="width:34px"><col style="width:34px"><col style="width:190px"><col style="width:90px"><col style="width:100px"><col style="width:110px"><col style="width:120px"><col style="width:150px"><col></colgroup>'
            '<tr><th></th><th></th><th>Hostname</th><th>Status</th><th>Status Since</th><th>First Seen</th><th>OS</th><th>Last Scan Completed</th><th>Actions</th></tr>'
            '<tr class="s"><td></td><td></td><td>Search</td><td>Search</td><td>Search</td><td>Search</td><td>Search</td><td>Search</td><td></td></tr>'
            + "".join(body) + "</table></div></div>")


def form_row(label, value, extra=""):
    return (f'<div class="row" style="margin-bottom:10px"><div class="lbl" style="width:240px;flex:none">{label}</div>'
            f'<div class="grow">{value}</div>{extra}</div>')


def select(value):
    return f'<div class="field">{value}<span class="caret">{icon("fa-caret-down")}</span></div>'


def screen_add_task(m):
    wins = m.profiles["standard"]["artifacts_per_os"]["windows"]
    lin = m.profiles["standard"]["artifacts_per_os"]["linux"]
    body = "".join([
        form_row('Description <span class="muted">(optional)</span>', '<div class="field">Ransomware triage HR</div>'),
        form_row("Assets", '<div class="field">HR-PC-017, HR-PC-021, FS-02, WEB-LNX-04</div>'),
        form_row("Module", select("Artefact Collection"), n(1)),
        form_row(f'Max. Runtime {icon("fa-circle-info")}', select("3 hours")),
        '<hr style="border:0;border-top:1px solid var(--line);margin:4px 0 12px">',
        form_row("Collection Template", select(f'Standard triage <span class="muted">&nbsp;(default)</span>'), n(2)),
        form_row('Case / Ticket <span class="muted">(optional)</span>', '<div class="field">IR-2026-0042</div>'),
        f'<div class="note i" style="margin-top:2px">{icon("fa-circle-info")}<div>Collects <b>{wins} artefacts</b> on the 3 Windows assets and <b>{lin} artefacts</b> on the Linux asset. '
        'Up to 32 GiB per asset, low priority. Results appear under Artefact Collection &gt; Collections.</div></div>',
    ])
    overlay = (f'<div class="scrim"></div><div class="modal" style="left:300px;top:90px;width:840px">'
               f'<div class="mh">Add Task<span class="x">{icon("fa-xmark")}</span></div><div style="padding:6px 18px 8px">{body}</div>'
               f'<div class="mf" style="border:0;padding-top:4px"><span class="grow"></span>{n(3)}<span class="btn">Customize selection&hellip;</span><span class="btn y">Add Task</span></div></div>')
    return page("Asset Management", "Assets", ["Asset Management", "Assets"], assets_table(), overlay)


def dialog(m, tab, main, summary=None, blocked=""):
    chips = " ".join(f'<span class="row" style="gap:4px">{os_badge(o, True)}{h}</span>' for h, o in TARGETS)
    ctx = (f'<div><div class="lbl">Assets</div><div class="field" style="gap:10px">{chips}</div></div>'
           f'<div><div class="lbl">Collection Template</div>{select("Standard triage <span class=&quot;amber&quot;>&nbsp;(modified)</span>".replace("&quot;", chr(34)))}</div>'
           '<div><div class="lbl">Description</div><div class="field">Ransomware triage HR</div></div>'
           '<div class="row" style="align-items:stretch;gap:12px"><div class="grow"><div class="lbl">Case / Ticket</div><div class="field">IR-2026-0042</div></div>'
           f'<div style="width:108px;flex:none"><div class="lbl">Max. Runtime</div>{select("3 hours")}</div></div>')
    union = len(m.selected)
    tabs = [("Artefacts", union, 1), ("Custom targets", 1, 2), ("Limits &amp; options", None, 3), ("Review", None, 4)]
    tabs_html = ""
    for label, c, i in tabs:
        count = f'<span class="c">{c}</span>' if c is not None else ""
        tabs_html += f'<div class="tab{" on" if i == tab else ""}">{label}{count}</div>'
    side = summary if summary is not None else summary_panel(m)
    side_html = f'<div class="msum">{side}</div>' if side else ""
    prev = '<span class="btn">Back</span>' if tab > 1 else ""
    nxt = '<span class="btn">Next</span>' if tab < 4 else ""
    blocked_note = f'<span class="red" style="font-size:14px;margin-right:6px">{icon("fa-circle-xmark")} {blocked}</span>' if blocked else ""
    return (f'<div class="scrim"></div><div class="modal" style="left:30px;top:18px;width:1380px;height:864px">'
            f'<div class="mh">Add Task <span class="muted" style="font-size:17px">&middot; Artefact Collection</span><span class="x">{icon("fa-xmark")}</span></div>'
            f'<div class="ctx">{ctx}</div><div class="tabs">{tabs_html}</div>'
            f'<div class="mbody"><div class="mmain">{main}</div>{side_html}</div>'
            f'<div class="mf"><span class="btn out">Save as template&hellip;</span><span class="grow"></span>{blocked_note}{prev}{nxt}<span class="btn {"dis" if blocked else "y"}">Add Task</span></div></div>')


def summary_panel(m, number=""):
    return (f'<div class="sec"><h4 class="row" style="gap:6px">Selection {number}</h4>'
            f'<div class="row">{os_badge("windows")}<span class="muted">3 assets</span><span class="grow"></span><b>{m.count("windows")}</b></div>'
            f'<div class="row mt8">{os_badge("linux")}<span class="muted">1 asset</span><span class="grow"></span><b>{m.count("linux")}</b></div></div>'
            '<div class="sec"><h4>Changes to template</h4>'
            '<div class="row"><span class="tag add">added</span><code class="ell">windows.rdp_bitmap_cache</code></div>'
            '<div class="row mt8"><span class="tag rem">removed</span><code class="ell">browser.brave</code></div>'
            '<div class="row mt8"><span class="tag add">custom</span><code class="ell">custom.ransom_notes</code></div></div>'
            '<div class="sec"><h4>Limits</h4><div class="dl" style="grid-template-columns:1fr auto">'
            '<div>Per file</div><div>2 GiB</div><div>Per asset</div><div>16 GiB</div><div>Collection time</div><div>2 h</div><div>Free space kept</div><div>5 GiB</div><div>Hashes</div><div>MD5, SHA1, SHA256</div></div></div>'
            f'<div class="note w">{icon("fa-triangle-exclamation")}<div><b>Sensitive content.</b> The selection includes password hashes and browser history. Access to the evidence is logged.</div></div>')


def screen_categories(m):
    toolbar = (f'<div class="row" style="margin-bottom:10px">{n(1)}<div class="field ph" style="width:360px">{icon("fa-magnifying-glass")} Filter by name, description or path</div>'
               f'<span class="seg"><span class="on">All</span><span>Windows</span><span>Linux</span></span>'
               '<span class="seg"><span class="on">All</span><span>Selected</span><span>Changed</span></span><span class="grow"></span>'
               '<span class="btn sm">Select all</span><span class="btn sm">Clear</span><span class="btn sm">Reset to template</span></div>')
    main = toolbar + tree(m, numbers={"registry": {"name": n(2), "os": n(3)}})
    return page("Asset Management", "Assets", ["Asset Management", "Assets"], assets_table(), dialog(m, 1, main, summary=summary_panel(m, n(4))))


def screen_items(m):
    toolbar = (f'<div class="row" style="margin-bottom:10px"><div class="field ph" style="width:360px">{icon("fa-magnifying-glass")} Filter by name, description or path</div>'
               '<span class="seg"><span class="on">All</span><span>Windows</span><span>Linux</span></span>'
               '<span class="seg"><span class="on">All</span><span>Selected</span><span>Changed</span></span><span class="grow"></span>'
               '<span class="btn sm">Select all</span><span class="btn sm">Clear</span><span class="btn sm">Reset to template</span></div>')
    marks = {"windows.recycle_bin": {"name": n(1)}, "windows.ntfs.mft": {"tags": n(2)}, "windows.ntfs.usnjrnl": {"detail": n(3) + " "},
             "browser.brave": {"tags": n(4)}}
    main = toolbar + tree(m, expanded={"file_system", "browser"}, detail="windows.ntfs.usnjrnl", marks=marks, start_at="file_system")
    return page("Asset Management", "Assets", ["Asset Management", "Assets"], assets_table(), dialog(m, 1, main))


def pattern_row(pattern, ok=True, message=""):
    state = f'<span class="green">{icon("fa-circle-check")}</span>' if ok else f'<span class="red">{icon("fa-circle-xmark")}</span>'
    msg = f'<div class="help red" style="margin:3px 0 0 2px">{esc(message)}</div>' if message else ""
    return (f'<div style="margin-bottom:6px"><div class="field {"ok" if ok else "err"}"><span class="mono grow ell">{esc(pattern)}</span>{state}'
            f'<span class="muted">{icon("fa-trash")}</span></div>{msg}</div>')


def screen_custom(m):
    left = ('<div style="width:196px;flex:none"><div class="lbl" style="margin-bottom:6px">Custom targets</div>'
            '<div class="tree"><div class="tr" style="background:#24465a">' + cb("on") + '<span class="ell">Ransom notes</span></div></div>'
            f'<div class="btn sm mt8">{icon("fa-plus")} Add custom target</div>'
            '<div class="help" style="margin-top:12px">Custom targets are always collected on the operating systems they define paths for.</div></div>')
    variables = " ".join(f'<span class="btn sm mono" style="font-size:12px">{v}</span>' for v in
                         ["%UserHome%", "%SystemDrive%", "%SystemRoot%", "%ProgramData%", "%ProgramFiles%"])
    form = (f'<div class="grow"><div style="display:grid;grid-template-columns:1fr 1.5fr .6fr;gap:10px">'
            '<div><div class="lbl">Name</div><div class="field">Ransom notes</div><div class="help">ID <code>custom.ransom_notes</code></div></div>'
            '<div><div class="lbl">Description</div><div class="field">Ransom notes on the desktop and in public folders</div></div>'
            f'<div><div class="lbl">Category</div>{select("Custom")}</div></div>'
            f'<div class="row" style="margin-top:8px">{n(1)}<span class="seg"><span class="on">{icon("fa-windows", "fa-brands")} Windows (3)</span><span>{icon("fa-linux", "fa-brands")} Linux (0)</span><span>{icon("fa-apple", "fa-brands")} macOS (0)</span></span>'
            f'<span class="grow"></span>{n(2)}<span class="row" style="gap:6px">{cb("off")} Collect before the catalog artefacts</span></div>'
            '<div class="lbl" style="margin-top:10px;margin-bottom:5px">Path patterns</div>'
            + pattern_row(r"%UserHome%\Desktop\*{README,DECRYPT,RECOVER}*.{txt,html,hta}")
            + pattern_row(r"%SystemDrive%\Users\Public\*{README,DECRYPT,RECOVER}*.{txt,html,hta}")
            + pattern_row(r"Users\Public\Documents\*.hta", False,
                          r'pattern "Users\\Public\\Documents\\*.hta" must be absolute and start with a drive letter (e.g. C:\ or %SystemDrive%\)')
            + f'<div class="row mt8">{n(3)}<span class="btn sm">{icon("fa-plus")} Add pattern</span><span class="muted" style="margin-left:8px;font-size:13px">Insert:</span>{variables}</div>'
            f'<div class="help mt8">Syntax: <code>*</code> within one directory, <code>**</code> any depth, <code>?</code> one character, <code>[a-z]</code>, <code>{{a,b}}</code> alternatives, <code>file:stream</code> NTFS stream. Literal % is written <code>%%</code>.</div>'
            f'<div class="row" style="margin-top:10px;gap:14px"><div class="grow"><div class="lbl">Exclude from this target</div><div class="field ph">Pattern, e.g. %UserHome%\\Desktop\\Archive\\**</div></div>'
            f'<div style="width:250px"><div class="lbl">Access method</div>{select("Automatic (OS, then raw NTFS)")}</div></div></div>')
    excl = (f'<div class="pane" style="margin-top:12px"><div class="h">{n(4)} Excluded paths <span class="muted" style="font-weight:400;font-size:13.5px">&nbsp;Never collected, for any artefact</span></div>'
            '<div class="b" style="padding-bottom:6px">'
            + pattern_row(r"%SystemRoot%\System32\winevt\Logs\Microsoft-Windows-Diagnostics-Performance%%4Operational.evtx")
            + f'<span class="btn sm">{icon("fa-plus")} Add exclusion</span></div></div>')
    main = f'<div class="row" style="align-items:flex-start;gap:18px">{left}{form}</div>{excl}'
    return page("Asset Management", "Assets", ["Asset Management", "Assets"], assets_table(), dialog(m, 2, main, blocked="1 invalid pattern"))


def limit_field(label, value, unit, help_text, number=""):
    u = f'<span class="muted" style="margin-left:auto">{unit}</span>' if unit else ""
    return (f'<div style="margin-bottom:10px"><div class="row" style="gap:6px"><span class="lbl">{label}</span>{number}</div>'
            f'<div class="field">{value}{u}</div><div class="help">{help_text}</div></div>')


def screen_limits(m):
    size = (f'<div class="pane"><div class="h">Size and time</div><div class="b">'
            + limit_field("Per file", "2", "GiB", "Larger files are handled as set below.")
            + f'<div style="margin-bottom:10px"><div class="lbl">Files above the limit</div><div class="row mt8" style="gap:14px"><span class="row" style="gap:5px"><span class="rd on"></span>Skip the file</span><span class="row" style="gap:5px"><span class="rd"></span>Store the first 2 GiB</span></div></div>'
            + limit_field("Per asset", "16", "GiB", "What is collected first is kept. Later files that do not fit are skipped.")
            + limit_field("Number of files", "Unlimited", "", "Stops the collection when reached.")
            + limit_field("Collection time", "2 h", "from Max. Runtime", "The archive is finalized when reached. 1 h of the task's 3 h remains for the upload.", n(1))
            + "</div></div>")
    impact = (f'<div class="pane"><div class="h">Impact on the endpoint</div><div class="b">'
              f'<div style="margin-bottom:10px"><div class="lbl">Process priority</div><div class="row mt8" style="gap:14px"><span class="row" style="gap:5px"><span class="rd on"></span>Low (CPU and I/O)</span><span class="row" style="gap:5px"><span class="rd"></span>Normal</span></div></div>'
              + limit_field("Read rate", "Unlimited", "", "Limit, e.g. 20 MiB/s on latency-sensitive servers.")
              + limit_field("Free space to keep", "5", "GiB", "Stops before the output volume runs full.")
              + limit_field("Memory limit", "512", "MiB", "Soft limit for the collector process.")
              + "</div></div>")
    hash_row = "".join(f'<span class="row" style="gap:5px">{cb(state)} {name}</span>' for name, state in
                       [("MD5", "on"), ("SHA1", "on"), ("SHA256", "on off-dim"), ("SHA512", "off")])
    ev = (f'<div class="pane"><div class="h">Evidence</div><div class="b">'
          f'<div style="margin-bottom:12px"><div class="row" style="gap:6px"><span class="lbl">Hashes per file</span>{n(2)}</div><div class="row mt8" style="gap:12px">{hash_row}</div><div class="help">SHA256 stays on: the file list and the hash search use it.</div></div>'
          + f'<div style="margin-bottom:10px"><div class="lbl">File access</div>{select("Automatic (OS, then raw NTFS)")}<div class="help">Locked files fall back to raw NTFS reads on Windows. The method is recorded per file.</div></div>'
          + f'<div class="row" style="gap:6px;margin-bottom:10px">{cb("off")} Follow symbolic links matched by wildcards</div>'
          + limit_field("Recursion depth for **", "32", "", "Maximum directory depth.")
          + f'<div style="margin-bottom:6px"><div class="lbl">Archive compression</div>{select("Automatic (store compressed files)")}</div>'
          + "</div></div>")
    head = (f'<div class="row" style="margin-bottom:10px"><span class="muted">Defaults come from the template. Changed values are saved with this task only.</span>'
            f'<span class="grow"></span><span class="btn sm">Reset to template</span></div>')
    main = head + f'<div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:12px">{size}{impact}{ev}</div>'
    return page("Asset Management", "Assets", ["Asset Management", "Assets"], assets_table(), dialog(m, 3, main))


def yaml_for_display():
    lines = []
    for raw in EXAMPLE.read_text().splitlines():
        if raw.lstrip().startswith("#"):
            continue
        line = esc(raw)
        if ":" in line and not line.lstrip().startswith("- "):
            head, tail = line.split(":", 1)
            line = f'<span class="k">{head}</span>:{tail}'
        lines.append(line)
    return "\n".join(lines)


def screen_review(m, res):
    s = res["summary"]
    preview = [("HR-PC-017", "windows", s["artifacts_selected"], s["files_matched"], s["bytes_collected"], "2 files above 2 GiB", "w"),
               ("HR-PC-021", "windows", 80, 4871, 2 * GIB + 700 * MIB, "Within limits", "ok"),
               ("FS-02", "windows", 80, 61902, 16 * GIB, "Stops at 16 GiB, 1,204 files skipped", "e"),
               ("WEB-LNX-04", "linux", 38, 9120, 4 * GIB + 600 * MIB, "Within limits", "ok")]
    rows = ""
    for host, goos, arts, files, size, note, level in preview:
        cls = {"w": "amber", "e": "red", "ok": "green"}[level]
        ic = {"w": "fa-triangle-exclamation", "e": "fa-circle-exclamation", "ok": "fa-circle-check"}[level]
        rows += (f'<tr><td>{host}</td><td>{os_badge(goos, True)}</td><td class="right">{arts}</td><td class="right">{files:,}</td>'
                 f'<td class="right">{fmt_bytes(size)}</td><td class="{cls}">{icon(ic)} {note}</td></tr>')
    left = (f'<div class="note g">{icon("fa-circle-check")}<div><b>Configuration valid</b> for Windows and Linux (Relic 0.3.0, 2 configurations). No warnings.</div>{n(1)}</div>'
            f'<div class="row mt12"><b>Preview on the endpoints</b>{n(2)}<span class="grow"></span><span class="muted" style="font-size:13px">Dry run, 2 minutes ago. Sizes are upper estimates.</span><span class="btn sm">{icon("fa-rotate")} Run again</span></div>'
            '<table class="t mt8"><colgroup><col style="width:120px"><col style="width:44px"><col style="width:72px"><col style="width:80px"><col style="width:90px"><col></colgroup>'
            '<tr><th>Asset</th><th>OS</th><th class="right">Artefacts</th><th class="right">Files</th><th class="right">Size</th><th>Result with these limits</th></tr>'
            + rows + "</table>"
            f'<div class="note w mt12">{icon("fa-triangle-exclamation")}<div><b>FS-02 reaches the 16 GiB limit.</b> Artefacts later in the collection order lose files. Raise the limit or remove large artefacts.</div></div>'
            f'<div class="note w mt8">{icon("fa-triangle-exclamation")}<div><b>Sensitive content.</b> <code>windows.registry.system</code> and <code>linux.accounts.shadow</code> hold password hashes, <code>browser.*</code> holds browsing history.</div></div>'
            '<div class="row mt12"><b>What will happen</b></div>'
            '<div class="help" style="font-size:13.5px;color:var(--text);line-height:1.5">One task per asset. Each endpoint runs Relic with low priority, keeps 5 GiB free, and stops collecting after 2 h. '
            'The archive is uploaded, verified and listed under <b>Artefact Collection &gt; Collections</b>. Temporary files are removed from the endpoint.</div>')
    right = (f'<div class="pane" style="height:100%"><div class="h">Generated configuration {n(3)}<span class="grow"></span><span class="seg" style="font-weight:400"><span class="on">{icon("fa-windows", "fa-brands")} Windows</span><span>{icon("fa-linux", "fa-brands")} Linux</span></span></div>'
             f'<div class="b" style="overflow:hidden"><div class="help" style="margin:0 0 8px">Read-only, for review. It is generated per task and stored with the evidence.</div><pre class="yaml">{yaml_for_display()}</pre></div></div>')
    main = (f'<div style="display:grid;grid-template-columns:1.02fr 1fr;gap:16px;height:100%">'
            f'<div style="min-width:0">{left}</div><div style="min-width:0;height:100%">{right}</div></div>')
    return page("Asset Management", "Assets", ["Asset Management", "Assets"], assets_table(), dialog(m, 4, main, summary=""))


# ---------------------------------------------------------------- sample result

SAMPLE = {
    # name: (collected files, bytes)
    "windows.registry.system": (14, 412 * MIB), "windows.registry.user": (46, 198 * MIB), "windows.registry.amcache": (5, 24 * MIB),
    "windows.prefetch": (212, 38 * MIB), "windows.eventlogs": (412, 734003200), "windows.scheduled_tasks": (96, 1 * MIB),
    "windows.startup_folders": (6, 18 * KIB), "windows.powershell_history": (3, 412 * KIB), "windows.ntfs.usnjrnl": (1, 612 * MIB),
    "windows.ntfs.logfile": (1, 64 * MIB), "windows.srum": (3, 52 * MIB), "windows.pca": (3, 1 * MIB), "windows.wmi_repository": (6, 38 * MIB),
    "windows.group_policy": (9, 410 * KIB), "windows.recent_files": (684, 92 * MIB), "windows.activities_cache": (6, 18 * MIB),
    "windows.notifications": (6, 3 * MIB), "windows.webcache": (15, 96 * MIB), "windows.recycle_bin": (41, 22 * KIB),
    "windows.setupapi_logs": (3, 6 * MIB), "windows.hosts": (5, 8 * KIB), "windows.firewall_logs": (2, 4 * MIB), "windows.bits": (2, 3 * MIB),
    "windows.defender": (34, 46 * MIB), "windows.remote_access_tools": (8, 31 * MIB), "windows.powershell.transcripts": (2, 96 * KIB),
    "windows.wer": (28, 12 * MIB), "windows.cryptnet_url_cache": (214, 640 * KIB), "windows.notepad": (11, 420 * KIB),
    "windows.office.persistence": (7, 3 * MIB), "windows.rdp_client_files": (1, 3 * KIB), "windows.vpn_clients": (4, 8 * MIB),
    "windows.capability_access": (2, 1 * MIB), "windows.startupinfo": (12, 180 * KIB), "windows.logs.debug": (5, 4 * MIB),
    "browser.chrome": (22, 184 * MIB), "browser.edge": (31, 96 * MIB), "windows.rdp_bitmap_cache": (18, 240 * MIB),
    "custom.ransom_notes": (2, 4 * KIB),
}
# name: (limit skips, failed, policy skips)
SAMPLE_PROBLEMS = {
    "windows.ntfs.mft": (1, 0, 0),
    "windows.webcache": (1, 0, 0),
    "browser.chrome": (0, 1, 0),
    "browser.edge": (0, 0, 8),
    "windows.recent_files": (0, 0, 6),
}
SAMPLE_REASONS = {
    "limit": [("windows.ntfs.mft", r"C:\$MFT", 2348810240, "size 2348810240 exceeds max_file_size 2GiB"),
              ("windows.webcache", r"C:\Users\jbauer\AppData\Local\Microsoft\Windows\WebCache\WebCacheV01.dat", 2264924160, "size 2264924160 exceeds max_file_size 2GiB")],
    "failed": [("browser.chrome", r"C:\Users\jbauer\AppData\Local\Google\Chrome\User Data\Default\Network\Cookies-journal", 41200,
                "open: The process cannot access the file because it is being used by another process.; raw NTFS fallback failed: file not found")],
}


def sample_result(m):
    """Build a result that decodes into pkg/result and whose counters add up."""
    arts = []
    tot = dict(artifacts_selected=0, files_matched=0, files_collected=0, files_truncated=0, files_skipped=0,
               files_failed=0, files_duplicate=12, bytes_collected=0, enumeration_errors=0)
    for name in m.selection_windows():
        files, size = SAMPLE.get(name, (0, 0))
        lim, fail, pol = SAMPLE_PROBLEMS.get(name, (0, 0, 0))
        matched = files + lim + fail + pol
        if matched == 0:
            status = "not_found"
        elif files == 0 and (fail or lim):
            status = "failed"
        elif files == 0:
            status = "not_found"
        elif fail or lim:
            status = "partial"
        else:
            status = "collected"
        by = "custom" if name.startswith("custom.") else "config" if name in ADDED else "profile:standard"
        entry = dict(name=name, status=status, selected_by=by, files_matched=matched, files_collected=files, bytes_collected=size)
        if name.startswith("custom."):
            entry["custom"] = True
        if lim + pol:
            entry["files_skipped"] = lim + pol
        if fail:
            entry["files_failed"] = fail
        arts.append(entry)
        tot["artifacts_selected"] += 1
        tot["files_matched"] += matched
        tot["files_collected"] += files
        tot["files_skipped"] += lim + pol
        tot["files_failed"] += fail
        tot["bytes_collected"] += size
    return {
        "schema_version": 1,
        "tool": {"name": "relic", "version": "0.3.0", "commit": "0278c75afa02", "go_version": "go1.26.10"},
        "status": "partial", "exit_code": 1,
        "status_reason": "1 file(s) failed, 2 file(s) skipped by limits",
        "job": {"id": "mc-task-18342", "name": "Ransomware triage HR",
                "labels": {"case": "IR-2026-0042", "requested_by": "mwagner", "template": "standard-triage"}},
        "host": {"hostname": "HR-PC-017", "os": "windows", "os_version": "Windows 11 Pro 24H2 (build 26100.2314)", "arch": "amd64",
                 "user": "NT AUTHORITY\\SYSTEM", "elevated": True, "time_zone": "W. Europe Standard Time"},
        "started_at": "2026-11-08T07:22:14Z", "finished_at": "2026-11-08T07:27:26.400Z", "duration_seconds": 312.4,
        "config": {"source": "file:C:\\ProgramData\\Nextron\\Relic\\mc-task-18342\\job.yaml",
                   "sha256": hashlib.sha256(b"job.yaml").hexdigest(), "profile": "standard"},
        "output": {"path": "C:\\ProgramData\\Nextron\\Relic\\mc-task-18342\\relic.zip", "format": "zip",
                   "size": int(tot["bytes_collected"] * 0.61), "sha256": hashlib.sha256(b"relic.zip").hexdigest()},
        "summary": tot, "artifacts": arts,
    }


def cat_rollup(m, res):
    custom = {a["name"] for a in res["artifacts"] if a.get("custom")}
    roll = {}
    for a in res["artifacts"]:
        key = "custom" if a["name"] in custom else m.by_name[a["name"]]["category"]
        r = roll.setdefault(key, dict(arts=[], matched=0, files=0, bytes=0, problems=0))
        r["arts"].append(a)
        r["matched"] += a["files_matched"]
        r["files"] += a["files_collected"]
        r["bytes"] += a["bytes_collected"]
    for r in roll.values():
        st = {a["status"] for a in r["arts"]}
        r["status"] = "failed" if "failed" in st and len(st - {"not_found", "failed"}) == 0 else "partial" if st & {"partial", "failed"} else \
            "collected" if "collected" in st else "not_found"
        r["found"] = sum(1 for a in r["arts"] if a["status"] != "not_found")
    return roll


def tabs_row(active, counts):
    labels = [("Overview", None), ("Files", counts["files"]), ("Problems", counts["problems"]), ("Log", None), ("Configuration", None)]
    out = ""
    for label, c in labels:
        count = f'<span class="c">{c:,}</span>' if c is not None else ""
        out += f'<div class="tab{" on" if label == active else ""}">{label}{count}</div>'
    return f'<div class="tabs" style="padding:0 2px">{out}</div>'


def detail_header(res, res_problems):
    o = res["output"]
    return (f'<div class="card"><div class="h" style="padding:9px 14px"><span style="font-size:18px">HR-PC-017</span>{os_badge("windows")}{status_pill("partial")}'
            f'<span class="pill p-green">{icon("fa-circle-check")} Verified</span>'
            f'<span class="muted" style="font-weight:400;font-size:14px;margin-left:6px" >1 file failed, 2 files skipped by limits</span><span class="grow"></span>'
            f'<span class="btn y">{icon("fa-download")} Archive ({fmt_bytes(o["size"])})</span><span class="btn">{icon("fa-file-csv")} File list</span><span class="btn">{icon("fa-rotate-right")} Collect again</span></div>'
            f'<div class="b row" style="padding:7px 14px;gap:22px;font-size:14px"><span><span class="muted">Case</span> IR-2026-0042</span><span><span class="muted">Task</span> mc-task-18342</span>'
            f'<span><span class="muted">Description</span> Ransomware triage HR</span><span><span class="muted">Template</span> Standard triage (modified)</span><span><span class="muted">Requested by</span> mwagner</span>'
            f'<span class="grow"></span><span class="muted">2026-11-08 07:22:14 UTC</span></div></div>')


def screen_overview(m, res):
    s = res["summary"]
    roll = cat_rollup(m, res)
    problems = s["files_failed"] + 2 + s["enumeration_errors"]
    selected = s["artifacts_selected"]
    found = sum(1 for a in res["artifacts"] if a["status"] != "not_found")
    kpis = [("Files collected", f'{s["files_collected"]:,}', f'of {s["files_matched"]:,} matched', ""),
            ("Collected data", fmt_bytes(s["bytes_collected"]), f'archive {fmt_bytes(res["output"]["size"])}', ""),
            ("Artefacts with data", f"{found}", f"of {selected} selected", ""),
            ("Problems", f"{problems}", "1 failed, 2 skipped by limits", "amber"),
            ("Duration", "5m 12s", "limit 2 h", ""),
            ("Hash algorithms", "3", "MD5, SHA1, SHA256", "")]
    tiles = "".join(f'<div class="kpi"><div class="l">{l}</div><div class="v {c}">{v}</div><div class="x">{x}</div></div>' for l, v, x, c in kpis)
    tiles = tiles.replace('<div class="l">Files collected</div>', f'<div class="l row" style="gap:6px">Files collected {n(1)}</div>', 1)
    maxb = max(r["bytes"] for r in roll.values()) or 1
    order = [k for k, _, _ in CATEGORIES if k in roll] + (["custom"] if "custom" in roll else [])
    rows = ""
    for key in order:
        r = roll[key]
        is_open = key == "file_system"
        chev = icon("fa-chevron-down" if is_open else "fa-chevron-right")
        pct = max(1, int(r["bytes"] * 100 / maxb)) if r["bytes"] else 0
        rows += (f'<tr><td class="tc">{chev}</td><td>{CAT_LABEL[key]}</td><td class="right">{r["found"]} / {len(r["arts"])}</td><td class="right">{r["files"]:,}</td>'
                 f'<td class="right">{fmt_bytes(r["bytes"]) if r["bytes"] else "&ndash;"}</td><td><div class="bar"><i style="width:{pct}%"></i></div></td><td>{status_pill(r["status"], ART_STATUS)}</td></tr>')
        if is_open:
            for a in r["arts"]:
                reason = " &nbsp;" + '<span class="muted">too large</span>' if a["name"] == "windows.ntfs.mft" else ""
                rows += (f'<tr class="sub"><td></td><td colspan="2" class="ell"><code>{a["name"]}</code>{reason}</td><td class="right">{a["files_collected"]:,}</td>'
                         f'<td class="right">{fmt_bytes(a["bytes_collected"]) if a["bytes_collected"] else "&ndash;"}</td><td></td><td>{status_pill(a["status"], ART_STATUS)}</td></tr>')
    table = ('<table class="t low" style="font-size:14px"><colgroup><col style="width:28px"><col style="width:170px"><col style="width:72px"><col style="width:62px"><col style="width:78px"><col><col style="width:90px"></colgroup>'
             '<tr><th></th><th>Category</th><th class="right">Found</th><th class="right">Files</th><th class="right">Size</th><th>Share of data</th><th>Status</th></tr>' + rows + "</table>")
    h = res["host"]
    t = res["tool"]
    prov = (f'<div class="pane" style="height:100%"><div class="h">{n(3)} Provenance</div><div class="b"><div class="dl">'
            f'<div>Host</div><div>{h["hostname"]}, {esc(h["os_version"])}, {h["arch"]}</div>'
            f'<div>Ran as</div><div>{esc(h["user"])} <span class="green">{icon("fa-shield-halved")} elevated</span></div>'
            f'<div>Host time zone</div><div>{esc(h["time_zone"])}</div>'
            f'<div>Collector</div><div>Relic {t["version"]} ({t["commit"]})</div>'
            f'<div>Started</div><div>2026-11-08 07:22:14 UTC</div><div>Finished</div><div>2026-11-08 07:27:26 UTC</div>'
            f'<div>Uploaded</div><div>2026-11-08 07:31:02 UTC</div><div>Verified</div><div class="green">2026-11-08 07:31:40 UTC, {s["files_collected"]:,} files, 0 problems</div>'
            f'<div>Archive SHA-256</div><div><code>{res["output"]["sha256"]}</code></div>'
            f'<div>Config SHA-256</div><div><code>{res["config"]["sha256"]}</code></div></div>'
            f'<div class="h5">Not found is normal</div><div class="help" style="margin:0">{selected - found} selected artefacts matched no file on this host, for example RMM tools that are not installed. They are not problems.</div>'
            f'<div class="h5">Limits reached</div><div class="help" style="margin:0">None ended the collection early. 2 files were above the 2 GiB per-file limit.</div></div></div>')
    body = (detail_header(res, problems) + f'<div class="mt12">{tabs_row("Overview", dict(files=s["files_matched"], problems=problems))}</div>'
            f'<div class="kpis mt12" style="grid-template-columns:repeat(6,1fr)">{tiles}</div>'
            f'<div style="display:grid;grid-template-columns:1fr 372px;gap:12px;margin-top:10px;height:566px"><div style="min-width:0;overflow:hidden"><div class="row" style="margin-bottom:6px;gap:6px">{n(2)}<b>Categories and artefacts</b></div>{table}</div>'
            f'<div style="min-width:0">{prov}</div></div>')
    return page("Artefact Collection", "Collections", ["Artefact Collection", "Collections", "HR-PC-017"], body)


def short_path(path, keep=40):
    """Truncate in the middle: the file name is what an analyst looks for."""
    if len(path) <= keep:
        return path
    parts = path.split("\\")
    tail = parts[-1]
    i = len(parts) - 2
    while i > 1 and len(parts[i]) + len(tail) + 1 < keep - 12:
        tail = parts[i] + "\\" + tail
        i -= 1
    return f"{parts[0]}\\{parts[1]}\\&hellip;\\{esc(tail)}"


def fake(path, algo):
    return hashlib.new(algo, path.encode()).hexdigest()


def screen_files(m, res):
    s = res["summary"]
    files = [(r"C:\Windows\System32\config\SYSTEM", "windows.registry.system", 18874368, "2026-11-08 07:14:02", "ntfs_raw", "collected", True),
             (r"C:\Windows\System32\config\SOFTWARE", "windows.registry.system", 94371840, "2026-11-08 07:18:40", "ntfs_raw", "collected", False),
             (r"C:\Windows\System32\config\SAM", "windows.registry.system", 65536, "2026-11-07 09:02:11", "ntfs_raw", "collected", False),
             (r"C:\Users\jbauer\NTUSER.DAT", "windows.registry.user", 3932160, "2026-11-08 06:58:33", "ntfs_raw", "collected", False),
             (r"C:\Windows\appcompat\Programs\Amcache.hve", "windows.registry.amcache", 2097152, "2026-11-08 06:12:50", "ntfs_raw", "collected", False),
             (r"C:\Windows\Prefetch\POWERSHELL.EXE-920BBA2A.pf", "windows.prefetch", 24108, "2026-11-08 06:44:12", "os", "collected", False),
             (r"C:\Windows\Prefetch\RCLONE.EXE-7B2F90C1.pf", "windows.prefetch", 18422, "2026-11-08 05:31:09", "os", "collected", False),
             (r"C:\Windows\System32\winevt\Logs\Security.evtx", "windows.eventlogs", 20975616, "2026-11-08 07:20:44", "ntfs_raw", "collected", False),
             (r"C:\Windows\System32\winevt\Logs\System.evtx", "windows.eventlogs", 8392704, "2026-11-08 07:19:12", "ntfs_raw", "collected", False),
             (r"C:\$MFT", "windows.ntfs.mft", 2348810240, "2026-11-08 07:22:10", "", "skipped", False),
             (r"C:\$Extend\$UsnJrnl:$J", "windows.ntfs.usnjrnl", 641728512, "2026-11-08 07:21:58", "ntfs_raw", "collected", False),
             (r"C:\Users\jbauer\AppData\Roaming\Microsoft\Windows\PowerShell\PSReadLine\ConsoleHost_history.txt", "windows.powershell_history", 18204, "2026-11-08 05:29:41", "os", "collected", False),
             (r"C:\Users\jbauer\AppData\Local\Google\Chrome\User Data\Default\History", "browser.chrome", 3145728, "2026-11-08 05:12:03", "ntfs_raw", "collected", False),
             (r"C:\Users\jbauer\AppData\Local\Google\Chrome\User Data\Default\Network\Cookies-journal", "browser.chrome", 41200, "2026-11-08 07:01:44", "", "failed", False),
             (r"C:\Users\jbauer\Desktop\README_DECRYPT.txt", "custom.ransom_notes", 2184, "2026-11-08 05:47:20", "os", "collected", False)]
    rows = ""
    for path, art, size, mod, method, st, sel in files:
        sha = fake(path, "sha256") if st == "collected" else ""
        short = f'<code>{sha[:12]}&hellip;</code>' if sha else '<span class="dim">&ndash;</span>'
        meth = method.replace("ntfs_raw", "raw") if method else "&ndash;"
        rows += (f'<tr class="{"sel" if sel else ""}"><td class="ell">{short_path(path)}</td><td class="ell"><code>{art}</code></td><td class="right">{fmt_bytes(size)}</td>'
                 f'<td>{short}</td><td>{mod[:16]}</td><td>{meth}</td><td>{status_pill(st, FILE_STATUS)}</td></tr>')
    table = ('<table class="t tight" style="font-size:13.5px"><colgroup><col><col style="width:178px"><col style="width:62px"><col style="width:112px"><col style="width:108px"><col style="width:34px"><col style="width:82px"></colgroup>'
             f'<tr><th>Source path</th><th>Artefact</th><th class="right">Size</th><th><span class="row" style="gap:5px">SHA-256 {n(2)}</span></th><th>Modified (UTC)</th><th>Via</th><th>Status</th></tr>'
             '<tr class="s"><td>Search</td><td>Search</td><td></td><td>Search</td><td>Search</td><td></td><td>All</td></tr>' + rows + "</table>")
    sel = r"C:\Windows\System32\config\SYSTEM"
    pane = (f'<div class="pane" style="height:100%"><div class="h">{n(3)}<span class="ell">SYSTEM</span><span class="grow"></span>{status_pill("collected", FILE_STATUS)}<span class="muted">{icon("fa-xmark")}</span></div><div class="b" style="padding-top:9px">'
            f'<div class="row" style="margin-bottom:8px"><span class="btn sm">{icon("fa-download")} Download file</span><span class="btn sm">{icon("fa-copy")} Copy SHA-256</span><span class="btn sm">{icon("fa-magnifying-glass")} Same hash</span></div>'
            f'<div class="dl sm" style="grid-template-columns:82px 1fr"><div>Source</div><div>{esc(sel)}</div><div>In archive</div><div>files/C/Windows/System32/config/SYSTEM</div><div>Artefact</div><div><code>windows.registry.system</code></div>'
            f'<div>Matched by</div><div class="mono" style="font-size:11px">C:\\Windows\\System32\\config\\{{SAM,SECURITY,SOFTWARE,SYSTEM,DEFAULT}}</div><div>Size</div><div>18,874,368 bytes, all stored</div></div>'
            f'<div class="h5">Hashes of the stored bytes</div><div class="dl sm" style="grid-template-columns:52px 1fr"><div>MD5</div><div><code>{fake(sel, "md5")}</code></div><div>SHA1</div><div><code>{fake(sel, "sha1")}</code></div><div>SHA256</div><div><code>{fake(sel, "sha256")}</code></div></div>'
            f'<div class="h5">Timestamps (UTC)</div><div class="dl sm" style="grid-template-columns:82px 1fr"><div>Modified</div><div>2026-11-08 07:14:02.481</div><div>Accessed</div><div>2026-11-08 07:14:02.481</div><div>Changed</div><div>2026-11-08 07:14:02.481 <span class="muted">(MFT entry)</span></div><div>Created</div><div>2024-08-19 11:03:47.112</div><div>Created (FN)</div><div>2024-08-19 11:03:47.112 <span class="muted">($FILE_NAME)</span></div><div>Collected</div><div>2026-11-08 07:22:26.847</div></div>'
            f'<div class="h5">Access</div><div class="dl sm" style="grid-template-columns:82px 1fr"><div>Method</div><div>Raw NTFS, after an OS error:</div><div></div><div class="amber">open: The process cannot access the file because it is being used by another process.</div><div>MFT entry</div><div class="mono" style="font-size:11.5px">mft:91342-4 &nbsp; -r--r--r--</div></div>'
            "</div></div>")
    filters = (f'<div class="row" style="margin-bottom:8px">{n(1)}{select("All categories")}{select("All statuses")}<div class="field ph" style="width:210px">SHA-256, path or artefact</div>'
               f'<span class="btn sm">{icon("fa-file-csv")} Export CSV</span><span class="grow"></span><span class="muted" style="font-size:13.5px">1 - 25 of {s["files_matched"]:,}</span><span class="btn sm">Show 25</span></div>')
    body = (detail_header(res, 3) + f'<div class="mt12">{tabs_row("Files", dict(files=s["files_matched"], problems=3))}</div>'
            f'<div style="display:grid;grid-template-columns:1fr 352px;gap:12px;margin-top:12px;height:640px"><div style="min-width:0;overflow:hidden">{filters}{table}</div>'
            f'<div style="min-width:0;overflow:hidden">{pane}</div></div>')
    return page("Artefact Collection", "Collections", ["Artefact Collection", "Collections", "HR-PC-017"], body)


def screen_problems(m, res):
    s = res["summary"]
    policy = s["files_skipped"] - 2
    tiles = [("Failed", "1", "could not be read", "red"), ("Skipped by limits", "2", "4.3 GiB not collected", "amber"), ("Truncated", "0", "", ""),
             ("Aborted", "0", "", ""), ("Enumeration errors", "0", "", ""), ("Skipped by policy", str(policy), "links and directories, no action", "dim")]
    kp = "".join(f'<div class="kpi"><div class="l">{l}</div><div class="v {c}">{v}</div><div class="x">{x or "&nbsp;"}</div></div>' for l, v, x, c in tiles)
    groups = (f'<tr><td class="tc">{icon("fa-chevron-down")}</td><td>{pill("Skipped", "grey")}</td><td>File above <b>max. file size</b> (2 GiB)</td><td class="right">2</td><td class="right">4.3 GiB</td>'
              f'<td><span class="btn sm y">{icon("fa-rotate-right")} Collect these files</span></td></tr>')
    gaps = {"windows.ntfs.mft": "No file system timeline", "windows.webcache": "Legacy browser history missing"}
    for art, path, size, reason in SAMPLE_REASONS["limit"]:
        groups += (f'<tr class="sub"><td></td><td colspan="2" class="ell"><span class="mono" style="font-size:12.5px">{esc(path)}</span><br><span class="muted"><code>{art}</code> &middot; {esc(reason)}</span></td>'
                   f'<td class="right"></td><td class="right">{fmt_bytes(size)}</td><td class="muted ell">{gaps[art]}</td></tr>')
    art, path, size, reason = SAMPLE_REASONS["failed"][0]
    groups += (f'<tr><td class="tc">{icon("fa-chevron-down")}</td><td>{pill("Failed", "red")}</td><td>Locked, and the raw NTFS fallback failed</td><td class="right">1</td><td class="right">40 KiB</td>'
               f'<td><span class="btn sm">{icon("fa-rotate-right")} Retry</span></td></tr>')
    groups += (f'<tr class="sub"><td></td><td colspan="2" class="ell"><span class="mono" style="font-size:12.5px">{esc(path)}</span><br><span class="muted"><code>{art}</code> &middot; {esc(reason)}</span></td>'
               f'<td></td><td class="right">{fmt_bytes(size)}</td><td class="muted ell">Temporary file, usually no gap</td></tr>')
    groups += (f'<tr><td class="tc">{icon("fa-chevron-right")}</td><td>{pill("Policy", "dark")}</td><td class="muted">Symbolic link not followed <span class="dim">(8)</span>, directory matched by a pattern <span class="dim">(6)</span></td><td class="right muted">{policy}</td><td class="right dim">&ndash;</td>'
               f'<td class="muted">No action needed</td></tr>')
    groups_tbl = ('<table class="t"><colgroup><col style="width:28px"><col style="width:84px"><col><col style="width:64px"><col style="width:84px"><col style="width:190px"></colgroup>'
                  '<tr><th></th><th>Type</th><th>Reason</th><th class="right">Files</th><th class="right">Size</th><th>Next step</th></tr>' + groups + "</table>")
    gap = (f'<div class="note w">{icon("fa-triangle-exclamation")}<div><b>Evidence gap in the file system category.</b> <code>windows.ntfs.mft</code> has no data because <code>$MFT</code> exceeded the per-file limit. '
           'Timeline analysis of this host is limited until it is collected.</div></div>')
    run = (f'<div class="pane mt12"><div class="h">{n(3)} Run-level messages</div><div class="b"><div class="dl" style="grid-template-columns:150px 1fr"><div>Limits reached</div><div>None ended the collection early</div>'
           f'<div>Run errors</div><div>None</div><div>Configuration issues</div><div>None</div><div>Elevation</div><div class="green">Elevated, protected files were readable</div>'
           f'<div>Changed while reading</div><div>38 files <span class="muted">(live system; hash matches the stored bytes, not necessarily the file now)</span> &nbsp;<span class="blue">Show files</span></div></div></div></div>')
    body = (detail_header(res, 3) + f'<div class="mt12">{tabs_row("Problems", dict(files=s["files_matched"], problems=3))}</div>'
            f'<div class="row mt12" style="gap:6px;margin-bottom:6px">{n(1)}<b>What is missing</b></div><div class="kpis" style="grid-template-columns:repeat(6,1fr)">{kp}</div>'
            f'<div class="mt12">{gap}</div>'
            f'<div class="row mt12" style="gap:6px;margin-bottom:6px">{n(2)}<b>Grouped by reason</b></div>{groups_tbl}{run}')
    return page("Artefact Collection", "Collections", ["Artefact Collection", "Collections", "HR-PC-017"], body)


def screen_list(m, res):
    s = res["summary"]
    data = [("HR-PC-017", "windows", "partial", "IR-2026-0042", "Standard triage*", "07:22", "5m 12s", f'{s["files_collected"]:,} / {s["files_matched"]:,}', fmt_bytes(s["bytes_collected"]), fmt_bytes(res["output"]["size"]), 3, "ok"),
            ("HR-PC-021", "windows", "complete", "IR-2026-0042", "Standard triage*", "07:22", "4m 41s", "4,871 / 4,871", "2.7 GiB", "1.6 GiB", 0, "ok"),
            ("WEB-LNX-04", "linux", "complete", "IR-2026-0042", "Standard triage*", "07:22", "3m 08s", "9,120 / 9,120", "4.6 GiB", "1.1 GiB", 0, "ok"),
            ("FS-02", "windows", "running", "IR-2026-0042", "Standard triage*", "07:22", "26m 40s", "", "", "", None, "wait"),
            ("DC-01", "windows", "uploading", "IR-2026-0042", "Standard triage*", "06:50", "18m 02s", "8,412 / 8,412", "9.4 GiB", "5.8 GiB", 0, "wait"),
            ("HR-PC-030", "windows", "queued", "IR-2026-0042", "Standard triage*", "", "", "", "", "", None, "none"),
            ("APP-SRV-03", "windows", "failed", "IR-2026-0039", "Minimal triage", "11-07 16:02", "0m 04s", "0 / 0", "&ndash;", "&ndash;", 1, "none"),
            ("MKT-MAC-02", "darwin", "partial", "IR-2026-0039", "Standard triage", "11-07 15:40", "7m 55s", "2,204 / 2,391", "1.9 GiB", "1.1 GiB", 187, "ok"),
            ("QA-LNX-11", "linux", "canceled", "", "Web server logs", "11-07 11:15", "2m 10s", "812 / 1,640", "640 MiB", "210 MiB", 0, "ok"),
            ("HR-PC-044", "windows", "complete", "IR-2026-0031", "Standard triage", "11-05 09:30", "6m 20s", "5,540 / 5,540", "3.4 GiB", "2.0 GiB", 0, "bad"),
            ("FIN-PC-009", "windows", "complete", "", "Minimal triage", "11-04 14:12", "1m 47s", "802 / 802", "1.2 GiB", "480 MiB", 0, "ok"),
            ("DEV-LNX-07", "linux", "complete", "", "Web server logs", "11-04 10:02", "0m 58s", "1,120 / 1,120", "410 MiB", "96 MiB", 0, "ok")]
    rows = ""
    for host, goos, st, case, tmpl, start, dur, files, col, arch, prob, ver in data:
        v = {"ok": f'<span class="green">{icon("fa-circle-check")} Verified</span>', "wait": '<span class="dim">Pending</span>',
             "bad": f'<span class="red">{icon("fa-circle-xmark")} Mismatch</span>', "none": '<span class="dim">&ndash;</span>'}[ver]
        pr = '<span class="dim">&ndash;</span>' if prob in (0, None) else f'<span class="amber bold">{prob}</span>' if st != "failed" else f'<span class="red bold">{prob}</span>'
        if st == "running":
            files_c = '<div class="row" style="gap:6px"><div class="bar grow"><i style="width:42%"></i></div><span style="font-size:13px" class="muted">step 34/80</span></div>'
            col = "6.1 GiB"
        else:
            files_c = files
        act = f'<span class="actions">{icon("fa-eye")}{icon("fa-download") if arch not in ("", "&ndash;") else ""}{icon("fa-rotate-right")}</span>'
        if st in ("running", "queued"):
            act = f'<span class="actions">{icon("fa-eye") if st == "running" else ""}{icon("fa-stop")}</span>'
        rows += (f'<tr><td class="tc">{icon("fa-arrow-right")}</td><td>{host}</td><td class="tc">{os_badge(goos, True)}</td><td>{status_pill(st)}</td><td>{case or "<span class=dim>&ndash;</span>"}</td><td class="ell">{tmpl}</td><td class="nowrap">{start}</td><td>{dur}</td>'
                 f'<td>{files_c}</td><td class="right">{col}</td><td class="right">{arch}</td><td class="center">{pr}</td><td>{v}</td><td>{act}</td></tr>')
    table = ('<table class="t" style="font-size:14px"><colgroup><col style="width:28px"><col style="width:104px"><col style="width:36px"><col style="width:92px"><col style="width:96px"><col style="width:112px"><col style="width:90px"><col style="width:68px"><col style="width:118px"><col style="width:66px"><col style="width:68px"><col style="width:50px"><col style="width:96px"><col></colgroup>'
             '<tr><th></th><th>Hostname</th><th>OS</th><th>Status</th><th>Case</th><th>Template</th><th>Started</th><th>Time</th><th>Files</th><th class="right">Data</th><th class="right">Archive</th><th class="center">Issues</th><th>Verification</th><th>Actions</th></tr>'
             '<tr class="s"><td></td><td>Search</td><td></td><td>All</td><td>Search</td><td>Search</td><td>Search</td><td></td><td></td><td></td><td></td><td></td><td>All</td><td></td></tr>' + rows + "</table>")
    kp = [("Collections", "38", "last 7 days"), ("Running or queued", "3", "1 uploading"), ("Complete", "27", ""), ("Partial or failed", "7", "needs review"), ("Evidence stored", "46.2 GiB", "of 2 TiB quota")]
    tiles = "".join(f'<div class="kpi"><div class="l">{l}</div><div class="v">{v}</div><div class="x">{x or "&nbsp;"}</div></div>' for l, v, x in kp)
    body = (f'<div class="kpis" style="grid-template-columns:repeat(5,1fr);margin-bottom:12px">{tiles}</div>'
            f'<div class="card"><div class="h">Collections<span class="grow"></span><span class="btn light">{icon("fa-plus")} New Collection</span></div><div class="b">'
            f'<div class="row" style="margin-bottom:10px"><div class="field ph grow">{icon("fa-magnifying-glass")} Search hostname, case, task ID or file hash across all collections</div><span class="muted" style="font-size:14px;margin-left:8px">1 - 12 of 38</span><span class="btn sm">Show 25</span><span class="btn sm">{icon("fa-rotate-right")} 5s</span></div>{table}</div></div>')
    return page("Artefact Collection", "Collections", ["Artefact Collection", "Collections"], body)


def screen_group(m, res):
    random.seed(18342)
    hosts = [("HR-PC-017", "windows", "partial", 0), ("HR-PC-021", "windows", "complete", 2.7), ("HR-PC-023", "windows", "complete", 3.1),
             ("HR-PC-025", "windows", "complete", 2.4), ("HR-PC-028", "windows", "complete", 3.6), ("HR-PC-030", "windows", "queued", 0),
             ("FS-02", "windows", "running", 6.1), ("DC-01", "windows", "uploading", 9.4), ("APP-SRV-05", "windows", "failed", 0),
             ("WEB-LNX-04", "linux", "complete", 4.6), ("WEB-LNX-05", "linux", "complete", 4.2), ("WEB-LNX-06", "linux", "partial", 3.3)]
    cols = ["registry", "execution", "logs", "persistence", "file_system", "accounts", "user_activity", "browser", "network", "remote_access", "security"]
    short = {"registry": "Registry", "execution": "Execution", "logs": "Logs", "persistence": "Persist.", "file_system": "File sys.", "accounts": "Accounts",
             "user_activity": "Activity", "browser": "Browsers", "network": "Network", "remote_access": "Remote", "security": "Security"}
    roll = cat_rollup(m, res)
    rows = ""
    grand = 0
    for host, goos, st, gib in hosts:
        avail = [c for c in cols if any(c == a["category"] and goos in a["os"] for a in m.items)]
        if st == "running":
            avail_sized = avail[:4]
        else:
            avail_sized = avail
        weights = {c: random.uniform(.15, 1) for c in avail_sized}
        scale = gib * GIB / (sum(weights.values()) or 1)
        cells = ""
        total = 0
        for c in cols:
            if c not in avail:
                cells += '<td style="padding:3px"><div class="cell x">&ndash;</div></td>'
                continue
            if host == "HR-PC-017":
                r = roll.get(c)
                size = r["bytes"] if r else 0
                cls = {"collected": "c", "partial": "p", "failed": "f", "not_found": "nf"}[r["status"]] if r else "nf"
            elif st in ("queued", "failed"):
                size, cls = 0, "x"
            elif st == "running" and c not in avail_sized:
                size, cls = 0, "r" if c == avail[4] else "x"
            else:
                size, cls = int(weights[c] * scale), "c"
                if host == "WEB-LNX-06" and c == "logs":
                    cls = "p"
            total += size
            text = fmt_bytes(size).replace(" ", "&nbsp;") if size else {"x": "&ndash;", "r": "&hellip;", "nf": "none"}[cls]
            cells += f'<td style="padding:3px"><div class="cell {cls}">{text}</div></td>'
        grand += total
        rows += (f'<tr><td>{host}</td><td class="tc">{os_badge(goos, True)}</td><td>{status_pill(st)}</td>{cells}<td class="right">{fmt_bytes(total) if total else "&ndash;"}</td></tr>')
    heads = "".join(f'<th class="center">{short[c]}</th>' for c in cols)
    colw = "".join('<col style="width:70px">' for _ in cols)
    table = (f'<table class="t" style="font-size:14px"><colgroup><col style="width:104px"><col style="width:36px"><col style="width:90px">{colw}<col></colgroup>'
             f'<tr><th>Hostname</th><th>OS</th><th>Status</th>{heads}<th class="right">Data</th></tr>{rows}</table>')
    legend = (f'<div class="row mt12" style="gap:14px;font-size:13.5px"><span class="muted">Legend</span>'
              + "".join(f'<span class="row" style="gap:5px"><span class="cell {c}" style="width:32px;height:16px;padding:0"></span>{t}</span>' for c, t in
                        [("c", "Collected"), ("p", "Partial"), ("f", "Failed"), ("nf", "No matching file"), ("r", "In progress"), ("x", "Not applicable or not started")]) + "</div>")
    kp = [("Target assets", "12", "label department=hr"), ("Finished", "8", "6 complete, 2 partial"), ("In progress", "3", "1 running, 1 uploading, 1 queued"), ("Failed", "1", "APP-SRV-05: no free space"), ("Data collected", fmt_bytes(grand), "rate: 1 per minute")]
    tiles = "".join(f'<div class="kpi"><div class="l">{l}</div><div class="v">{v}</div><div class="x">{x}</div></div>' for l, v, x in kp)
    body = (f'<div class="card"><div class="h"><span style="font-size:17px">Ransomware triage HR</span> {pill("Running", "blue")}<span class="muted" style="font-weight:400;font-size:14px;margin-left:8px">Case IR-2026-0042 &middot; Template Standard triage (modified) &middot; started 2026-11-08 06:50 UTC by mwagner</span>'
            f'<span class="grow"></span><span class="btn">{icon("fa-file-csv")} Export hash list</span><span class="btn">{icon("fa-download")} All archives</span><span class="btn">{icon("fa-stop")} Stop</span></div></div>'
            f'<div class="kpis mt12" style="grid-template-columns:repeat(5,1fr)">{tiles}</div>'
            f'<div class="card mt12"><div class="h">Coverage by asset and category <span class="muted" style="font-weight:400;font-size:13.5px;margin-left:8px">Which evidence exists where, and where it is missing</span></div><div class="b">{table}{legend}</div></div>')
    return page("Artefact Collection", "Group Collections", ["Artefact Collection", "Group Collections", "Ransomware triage HR"], body)


# ---------------------------------------------------------------- rendering

def chrome_path():
    env = os.environ.get("CHROME")
    if env:
        return env
    for c in ["/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", shutil.which("google-chrome"),
              shutil.which("chromium"), shutil.which("chromium-browser")]:
        if c and os.path.exists(c):
            return c
    sys.exit("Google Chrome not found; set CHROME=/path/to/chrome")


def render(pairs):
    """Screenshot [(html, png), ...] and optionally reduce the PNG size."""
    args = [str(x) for pair in pairs for x in pair]
    subprocess.run(["node", str(HERE / "shot.mjs"), chrome_path(), *args], check=True, timeout=300)
    magick = shutil.which("magick")
    if magick:
        for _, png in pairs:
            subprocess.run([magick, str(png), "-strip", "-colors", "200", "-define", "png:compression-level=9", f"PNG8:{png}"], check=True)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--relic", help="path to a relic binary (default: go run ./cmd/relic)")
    ap.add_argument("--only", help="comma-separated screen numbers, e.g. 03,08")
    ap.add_argument("--html-only", metavar="DIR", help="write the HTML files and the sample result to DIR, render nothing")
    args = ap.parse_args()

    model = Model(Relic(args.relic))
    res = sample_result(model)
    screens = [("01", "add-task", lambda: screen_add_task(model)),
               ("02", "select-categories", lambda: screen_categories(model)),
               ("03", "select-items", lambda: screen_items(model)),
               ("04", "custom-targets", lambda: screen_custom(model)),
               ("05", "limits-options", lambda: screen_limits(model)),
               ("06", "review", lambda: screen_review(model, res)),
               ("07", "collections", lambda: screen_list(model, res)),
               ("08", "detail-overview", lambda: screen_overview(model, res)),
               ("09", "detail-files", lambda: screen_files(model, res)),
               ("10", "detail-problems", lambda: screen_problems(model, res)),
               ("11", "group-coverage", lambda: screen_group(model, res))]
    only = set(args.only.split(",")) if args.only else None
    work = pathlib.Path(args.html_only or tempfile.mkdtemp(prefix="relic-mockups-")).resolve()
    work.mkdir(parents=True, exist_ok=True)
    shutil.copy(HERE / "mc.css", work / "mc.css")
    (work / "sample-result.json").write_text(json.dumps(res, indent=2))
    OUT.mkdir(parents=True, exist_ok=True)
    pairs = []
    for num, name, fn in screens:
        if only and num not in only:
            continue
        page_file = work / f"{num}-{name}.html"
        page_file.write_text(fn())
        pairs.append((page_file, OUT / f"{num}-{name}.png"))
    if not args.html_only:
        render(pairs)
        for _, png in pairs:
            print(png.name, png.stat().st_size // 1024, "KiB")
    print("HTML:", work)


if __name__ == "__main__":
    main()
