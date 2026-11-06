# Built-in artifacts

Generated from the embedded catalog (`internal/catalog/data/*.yaml`) with
`relic list artifacts --json | python3 scripts/gen-artifact-docs.py`. Use
`relic show artifact <name>` for the exact path patterns.

Profiles are cumulative: `standard` contains `minimal`, `full` contains everything.

## Windows

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `windows.registry.system` | Windows | minimal | System registry hives (SAM, SECURITY, SOFTWARE, SYSTEM, DEFAULT) with transaction logs and RegBack copies. |
| `windows.registry.user` | Windows | minimal | Per-user registry hives NTUSER.DAT and UsrClass.dat with transaction logs (all profiles incl. service accounts). |
| `windows.registry.amcache` | Windows | minimal | Amcache.hve (program execution and installation evidence) and RecentFileCache.bcf (Windows 7). |
| `windows.eventlogs` | Windows | minimal | Windows event logs (EVTX) of all channels. |
| `windows.prefetch` | Windows | minimal | Prefetch files (program execution evidence). |
| `windows.srum` | Windows | standard | System Resource Usage Monitor database (application and network usage per user). |
| `windows.pca` | Windows | standard | Program Compatibility Assistant launch records (Windows 11). |
| `windows.scheduled_tasks` | Windows | minimal | Scheduled task definitions (XML and legacy .job) and the task scheduler log. |
| `windows.startup_folders` | Windows | minimal | Contents of the per-machine and per-user Startup folders. |
| `windows.wmi_repository` | Windows | standard | WMI repository (CIM database), used to detect WMI event subscription persistence. |
| `windows.group_policy` | Windows | standard | Local Group Policy files including logon/startup scripts. |
| `windows.powershell_history` | Windows | minimal | PSReadLine console history of every user. |
| `windows.recent_files` | Windows | standard | LNK files and jump lists (AutomaticDestinations, CustomDestinations) and Office recent files. |
| `windows.activities_cache` | Windows | standard | Windows Timeline database (ActivitiesCache.db). |
| `windows.notifications` | Windows | standard | Windows push notification database. |
| `windows.webcache` | Windows | standard | WebCache ESE database (Internet Explorer / legacy Edge history, file access via file:// URLs). |
| `windows.recycle_bin` | Windows | standard | Recycle Bin $I metadata files (original path, size and deletion time; not the deleted content). |
| `windows.ntfs.mft` | Windows | standard | NTFS Master File Table of the system drive (read via raw NTFS access). |
| `windows.ntfs.logfile` | Windows | standard | NTFS transaction log ($LogFile) of the system drive (raw NTFS access). |
| `windows.ntfs.usnjrnl` | Windows | standard | NTFS change journal $UsnJrnl:$J of the system drive. The leading sparse region is skipped; its offset is recorded in the manifest. |
| `windows.ntfs.boot` | Windows | full | NTFS boot sector file ($Boot) of the system drive. |
| `windows.ntfs.secure` | Windows | full | NTFS security descriptor stream ($Secure:$SDS) of the system drive. |
| `windows.setupapi_logs` | Windows | standard | SetupAPI device installation logs (USB device history). |
| `windows.hosts` | Windows | standard | hosts, lmhosts, networks, protocol and services files. |
| `windows.firewall_logs` | Windows | standard | Windows Firewall logs. |
| `windows.bits` | Windows | standard | Background Intelligent Transfer Service queue database (qmgr). |
| `windows.user_access_logging` | Windows | standard | User Access Logging databases (Windows Server). |
| `windows.defender` | Windows | standard | Microsoft Defender support logs (MPLog) and detection history. |
| `windows.ssh` | Windows | standard | OpenSSH for Windows server configuration, logs and user authorized_keys/known_hosts (no private keys). |
| `windows.remote_access_tools` | Windows | standard | Logs and connection traces of common remote administration tools (AnyDesk, TeamViewer). |
| `windows.rdp_bitmap_cache` | Windows | full | RDP client bitmap cache (screen fragments of outgoing RDP sessions). |
| `windows.thumbcache` | Windows | full | Explorer thumbnail and icon caches. |
| `windows.iis_logs` | Windows | full | IIS web server logs (can be large on busy servers). |

## Linux

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `linux.accounts` | Linux | minimal | Local users, groups and sudo configuration. |
| `linux.accounts.shadow` | Linux | standard | Shadow password files (password hashes and password change dates). |
| `linux.logs.auth` | Linux | minimal | Authentication, system and audit logs including rotated files, wtmp/btmp/lastlog. |
| `linux.logs.var_log` | Linux | standard | Everything below /var/log except the systemd journal. |
| `linux.logs.journal` | Linux | full | Persistent and volatile systemd journal files. |
| `linux.persistence.cron` | Linux | minimal | cron, anacron and at job definitions and spool directories. |
| `linux.persistence.systemd` | Linux | minimal | Local and runtime systemd units and configuration, user units. |
| `linux.persistence.systemd_vendor` | Linux | full | Vendor-supplied systemd units below /usr/lib/systemd. |
| `linux.persistence.init` | Linux | minimal | SysV init scripts, rc.local, shell profiles, dynamic linker preload, kernel module, udev, motd and XDG autostart configuration. |
| `linux.system_info` | Linux | minimal | Host identity, release, time zone, mounts, name resolution, PAM, syslog, sysctl and package sources. |
| `linux.packages` | Linux | standard | Installed package inventory (dpkg status) and package manager logs. |
| `linux.packages.rpmdb` | Linux | full | RPM package database (can be 100 MB or more). |
| `linux.web_server_config` | Linux | standard | nginx and Apache httpd configuration. |

## Linux and macOS

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `unix.shell_history` | Linux, macOS | minimal | Shell, REPL and tool histories of every user (bash, zsh, fish, python, mysql, psql, less, vim, wget). |
| `unix.shell_config` | Linux, macOS | standard | Per-user shell startup files and git configuration. |
| `unix.ssh` | Linux, macOS | minimal | SSH server/client configuration and per-user authorized_keys, known_hosts and config (no private keys). |

## macOS

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `macos.system_info` | macOS | minimal | macOS version, local accounts files, hosts, sudoers, network and login window preferences. |
| `macos.accounts` | macOS | standard | Local directory service user records (include password hash data) and the admin group. |
| `macos.persistence` | macOS | minimal | Launch agents and daemons, startup items, login/background items, periodic scripts, cron/at tabs and emond rules. |
| `macos.logs` | macOS | minimal | Classic log files below /private/var/log and crash/diagnostic reports. |
| `macos.logs.all` | macOS | full | All application logs below /Library/Logs and the users' Library/Logs. |
| `macos.unified_logs` | macOS | standard | Unified logging tracev3 files and uuidtext (parse with 'log show --archive' after rebuilding a .logarchive). Often 1 GB or more. |
| `macos.fsevents` | macOS | standard | File system event logs (.fseventsd) of the system and data volumes. |
| `macos.quarantine_events` | macOS | standard | LaunchServices quarantine events database (downloaded files and their origin). |
| `macos.knowledgec` | macOS | standard | KnowledgeC databases (application usage, device state). TCC protected. |
| `macos.tcc` | macOS | standard | Transparency, Consent and Control databases (granted privacy permissions). Requires Full Disk Access. |
| `macos.install_history` | macOS | standard | Software installation history and package receipts. |
| `macos.recent_items` | macOS | standard | Shared file lists (recent documents, applications, servers) and Finder preferences. |

## Browsers (all platforms)

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `browser.chrome` | Windows, Linux, macOS | standard | Google Chrome history, bookmarks, preferences, shortcuts, top sites and extension manifests of all profiles. |
| `browser.edge` | Windows, Linux, macOS | standard | Microsoft Edge (Chromium) history, bookmarks, preferences and extension manifests of all profiles. |
| `browser.brave` | Windows, Linux, macOS | standard | Brave browser history, bookmarks and preferences of all profiles. |
| `browser.firefox` | Windows, Linux, macOS | standard | Mozilla Firefox history, downloads, form history, bookmarks backups, extensions and session state of all profiles. |
| `browser.safari` | macOS | standard | Safari history, downloads, bookmarks and last session. Requires Full Disk Access. |

