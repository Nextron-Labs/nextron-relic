# macOS artifact gaps in the Relic catalog (research report)

> Research notes from 2026-11-07 that informed the catalog extension in this
> release. The catalog does not adopt every recommendation: credential
> stores (rclone/ngrok configs, keychains, cloud CLI credentials, RDP
> connection managers, crash dumps, NTDS.dit) stay out of the built-in
> catalog, and items that need engine features (per-source size caps,
> opt-in artifacts, a process source) are listed in `docs/design.md`.
> The authoritative paths are in `internal/catalog/data/*.yaml`.

Date: 2026-11-07. Scope: macOS file artifacts that Relic schema v1 can collect, ranked by value for
intrusion, infostealer and persistence IR.

## Method and evidence base

- Read the current catalog: `internal/catalog/data/{macos,browsers,linux,profiles}.yaml`,
  `docs/configuration.md`. Checked the resolver: intermediate literal symlinks (for example `/etc`)
  are followed, `{,a}` empty alternatives work, and `%UserHome%` on macOS = `/Users/*` + `/var/root`.
- Read the source of the reference tools (shallow clones, grep for paths):
  - Jamf Aftermath (github.com/jamf/aftermath): `artifacts/*.swift`, `persistence/*.swift`, `systemRecon/`
  - UAC (github.com/tclahr/uac): `artifacts/files/{system,logs,packages,applications,browsers}/*.yaml`
  - mac_apt (github.com/ydkhatri/mac_apt): `plugins/*.py`
  - ForensicArtifacts (github.com/ForensicArtifacts/artifacts): `artifacts/data/macos.yaml`
  - Velociraptor: `artifacts/definitions/MacOS/**` (Autoruns, Dock, Wifi, TimeMachine)
  - Objective-See KnockKnock (github.com/objective-see/KnockKnock): `Plugins/*.m`
- Web: theevilbit "Beyond the good ol' LaunchAgents" (35 posts), Eclectic Light (security data files
  in macOS 27 "Golden Gate", provenance tracking, XProtect BehaviourService), Unit 42 (Biome
  App.MenuItem), CrowdStrike (CoreAnalytics, Terminal saved state), 9to5Mac/heise (Sequoia
  notification DB move), Sophos (AMOS), AnyDesk docs, Jamf community, APOLLO.
- Local spot checks on a macOS 27.2 Mac (non-root, no FDA shell, `ls`/`du` only). "EPERM" below means
  the directory exists but listing failed with "Operation not permitted", which matches TCC/SIP
  protection (could also be shell sandboxing, so treat as "needs FDA, verify").

## Already covered (verified, no action)

| Item asked about | Relic artifact |
|---|---|
| BTM `BackgroundItems-v*.btm` + per-user `backgrounditems.btm` | `macos.persistence` (`/private/var/db/com.apple.backgroundtaskmanagement/*`) |
| QuarantineEventsV2 | `macos.quarantine_events` |
| Unified logs (`/private/var/db/diagnostics`, `uuidtext`) | `macos.unified_logs` |
| sharedfilelist, finder.plist | `macos.recent_items` |
| KnowledgeC (system + user) | `macos.knowledgec` |
| TCC.db (system + user) | `macos.tcc` |
| receipts `*.plist`, InstallHistory.plist, `install.log` | `macos.install_history`, `macos.logs` |
| Crash/diag reports incl. XProtect Remediator `XProtect_*.diag` and daily `*.core_analytics` (both under `[~]/Library/Logs/DiagnosticReports`, root's via `/var/root`) | `macos.logs` |
| `/private/var/log/anydesk.trace` (installed AnyDesk) | `macos.logs` |
| `/etc/periodic`, emond, cron tabs, StartupItems, LaunchAgents/Daemons | `macos.persistence` |
| `com.apple.loginwindow.plist` (system), `com.apple.alf.plist`, airport prefs, `preferences.plist`, `NetworkInterfaces.plist` | `macos.system_info` |
| dslocal user records | `macos.accounts` |
| `~/.zsh_sessions/*` | `unix.shell_history` (darwin source) |
| Safari History/Downloads/Bookmarks/LastSession, Chrome/Edge/Brave/Firefox | `browser.*` |

## Corrections and gaps in existing artifacts (quick wins)

1. **Gatekeeper DB moved.** `/private/var/db/SystemPolicy` (ForensicArtifacts, macOS <= 14) is absent on
   macOS 27.2; the active DBs are `/private/var/db/SystemPolicyConfiguration/{ExecPolicy,SystemPolicy,KextPolicy,Tickets}`
   (local listing; matches "Gatekeeper's databases have relocated slightly from macOS <=14 to 15",
   swiftly-detecting Sequoia notes). Collect both locations (entry 1).
2. **XPdb moved into a subfolder.** mac_apt checks `/private/var/protected/xprotect/XPdb` and
   `/private/var/protected/xprotect/db/XPdb`; `db/` exists on 27.2. Collect both (entry 2).
3. **Notification DB moved** in macOS 15 to `~/Library/Group Containers/group.com.apple.usernoted/db2/db`
   (TCC protected); the old `/private/var/folders/*/*/0/com.apple.notificationcenter/` is gone on 27.2.
4. **KnowledgeC is no longer the main app-usage store** on recent macOS; Biome streams are (mac_apt
   parses `App.InFocus`/`App.WebUsage`; Unit 42 found `App.MenuItem` on Tahoe 26). See entry 12.
5. `macos.persistence` misses at jobs, launchd `disabled*/loginitems*` plists, root LoginHook,
   ByHost re-opened apps, rc boot scripts, periodic config, global shell startup files (entry 3).
6. `unix.ssh` misses `~/.ssh/rc` and `/etc/ssh/sshrc`; `unix.shell_history` misses `~/.bash_sessions/*`
   (macOS bash per-session history, mac_apt `term_sessions.py`) (entry 4).
7. `%UserHome%` only covers `/Users/*` and `/var/root`. Velociraptor Autoruns also globs
   `/private/var/*/Library/LaunchAgents` (service-account homes). Low priority; consider reading
   `NFSHomeDirectory` from the dslocal user plists later.
8. Do NOT wildcard `/private/etc/*` (UAC does): it contains `krb5.keytab` (Kerberos keys on AD-bound
   Macs) and `kcpassword` (auto-login password, XOR-obfuscated). Use explicit lists.

## Top 15 ranked (intrusion / infostealer / persistence)

| # | Proposed artifact | New/ext | Profile | Why |
|---|---|---|---|---|
| 1 | `macos.gatekeeper` | new | standard | Execution provenance of downloaded apps, Gatekeeper decisions, kext approvals |
| 2 | `macos.xprotect` | new | standard | XProtect Behavior Service detections (processes touching browser data etc.) |
| 3 | `macos.persistence` | ext | minimal | at jobs, launchd overrides, login/logout hooks, re-opened apps, global shell rc |
| 4 | `unix.ssh`, `unix.shell_history` | ext | minimal | `sshrc`/`~/.ssh/rc` persistence, bash session histories |
| 5 | `macos.terminal_state` | new | standard | Terminal/iTerm2 scrollback (ClickFix "paste into Terminal" stealers) |
| 6 | `macos.privileged_helpers` | new | standard | Root helper binaries (SMJobBless), common for adware/backdoors |
| 7 | `macos.managed_config` | new | standard | Rogue config profiles / forced Chrome extensions / MDM |
| 8 | `macos.network_usage` | new | standard | Per-process bytes in/out (exfil, C2), SRUM-like |
| 9 | `macos.extensions` | new | standard | System/network extensions and kexts |
| 10 | `macos.persistence.auth` | new | standard | PAM, authorization DB, SecurityAgent and DirectoryServices plugins |
| 11 | `macos.remote_access_tools` | new | standard | ARD/Screen Sharing/RDP client + AnyDesk/TeamViewer/RustDesk/Splashtop |
| 12 | `macos.biome` | new | standard (subset) / full | App focus, web usage, menu-item timeline (Sonoma+ successor to KnowledgeC) |
| 13 | `macos.coreanalytics` | new | standard | Aggregated Mach-O execution evidence |
| 14 | `macos.persistence.plugins` | new | standard (metadata+exe) / full | QuickLook, Spotlight, screen saver, color picker, prefpane, audio HAL, osax |
| 15 | `macos.login_records` | new | standard | utmpx, sudo timestamps, BSM audit trail + audit config |

## Detailed entries (top 15)

### 1. `macos.gatekeeper` (new) - category `system` (or a new `security` category)
- Description: syspolicyd databases: execution policy/provenance, Gatekeeper rules, kext approvals, notarization tickets.
- Paths:
  ```
  /private/var/db/SystemPolicyConfiguration/{ExecPolicy,SystemPolicy,KextPolicy,Tickets}{,-shm,-wal}
  /private/var/db/SystemPolicyConfiguration/{.LastGKReject,SystemPolicy-prefs.plist}
  /private/var/db/SystemPolicy{,-shm,-wal}
  ```
- Value: `ExecPolicy` holds the `provenance_tracking` table (cdhash and app identity, created at first
  run after Gatekeeper evaluation; Eclectic Light) and, per community sources, `legacy_exec_history_v4`
  (execution history). `KextPolicy` (`kext_policy`, `kext_policy_mdm`) records user/MDM kext approvals.
  `SystemPolicy` `authority` table holds Gatekeeper rules. Directly answers "what downloaded code ran and
  was it allowed".
- Size: about 12 MB on the test Mac (ExecPolicy 5 MB + 2 MB WAL, Tickets 1 MB + 2.6 MB WAL, others < 1 MB).
- Sensitive: no. TCC: files are root 0600; Aftermath copies them under FDA. Assume root + FDA.
- Profile: standard. Confidence: high (paths: Aftermath `ProvenanceTracking.swift`, ForensicArtifacts
  `MacOSSystemPolicySQLiteDatabaseFile`, local listing on 27.2); medium for `.LastGKReject` content
  (file seen locally, content not inspected).

### 2. `macos.xprotect` (new) - category `system`
- Description: XProtect Behavior Service database and XProtect/XProtect Remediator/MRT version info.
- Paths:
  ```
  /private/var/protected/xprotect/{,db/}XPdb{,-shm,-wal}
  /private/var/protected/xprotect/XProtect.bundle/Contents/{Info.plist,version.plist,Resources/XProtect.meta.plist}
  /Library/Apple/System/Library/CoreServices/XProtect.bundle/Contents/{Info.plist,version.plist,Resources/XProtect.meta.plist}
  /Library/Apple/System/Library/CoreServices/{XProtect.app,MRT.app}/Contents/{Info.plist,version.plist}
  ```
- Value: XPdb (Bastion rules, macOS 13+) logs, without blocking, processes that touch protected
  locations such as browser data folders: direct infostealer evidence with timestamps and responsible
  process. Version plists tell whether the signature set could have detected a family on that date.
- Size: small (KB to a few MB). Sensitive: no.
- TCC: `/private/var/protected/xprotect/db` gave EPERM locally; community reads it with `sudo sqlite3`
  from an FDA terminal. Assume root + FDA.
- Profile: standard. Confidence: high (Aftermath `XProtectBehavioralService.swift`, mac_apt
  `xprotect.py` both XPdb paths, UAC `xprotect.yaml` version files, Eclectic Light 2026-09 for paths on
  macOS 27). Note: one 2026 Eclectic Light article suspects the Bastion DB may be unused on the newest
  release; collecting it is still cheap.

### 3. `macos.persistence` (extension) - category `persistence`
- Add:
  ```
  /private/var/at/jobs/*
  /private/var/db/com.apple.xpc.launchd/{disabled,loginitems}*.plist
  /private/var/db/launchd.db/com.apple.launchd*/overrides.plist
  /private/etc/{rc.netboot,rc.server,rc.cdrom,rc.cleanup,rc.installer_cleanup}
  /private/etc/{periodic.conf,periodic.conf.local,defaults/periodic.conf,daily.local,weekly.local,monthly.local}
  /private/etc/{profile,bashrc,bashrc_Apple_Terminal,zshenv,zprofile,zshrc,zshrc_Apple_Terminal,zlogin,zlogout}
  %UserHome%/Library/Preferences/com.apple.loginwindow.plist
  %UserHome%/Library/Preferences/ByHost/com.apple.loginwindow.*.plist
  %UserHome%/Library/Preferences/com.apple.loginitems.plist
  ```
- Value: at jobs (`atrun`, theevilbit #14; Aftermath reads `/usr/lib/cron/jobs` = `/private/var/at/jobs`);
  launchd `disabled.<uid>.plist`/`loginitems.<uid>.plist` (enabled/disabled state, SMLoginItem
  helpers; UAC `startup_items.yaml`, Velociraptor Autoruns); legacy overrides (Aftermath, mac_apt);
  LoginHook/LogoutHook live in root's `/private/var/root/Library/Preferences/com.apple.loginwindow.plist`
  (ForensicArtifacts, KnockKnock checks `~/Library/Preferences/com.apple.loginwindow.plist`);
  re-opened apps `TALAppsToRelaunchAtLogin` in ByHost (theevilbit #21, mac_apt `autostart.py`);
  rc boot scripts (theevilbit #34, KnockKnock StartupScripts); periodic config (ForensicArtifacts,
  Velociraptor `/private/etc/*.local`); global zsh/bash startup files (Aftermath, KnockKnock ShellConfigs,
  theevilbit #1). Putting them here keeps them in `minimal`.
- Size: < 1 MB. Sensitive: no. TCC: none known (`/private/var/at/tabs` is 0700 root, already collected).
- Profile: minimal. Confidence: high (all verified in at least one tool; `disabled*.plist` and
  `at/jobs` seen on 27.2).

### 4. `unix.ssh` and `unix.shell_history` (extensions)
- `unix.ssh` add (all OS): `'/etc/ssh/sshrc'`, `'%UserHome%/.ssh/rc'` (theevilbit #6; executed by sshd on
  every SSH login; works because the resolver follows the `/etc` -> `/private/etc` symlink).
- `unix.shell_history` darwin source add: `'%UserHome%/.bash_sessions/*'` (mac_apt `term_sessions.py`).
- Size: tiny. Sensitive: history may contain secrets (already accepted for shell history). TCC: none.
- Profile: minimal (same as the base artifacts). Confidence: high.

### 5. `macos.terminal_state` (new) - category `execution`
- Description: Terminal and iTerm2 window restoration state, which includes scrollback text.
- Paths:
  ```
  %UserHome%/Library/Saved Application State/{com.apple.Terminal,com.googlecode.iterm2}.savedState/*
  %UserHome%/Library/Application Support/iTerm2/SavedState/restorable-state.sqlite{,-shm,-wal}
  ```
- Value: `data.data` holds AES-encrypted per-window state, with keys in `windows.plist`. It keeps the
  Terminal scrollback even when shell history was disabled or deleted (CrowdStrike "Saved by the Shell";
  mac_apt `terminalstate.py`). This is high value for stealers delivered by "paste this into Terminal"
  lures and for hands-on-keyboard intruders.
- Size: KB to tens of MB per user. Sensitive: YES, scrollback can contain secrets. TCC: not known to be
  protected.
- Profile: standard. Confidence: medium. The Terminal path comes from mac_apt and UAC
  `saved_application_state.yaml`, but `~/Library/Saved Application State` did not exist on the 27.2 test
  Mac. An Apple forum thread also reports `com.apple.finder.savedState` missing since 15.5, so verify on
  macOS 15.5+. The iTerm2 `SavedState/restorable-state.sqlite` was seen only locally (low/medium).

### 6. `macos.privileged_helpers` (new) - category `persistence`
- Paths: `'/Library/PrivilegedHelperTools/*'`
- Value: root helper executables installed via SMJobBless/launchd; referenced by LaunchDaemons, and a
  frequent place for adware and backdoor components. Collecting the binary gives a hash for triage.
- Size: 25 MB on the test Mac (21 helpers); usually 1 to 30 MB. Sensitive: no. TCC: no.
- Profile: standard. Confidence: high (UAC `macos_privilegedhelpertools.yaml`; local).

### 7. `macos.managed_config` (new) - category `system`
- Description: managed preferences and the configuration profile store.
- Paths:
  ```
  /Library/Managed Preferences/**
  /private/var/db/ConfigurationProfiles/**
  ```
- Value: malicious or rogue `.mobileconfig` profiles are used to force-install browser extensions
  (Chrome/Edge read `ExtensionInstallForcelist` from `/Library/Managed Preferences`), set proxies, DNS,
  root CAs or PPPC grants. The Store (`ConfigProfiles.binary`) is an undocumented binary format; the
  managed-preference plists are readable.
- Size: 564 KB for `/Library/Managed Preferences` on the test Mac. The ConfigurationProfiles size was not
  measured (EPERM); it is expected to be small.
- Sensitive: low. Profiles can embed Wi-Fi/VPN credentials or certificates, so review before sharing.
- TCC: `ConfigurationProfiles/Store` gave EPERM, so it needs FDA. Managed Preferences is world-readable.
- Profile: standard. Confidence: medium. The Store path comes from a BigFix forum thread and was seen
  locally; Aftermath uses `profiles -C -o stdout-xml` instead, which is a roadmap item.

### 8. `macos.network_usage` (new) - category `network`
- Paths: `'/private/var/networkd/{,db/}netusage.sqlite{,-shm,-wal}'` (`db/` on macOS 11+).
- Value: per-process (`ZPROCESS`) and per-interface byte counters with first/last timestamps
  (`ZLIVEUSAGE`). It shows which binary moved data and when, comparable to SRUM network usage, and is
  strong for exfiltration after an infostealer zips its loot.
- Size: about 1 to 20 MB (not measured; EPERM). Sensitive: no. TCC: `/private/var/networkd/db` gave
  EPERM, so it needs FDA.
- Profile: standard. Confidence: high (mac_apt `netusage.py` both paths, Velociraptor exchange
  `MacOS.Applications.NetworkUsage`, UAC `network_application_usage.yaml`, APOLLO module).
  UAC's `/private/var/wireless/Library/Databases/DataUsage.sqlite` looks iOS-oriented; add it only as an
  optional extra (low confidence on macOS).

### 9. `macos.extensions` (new) - category `system` (or `persistence`)
- Paths, standard:
  ```
  /Library/SystemExtensions/db.plist
  /Library/SystemExtensions/*/*.systemextension/Contents/Info.plist
  /Library/Extensions/*/Contents/Info.plist
  ```
  full: `'/Library/SystemExtensions/**'`, `'/Library/Extensions/**'`
- Value: system extensions (network filters, VPN/proxy, endpoint security, camera/driver) and
  third-party kexts. `db.plist` records the state (activated, enabled, replaced) and the owning app. Use it
  to spot malicious network extensions or tampering with the EDR's extension. KextPolicy is collected by
  entry 1.
- Size: standard < 1 MB; full was 14 MB + 0.8 MB on the test Mac. Sensitive: no. TCC: no.
- Profile: standard (metadata), full (bundles). Confidence: high (KnockKnock `SystemExtensions.m`
  `/Library/SystemExtensions/db.plist`, Kexts dirs; Aftermath `SystemExtensions.swift`; ForensicArtifacts
  `/Library/Extensions/*`; the layout `<UUID>/<bundle>.systemextension/Contents` was seen locally). The
  `*/Contents` glob is deliberate because disabled kexts are renamed, for example `X.kext_DISABLED`.

### 10. `macos.persistence.auth` (new) - category `persistence`
- Paths:
  ```
  /private/etc/pam.d/*
  /private/var/db/auth.db{,-shm,-wal}
  /Library/Security/SecurityAgentPlugins/**
  /Library/DirectoryServices/PlugIns/**
  /usr/local/lib/pam/*
  ```
- Value: PAM module injection (theevilbit #5), authorization plugins wired into
  `system.login.console` in the authorization DB (theevilbit #28; they can capture the login password),
  and DirectoryServices plugins (KnockKnock). These are rare but high-impact credential-capture
  persistence.
- Size: < 2 MB. Sensitive: no (auth.db holds rules, not secrets). TCC: none known (auth.db is root 0600).
- Profile: standard. Confidence: high for `pam.d`, `auth.db` (ForensicArtifacts
  `MacOSAuthorizationRulesSQLiteDatabaseFile`; `-shm`/`-wal` seen locally), SecurityAgentPlugins and
  DS PlugIns (KnockKnock). Low for `/usr/local/lib/pam` (conventional third-party module dir; SIP blocks
  `/usr/lib/pam`).

### 11. `macos.remote_access_tools` (new) - category `remote_access` (macOS counterpart of `windows.remote_access_tools`)
- Paths:
  ```
  /private/var/db/RemoteManagement/caches/{AppUsage.plist,AppUsage.tmp,UserAcct.tmp}
  /private/var/db/RemoteManagement/RMDB/rmdb.sqlite3
  /private/var/db/RemoteManagement/ClientCaches/**
  %UserHome%/Library/Containers/com.apple.ScreenSharing/Data/Library/Preferences/com.apple.ScreenSharing.plist
  %UserHome%/Library/Containers/com.microsoft.rdc.macos/Data/Library/Application Support/com.microsoft.rdc.macos/com.microsoft.rdc.application-data.sqlite{,-shm,-wal}
  %UserHome%/.anydesk*/*.{trace,conf,txt}
  %UserHome%/Library/Logs/TeamViewer/*
  %UserHome%/Library/Caches/TeamViewer/{tvchatfilecache,tvprint}.db*
  %UserHome%/Library/Logs/RustDesk/**
  /Library/Application Support/Splashtop*/Logs/*
  %UserHome%/Library/Application Support/Splashtop*/{Logs/*,STServerList}
  ```
- Value: ARD client caches record users and application usage while the Mac was managed by ARD; on an
  admin Mac, `RMDB`/`ClientCaches` list managed clients. The Screen Sharing plist shows outbound VNC
  targets (lateral movement from this Mac). The MS Remote Desktop DB shows outbound RDP targets.
  Third-party RAT logs record incoming sessions and IDs. ARD/Screen Sharing settings themselves are
  in `/Library/Preferences/com.apple.RemoteManagement.plist` (entry 17).
- Size: small (rmdb can grow on admin consoles). Sensitive: AnyDesk `*.conf` may hold the hashed
  unattended-access password and RustDesk config is excluded on purpose; the logs are not sensitive.
- TCC: app containers (ScreenSharing, MS RDC) need FDA on macOS 14+. RemoteManagement is root-only.
- Profile: standard. Confidence: ARD high (ForensicArtifacts, mac_apt `ard.py`); ScreenSharing and MS RDC
  medium (mac_apt `screensharing.py`, `msrdc.py`); AnyDesk medium (AnyDesk docs:
  `~/.anydesk/anydesk.trace`, `~/.anydesk_ad_<prefix>/`; installed: `/var/log/anydesk.trace`, already
  collected); TeamViewer/RustDesk/Splashtop medium (UAC application YAMLs). ScreenConnect on macOS uses
  `/opt/{screenconnect,connectwisecontrol}-<id>` (Jamf community), but no log file is documented there, so
  it is not proposed. Its launch items are already collected.

### 12. `macos.biome` (new) - category `user_activity`
- Paths, standard (`macos.biome.app_usage`):
  ```
  %UserHome%/Library/Biome/streams/restricted/{App.InFocus,App.WebUsage,App.MenuItem}/local/**
  ```
  full (`macos.biome`): `'%UserHome%/Library/Biome/streams/**'`, `'/private/var/db/biome/streams/**'`
- Value: SEGB-encoded protobuf streams. App.InFocus is the foreground-app timeline (GUI execution
  evidence), App.WebUsage holds web usage by app, and App.MenuItem (macOS 26+) holds the text of each menu
  item the user selected with a timestamp, for example "Move to Trash" or "Export". On Sonoma and later
  Biome is the main successor to KnowledgeC usage data.
- Size: user streams are typically a few MB to over 100 MB; full can be hundreds of MB (not measured; EPERM).
- Sensitive: moderate (WebUsage URLs/domains, menu text). TCC: `~/Library/Biome` gave EPERM, so it needs
  FDA. `/private/var/db/biome/streams` is `_biome` 0700.
- Profile: standard (subset), full (all). Confidence: medium. Paths come from mac_apt `biome.py`
  (`~/Library/Biome/streams/`, `/private/var/db/biome/streams/`, stream names App.InFocus/App.WebUsage),
  UAC `biome.yaml` (`restricted/*/local`) and Unit 42 (`restricted/App.MenuItem/local`). The per-stream
  subset should be checked on 26/27.

### 13. `macos.coreanalytics` (new) - category `execution`
- Paths: `'/private/var/db/analyticsd/aggregates/**'` (the daily `*.core_analytics` files are already in
  `macos.logs`).
- Value: about one month of Mach-O program executions (path, usage counters) with day-level granularity.
  It is evidence that a binary ran even after deletion (CrowdStrike, macOS 10.13+).
- Size: small (KB to a few MB). Sensitive: no. TCC: root-only (EACCES as a user locally); FDA unknown.
- Profile: standard. Confidence: medium. The path is from ForensicArtifacts `MacOSCoreAnalyticsFile` and
  CrowdStrike. How useful the aggregates are on 14 to 27 has not been re-verified (sources disagree on
  retention).

### 14. `macos.persistence.plugins` (new) - category `persistence`
- Paths, standard (bundle metadata + main executable):
  ```
  /Library/{QuickLook,Spotlight,Screen Savers,ColorPickers,PreferencePanes,Input Methods,ScriptingAdditions,Audio/Plug-Ins/HAL}/*/Contents/{Info.plist,MacOS/*}
  %UserHome%/Library/{QuickLook,Spotlight,Screen Savers,ColorPickers,PreferencePanes,Input Methods}/*/Contents/{Info.plist,MacOS/*}
  ```
  full: the same directories with `/**`.
- Value: these are plugin load points that run code inside Apple processes: QuickLook (#12), Spotlight
  importers (#11), screen savers (#16), color pickers (#17), preference panes (#9), CoreAudio HAL
  drivers running as root in coreaudiod (#13), and scripting additions (`.osax`, Velociraptor Autoruns).
- Size: about 25 MB on the test Mac with executables (PreferencePanes 23 MB). The full profile can reach
  tens of MB. Sensitive: no. TCC: no.
- Profile: standard (as above), full (`**`). Confidence: high for the directories (theevilbit posts,
  KnockKnock `SpotlightImporters.m`, Velociraptor `/Library/ScriptingAdditions/*.osax`). Input Methods is
  medium (a known code-loading location, but not in the sources read here). QuickLook generators
  (`.qlgenerator`) are legacy on recent macOS but still worth listing.

### 15. `macos.login_records` (new) - category `accounts`
- Paths:
  ```
  /private/var/run/utmpx
  /private/var/db/sudo/ts/*
  /private/var/audit/*
  /private/etc/security/audit_{control,class,event,user,warn}
  ```
- Value: `utmpx` holds current and recent logins. sudo `ts` files give the last successful sudo per
  user/tty (mac_apt `sudo_lastrun.py`). The BSM audit trail exists if auditing was enabled. `audit_warn`
  is a root persistence hook (theevilbit #31).
- Size: < 1 MB without audit. The audit trail can be large when enabled; BSM is deprecated since macOS 11
  and usually empty. Sensitive: no. TCC: root only (`sudo/ts`, `audit` are 0700).
- Profile: standard. Confidence: high (ForensicArtifacts `MacOSUtmpxFile`, `MacOSAuditLogFile`; mac_apt;
  Aftermath `LogFiles.swift` `/var/audit/`; UAC `logs/macos.yaml`; all paths seen locally).

## Further recommendations (rank 16+)

| # | Artifact | Paths (Relic syntax) | Profile | Size / sens. / TCC | Conf. | Source |
|---|---|---|---|---|---|---|
| 16 | `macos.persistence.user_apps` (new, persistence) | `%UserHome%/Library/Preferences/{com.apple.dock.plist,com.apple.Terminal.plist,com.googlecode.iterm2.plist,com.apple.FolderActionsDispatcher.plist}`; `/Library/Scripts/Folder Action Scripts/*`; `%UserHome%/Library/Scripts/Folder Action Scripts/*`; `%UserHome%/Library/Application Support/iTerm2/Scripts/AutoLaunch{,.scpt}`; `%UserHome%/Library/Application Support/iTerm2/Scripts/AutoLaunch/**`; `%UserHome%/Library/Application Support/xbar/plugins/*`; `%UserHome%/.hammerspoon/init.lua` | standard | < 1 MB / no / no | high | theevilbit #2,7,8,20,24,27; Velociraptor `Dock.yaml` |
| 17 | `macos.preferences.system` (new, system) | `/Library/Preferences/**` | standard | 0.5 MB on test / low / `com.apple.TimeMachine.plist` needs FDA (Elastic uses it as FDA probe) | high | UAC `library_preferences.yaml`, ForensicArtifacts `/Library/Preferences/**/*.plist`. Covers `com.apple.wifi.known-networks.plist` (mac_apt wifi), `com.apple.Bluetooth.plist`, `com.apple.TimeMachine.plist`, `com.apple.networkextension.plist` (VPN/proxy/content filters), `com.apple.RemoteManagement.plist`, `com.apple.preferences.accounts.plist` (deletedUsers, mac_apt users), `com.apple.SoftwareUpdate.plist`, `com.google.Chrome.plist`, all of `SystemConfiguration/*.plist` |
| 18 | `macos.system_info` (ext) | `/private/etc/{paths,manpaths,man.conf,asl.conf,syslog.conf,newsyslog.conf,pf.conf,hosts.equiv,exports,nfs.conf,auto_master,autofs.conf,ttys,kern_loader.conf,localtime}`; `/private/etc/{paths.d,manpaths.d,newsyslog.d,asl,pf.anchors}/*`; `/private/var/db/.AppleSetupDone`; `/private/var/db/dhcpclient/leases/*` | standard | < 1 MB / no / root | high (mac_apt networking/basicinfo, ForensicArtifacts, theevilbit #30, local `/private/etc` listing) | log-config tampering, PATH hijack, pf redirects, NFS exports, install date, DHCP history |
| 19 | `macos.accounts` (ext) | `/private/var/db/dslocal/nodes/Default/groups/*.plist`; `/private/var/db/dslocal/nodes/Default/sharepoints/*.plist` | standard | < 1 MB / no / root | high | mac_apt `filesharing.py`; all groups give `com.apple.access_ssh`/`access_screensharing` membership |
| 20 | `macos.devices` (new, devices) | `/Library/Bluetooth/Library/Preferences/com.apple.MobileBluetooth.devices.plist`; `/Library/Bluetooth/com.apple.MobileBluetooth.ledevices.*`; `%UserHome%/Library/Preferences/com.apple.iPod.plist` | standard | small / no / no | medium | mac_apt bluetooth/iDeviceInfo, UAC `bluetooth.yaml`, ForensicArtifacts `MacOSiDevices` |
| 21 | `macos.app_inventory` (new, system) | `/Applications/*.app/Contents/Info.plist`; `/Applications/*/*.app/Contents/Info.plist`; `%UserHome%/Applications/*.app/Contents/Info.plist`; `/Applications/*.app/Contents/Library/LoginItems/*/Contents/Info.plist` | standard (full: + `_CodeSignature/CodeResources`) | about 1 MB for 94 apps on test / no / no | high | UAC `installed_applications.yaml`, KnockKnock `LoginItems.m` |
| 22 | `macos.spotlight_shortcuts` (new, user_activity) | `%UserHome%/Library/Preferences/com.apple.Spotlight.plist`; `%UserHome%/Library/Application Support/com.apple.spotlight.Shortcuts`; `%UserHome%/Library/Application Support/com.apple.spotlight/com.apple.spotlight.Shortcuts{,.v3}`; `%UserHome%/Library/Group Containers/group.com.apple.spotlight/com.apple.spotlight.Shortcuts.v3` | standard | KB / low / Group Container needs FDA (EPERM) | high | mac_apt `spotlightshortcuts.py` (per-version list), UAC `spotlight.yaml` |
| 23 | `macos.tcc` (ext) | `/Library/Application Support/com.apple.TCC/MDMOverrides.plist` | standard | KB / no / FDA | high | Aftermath `Overrides.swift`; seen locally (MDM PPPC grants) |
| 24 | `browser.safari` (ext) | `%UserHome%/Library/Safari/{RecentlyClosedTabs.plist,PerSitePreferences.db*,CloudTabs.db*,SafariTabs.db*,UserNotificationPermissions.plist,Extensions/Extensions.plist}`; `%UserHome%/Library/Containers/com.apple.Safari/Data/Library/Safari/{AppExtensions,WebExtensions}/Extensions.plist` | standard | small / low / FDA | medium (UAC `safari.yaml`, Aftermath, osquery issue #8684; Safari 17 profile layout not verified) | malicious Safari extensions, notification-permission abuse |
| 25 | `browser.arc`, `browser.opera`, `browser.vivaldi` (new, browser) | `%UserHome%/Library/Application Support/Arc/User Data/*/{History,...}`; `%UserHome%/Library/Application Support/com.operasoftware.Opera/{History,...}`; `%UserHome%/Library/Application Support/Vivaldi/*/{History,...}` (same file set and Extensions manifests as `browser.chrome`) | standard | as Chrome | medium | Aftermath `Arc.swift`; KnockKnock (Opera dir); UAC opera/vivaldi. Stealers target all Chromium browsers |

Full-only (large, niche or more sensitive):

| Artifact | Paths | Notes | Conf. |
|---|---|---|---|
| `macos.powerlog` | `/private/var/db/powerlog/Library/BatteryLife/CurrentPowerlog.PLSQL{,-shm,-wal}`; `/private/var/db/powerlog/Library/BatteryLife/Archives/*.PLSQL.gz` | 85 MB on the test Mac; app/process energy and wake timeline (UAC `powerlog.yaml`, seen locally) | high |
| `macos.screentime` | `/private/var/folders/*/*/0/com.apple.ScreenTimeAgent/Store/RMAdminStore-{Local,Cloud}.sqlite{,-shm,-wal}` | Per-app usage only if Screen Time is on; about 15 MB; FDA (EPERM, but the dir exists on 27.2) (mac_apt `screentime.py`) | medium |
| `macos.notifications` | `%UserHome%/Library/Group Containers/group.com.apple.usernoted/db2/db{,-shm,-wal}`; `/private/var/folders/*/*/0/com.apple.notificationcenter/db2/db{,-shm,-wal}` | SENSITIVE (message previews, OTP codes); FDA on 15+ (mac_apt `notifications.py`, 9to5Mac/heise) | high |
| `macos.saved_state` | `%UserHome%/Library/Saved Application State/*.savedState/{windows.plist,data.data,window_*.data}`; `%UserHome%/Library/Containers/*/Data/Library/Saved Application State/*.savedState/{windows.plist,data.data,window_*.data}` | All apps' window state; sensitive; containers need FDA (UAC, mac_apt `savedstate.py`; container variant seen locally) | medium |
| `macos.quicklook_thumbnails` | `/private/var/folders/*/*/C/com.apple.QuickLook.thumbnailcache/{index.sqlite*,thumbnails.data}` | Files previewed, including on removed volumes; thumbnails are sensitive (mac_apt `quicklook.py`) | medium |
| `macos.icloud_drive` | `%UserHome%/Library/Application Support/CloudDocs/session/db/{client,server}.db*` | iCloud Drive file inventory and sync, a possible exfil route (mac_apt `iCloud.py`) | medium |
| `macos.internet_accounts` | `%UserHome%/Library/Accounts/Accounts4.sqlite*` | Configured Apple/Google/Exchange accounts, no passwords; likely FDA (UAC, ForensicArtifacts) | medium |
| `macos.install_history` (ext) | `/private/var/db/receipts/*.bom` | Which pkg installed which file; 154 MB on the test Mac | high |
| `macos.staging_dirs` | `/private/tmp/**`, `/private/var/tmp/**`, `/Users/Shared/**` | AMOS stages loot in `/tmp/<n>/` and `/tmp/out.zip` (Sophos). Size is unbounded, so it needs a per-source `max_file_size`, which v1 lacks; UAC caps at 10 MB per file. Prefer a listing source (roadmap) | medium |
| `macos.biome`, `macos.persistence.plugins` full variants | see entries 12 and 14 | | |

## Deliberately NOT recommended as built-ins (credentials or message content)

`full` is `'*'`, so anything built in ends up in `full`. Keep these as documented custom-artifact
examples only, mirroring the browser cookie and credential policy:

- Keychains: `%UserHome%/Library/Keychains/**`, `/Library/Keychains/System.keychain`,
  `/private/var/db/SystemKey` (UAC collects keychains; infostealers exfiltrate `login.keychain-db`).
- `/private/etc/kcpassword` (auto-login password, XOR; Aftermath and mac_apt read it),
  `/private/etc/krb5.keytab`, `/private/var/db/lockdown/*` (iOS pairing records with keys).
- Messages `chat.db`, Mail (`~/Library/Mail`, `com.apple.mail` container), Notes group container,
  Chromium `Login Data`/`Cookies`/`Web Data`, crypto wallet folders, `~/.ssh/id_*`,
  AnyDesk/RustDesk configs with access passwords.
- For infostealer cases what usually matters is presence and timestamps of these files, not content.
  That calls for a metadata-only source (see roadmap).

## Non-file evidence for the roadmap (needs new source types)

- **Unified log export:** `log collect --output <x>.logarchive [--last 7d]` (self-contained archive), or
  `log show --style ndjson --predicate ...` for high-value subsystems (TCC prompts, `osascript`, sudo,
  sshd, `com.apple.xprotect`, XProtect Remediator, syspolicyd).
- **BTM decoded:** `sfltool dumpbtm` (Aftermath `BTM.swift`). The `.btm` files are already collected,
  but they are a keyed archive that is hard to parse offline.
- **launchd live state:** `launchctl print system`, `launchctl print gui/<uid>`,
  `launchctl print-disabled system|gui/<uid>`, `launchctl list`. These show jobs loaded from deleted
  plists and the effective disabled state, which can differ from the plist (per launchd cheatsheets).
- **Profiles:** `profiles show -o stdout-xml` / `profiles -C -o stdout-xml` (Aftermath), because
  `ConfigProfiles.binary` is undocumented.
- **Extensions:** `systemextensionsctl list`, `kmutil showloaded`, `pluginkit -vmA` (KnockKnock;
  Finder Sync, Safari and other app extensions, per user).
- **Processes and network:** `ps -axwwo user,pid,ppid,lstart,command`, `lsof -i -n -P` (Aftermath),
  `netstat -anv`, `ifconfig`, `arp -an`, `scutil --proxy`, `scutil --dns`, `pfctl -sr`.
- **Code signing and xattrs** for persistence targets and running binaries: `codesign -dvvv
  --entitlements -`, `spctl --assess -vv`, and extended attributes `com.apple.quarantine`,
  `com.apple.provenance` (links to ExecPolicy `provenance_tracking`), `com.apple.macl`,
  `kMDItemWhereFroms`. Cheapest variant: record xattrs as metadata of every collected file.
- **Referenced-file follow-up:** parse `Program`/`ProgramArguments` of collected launch plists, BTM
  entries and LoginHook values, then collect the referenced executables. Aftermath does this for
  login hooks.
- **Security posture:** `csrutil status`, `spctl --status`, `fdesetup status`,
  `systemsetup -getremotelogin`, `last` (Aftermath `securityAssessment`), `nvram -p`
  (boot-args, theevilbit #35), `security authorizationdb read system.login.console`.
- **Directory listings (metadata only, no content):** `~/Downloads`, `~/.Trash`, `/private/tmp`,
  `/Users/Shared`, hidden dotfiles in homes (AMOS drops `~/.pass`, `~/.helper`-style files; Sophos),
  `~/Library/Application Support/*`. Aftermath `CommonDirectories.swift` and UAC bodyfile do this.
- **Rosetta AOT cache** `/private/var/db/oah` (evidence of x86_64 binaries executed on Apple silicon).
  It exists on 27.2 but is SIP-restricted, so it is not file-collectable live; note it for image-based
  collection.
- `sysdiagnose` (very large, has many of the above), `system_profiler SPApplicationsDataType
  SPExtensionsDataType SPUSBDataType`, `tmutil destinationinfo`.

## Sources

- Jamf Aftermath: https://github.com/jamf/aftermath (ProvenanceTracking.swift, XProtectBehavioralService.swift, Overrides.swift, Cron.swift, LoginHooks.swift, ShellHistoryAndProfiles.swift, SystemConfig.swift, SystemExtensions.swift, LogFiles.swift, SystemReconModule.swift, ConfigurationProfiles.swift, BTM.swift, browsers/Arc.swift)
- UAC: https://github.com/tclahr/uac (artifacts/files/system/{biome,powerlog,network_application_usage,xprotect,macos_privilegedhelpertools,saved_application_state,startup_items,library_preferences,bluetooth,user_accounts,keychain,tmp,etc}.yaml; applications/{anydesk,teamviewer,rustdesk,splashtop,spotlight}.yaml; packages/installed_applications.yaml; browsers/*.yaml)
- mac_apt: https://github.com/ydkhatri/mac_apt (plugins/xprotect.py, netusage.py, biome.py, spotlightshortcuts.py, notifications.py, screentime.py, sudo_lastrun.py, utmpx.py, ard.py, screensharing.py, msrdc.py, term_sessions.py, terminalstate.py, autostart.py, networking.py, airport_preferences.py, bluetooth.py, users.py, filesharing.py, quicklook.py, iCloud.py)
- ForensicArtifacts: https://github.com/ForensicArtifacts/artifacts/blob/main/artifacts/data/macos.yaml
- Velociraptor: https://github.com/Velocidex/velociraptor/tree/master/artifacts/definitions/MacOS (Detection/Autoruns.yaml); https://docs.velociraptor.app/exchange/artifacts/pages/macos.applications.networkusage
- KnockKnock: https://github.com/objective-see/KnockKnock/tree/master/Plugins
- theevilbit, Beyond the good ol' LaunchAgents: https://theevilbit.github.io/beyond/ (posts 0001-0035)
- Eclectic Light: https://eclecticlight.co/2026/09/28/silently-updated-security-data-files-in-golden-gate/ ; https://eclecticlight.co/2023/05/10/how-macos-now-tracks-the-provenance-of-apps/ ; https://eclecticlight.co/2024/06/28/what-do-xprotect-behaviourservice-and-bastion-rules-do/
- Sequoia Gatekeeper changes: https://swiftly-detecting.notion.site/Sequoia-Gatekeeper-Updates-62987010c3504842a8b946982b961340
- Unit 42, Biome App.MenuItem: https://unit42.paloaltonetworks.com/new-macos-artifact-discovered/
- CrowdStrike CoreAnalytics: https://www.crowdstrike.com/en-us/blog/i-know-what-you-did-last-month-a-new-artifact-of-execution-on-macos-10-13/
- CrowdStrike Terminal saved state: https://crowdstrike.com/blog/reconstructing-command-line-activity-on-macos
- Notification DB move: https://9to5mac.com/2024/09/01/security-bite-apple-addresses-privacy-concerns-around-notification-center-database-in-macos-sequoia/ ; https://mjtsai.com/blog/2024/07/15/sequoia-finally-addresses-notification-center-privacy
- APOLLO netusage: https://github.com/mac4n6/APOLLO/blob/master/modules/datausage_zliveusage.txt
- Sophos on AMOS: https://www.sophos.com/en-us/blog/why-amos-matters-the-macos-malware-stealing-data-at-scale
- AnyDesk trace files: https://support.anydesk.com/knowledge/trace-files
- ConfigProfiles.binary: https://forum.bigfix.com/t/request-for-comment-c3-mac-os-x-configuration-profiles/20507
- ScreenConnect on macOS: https://community.jamf.com/t5/jamf-pro/uninstall-connect-control-client-for-mac/m-p/245402
- Safari extensions plist: https://github.com/osquery/osquery/issues/8684
- Finder savedState missing in 15.5: https://discussions.apple.com/thread/256078316
