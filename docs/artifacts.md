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
| `windows.scheduled_tasks` | Windows | minimal | Scheduled task definitions (XML and legacy .job), PowerShell scheduled jobs and the task scheduler log. |
| `windows.startup_folders` | Windows | minimal | Contents of the per-machine and per-user Startup folders. |
| `windows.wmi_repository` | Windows | standard | WMI repository (CIM database), used to detect WMI event subscription persistence. |
| `windows.group_policy` | Windows | standard | Local Group Policy files including logon/startup scripts, and the cache of applied domain Group Policy Preferences (scheduled tasks, files, services pushed by GPO). |
| `windows.powershell_history` | Windows | minimal | PSReadLine console history of every user. |
| `windows.recent_files` | Windows | standard | LNK files and jump lists (AutomaticDestinations, CustomDestinations), Office recent files, Desktop and Start Menu shortcuts. |
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
| `windows.firewall_logs` | Windows | standard | Windows Firewall logs including the rotated previous log. |
| `windows.bits` | Windows | standard | Background Intelligent Transfer Service queue database (qmgr). |
| `windows.user_access_logging` | Windows | standard | User Access Logging databases (Windows Server). |
| `windows.defender` | Windows | standard | Microsoft Defender support logs (MPLog), MpCmdRun log (signature removal, downloads), detection history and quarantine metadata (not the quarantined files). |
| `windows.ssh` | Windows | standard | OpenSSH for Windows server configuration, logs and user authorized_keys/known_hosts (no private keys). |
| `windows.remote_access_tools` | Windows | standard | AnyDesk and TeamViewer logs, incoming connection and file transfer records, chat logs and configuration (not AnyDesk's service.conf, which holds the password hash and private key). |
| `windows.rdp_bitmap_cache` | Windows | full | RDP client bitmap cache (screen fragments of outgoing RDP sessions). |
| `windows.thumbcache` | Windows | full | Explorer thumbnail and icon caches. |
| `windows.iis_logs` | Windows | full | IIS web server logs (can be large on busy servers). |
| `windows.rmm.screenconnect` | Windows | standard | ConnectWise ScreenConnect client command and file transfer traces and configuration; on a ScreenConnect server the session database, user and role definitions, web.config and App_Extensions (webshells). |
| `windows.rmm.splashtop` | Windows | standard | Splashtop Streamer and gateway logs (sessions with client IP, file transfers, chat). Also used by Atera remote access. |
| `windows.rmm.atera` | Windows | standard | Atera agent package logs and configuration (remote commands and their output). |
| `windows.rmm.ninjaone` | Windows | standard | NinjaOne (NinjaRMM) agent logs. |
| `windows.rmm.datto` | Windows | standard | Datto RMM (CentraStage) agent and service logs. |
| `windows.rmm.nable` | Windows | standard | N-able (N-central) Windows agent logs and configuration. |
| `windows.rmm.kaseya` | Windows | standard | Kaseya VSA agent logs, Live Connect logs, setup logs and scripts in the agent working directory. |
| `windows.rmm.action1` | Windows | standard | Action1 agent logs, scripts and rule data. |
| `windows.rmm.meshagent` | Windows | standard | MeshCentral agent logs and settings, Tactical RMM agent log. |
| `windows.rmm.level` | Windows | standard | Level RMM agent logs. |
| `windows.rmm.immybot` | Windows | standard | ImmyBot agent logs, scripts and configuration. |
| `windows.rmm.rustdesk` | Windows | standard | RustDesk logs (client and service; not the configuration, which holds the access password). |
| `windows.rmm.quick_assist` | Windows | standard | Quick Assist and Remote Help embedded browser history and preferences (help desk impersonation lures). |
| `windows.rmm.simplehelp` | Windows | standard | SimpleHelp remote access and technician logs. |
| `windows.rmm.logmein` | Windows | standard | LogMeIn host logs. |
| `windows.rmm.zoho_assist` | Windows | standard | Zoho Assist logs and configuration. |
| `windows.rmm.ultraviewer` | Windows | standard | UltraViewer connection and service logs. |
| `windows.rmm.other` | Windows | standard | Logs of further remote access tools (Supremo, Radmin, Remote Utilities, RealVNC, TightVNC, GoTo, DWAgent). |
| `windows.av.sophos` | Windows | standard | Sophos endpoint logs. |
| `windows.av.symantec` | Windows | standard | Symantec Endpoint Protection logs and registration information. |
| `windows.av.trellix` | Windows | standard | Trellix / McAfee Endpoint Security, VirusScan and agent logs. |
| `windows.av.eset` | Windows | standard | ESET endpoint and management agent logs. |
| `windows.av.bitdefender` | Windows | standard | Bitdefender endpoint logs. |
| `windows.av.trendmicro` | Windows | standard | Trend Micro Security Agent reports and connection logs. |
| `windows.av.malwarebytes` | Windows | standard | Malwarebytes service logs and scan results. |
| `windows.av.kaspersky` | Windows | standard | Kaspersky Endpoint Security trace logs (only present when tracing is enabled; detections are in the Kaspersky event log). |
| `windows.powershell.transcripts` | Windows | standard | PowerShell transcripts in the default locations (Start-Transcript and Group Policy transcription without an output directory) and unsaved PowerShell ISE scripts. |
| `windows.powershell.profiles` | Windows | standard | PowerShell profile scripts of Windows PowerShell and PowerShell 7 (machine and user). |
| `windows.wer` | Windows | standard | Windows Error Reporting reports (crash time, faulting module and process path; exploitation and crashing malware). Memory dumps are not collected. |
| `windows.cryptnet_url_cache` | Windows | standard | CryptnetUrlCache metadata (URL, time and size of files downloaded through CryptoAPI, e.g. certutil -urlcache). |
| `windows.notepad` | Windows | standard | Windows 11 Notepad tab and window state (unsaved text) and Notepad++ session and backups. May contain arbitrary user text. |
| `windows.lateral.impacket` | Windows | standard | Output and batch files left by Impacket smbexec, wmiexec and dcomexec (lateral movement). |
| `windows.appcompat.custom_shims` | Windows | standard | Custom application compatibility shim databases (shim persistence). |
| `windows.office.persistence` | Windows | standard | Office startup folders, add-ins, the Normal template and Outlook VBA project (Office persistence). |
| `windows.rdp_client_files` | Windows | standard | Outgoing RDP connection files (Default.rdp holds the last target) and Remote Desktop Manager connection log (not the credential-bearing .rdg files). |
| `windows.vpn_clients` | Windows | standard | VPN client logs (FortiClient, GlobalProtect, Ivanti/Pulse Secure, OpenVPN); no configuration files, which can embed keys. |
| `windows.network_scanners` | Windows | standard | Advanced IP Scanner and Advanced Port Scanner remnants (discovery phase). |
| `windows.capability_access` | Windows | standard | Capability Access Manager database (per-application microphone, camera and location use; Windows 11). |
| `windows.startupinfo` | Windows | standard | StartupInfo XML files (processes started shortly after logon, with command lines). |
| `windows.logs.debug` | Windows | standard | Logs below %SystemRoot%\debug (netlogon.log for Zerologon and secure channel failures, NetSetup.LOG for domain joins, MSRT log). |
| `windows.ad.sysvol` | Windows | standard | Domain controller SYSVOL policies and NETLOGON scripts (GPO-based ransomware deployment, logon scripts, Group Policy Preferences). Binaries in NETLOGON are not collected; they are listed in $MFT. |
| `windows.iis.config` | Windows | standard | IIS configuration (modules and handlers, IIS backdoors), automatic configuration history snapshots, web.config files and HTTP.sys error logs. |
| `windows.exchange.webshells` | Windows | standard | Script files in the Exchange webshell locations (owa/auth, ecp/auth, aspnet_client) and the Exchange setup log. |
| `windows.exchange.logs` | Windows | full | Exchange HttpProxy, ECP, OAB generator and transport logs (ProxyLogon/ProxyShell investigations). Often several GB. |
| `windows.mssql.errorlog` | Windows | standard | SQL Server error logs (sa brute force, xp_cmdshell enablement, CLR assemblies). |
| `windows.dhcp_server` | Windows | standard | DHCP server audit logs (IP to MAC and host name during the attack window; 7 days). |
| `windows.backup.veeam` | Windows | standard | Veeam Backup and Replication service log (credential theft and backup deletion before encryption). |
| `windows.web.tomcat` | Windows | standard | Apache Tomcat logs (application server exploitation, manager application uploads). |
| `windows.dns_server` | Windows | full | DNS server debug log (only if enabled; can reach 500 MB) and zone files. |
| `windows.ntfs.txf` | Windows | full | NTFS transactional log ($TxfLog $Tops:$T) of the system drive (raw NTFS access). |
| `windows.syscache` | Windows | full | Syscache.hve (execution evidence on Windows 7 / Server 2008 R2 era systems). |
| `windows.staging` | Windows | full | Scripts and executables in temporary and staging folders (attacker tools, RMM-pushed scripts). Unbounded; may contain malware. |
| `windows.etl` | Windows | full | Event trace logs (boot and shutdown process traces, WFP diagnostics, sleep study). Tens to hundreds of MB. |
| `windows.update_logs` | Windows | full | Component servicing, Windows Update and setup logs (patch level, install and upgrade times). |
| `windows.cloud_sync` | Windows | full | OneDrive sync logs and settings (exfiltration or insider activity through sanctioned sync). |
| `windows.old` | Windows | full | Registry hives, event logs and scheduled tasks of the previous installation in Windows.old (after an in-place upgrade). |
| `windows.sticky_notes` | Windows | full | Sticky Notes database (user text). |

## Linux

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `linux.accounts` | Linux | minimal | Local users and groups with their backup copies (the difference shows the last change), sudo/doas and PAM security configuration. |
| `linux.accounts.shadow` | Linux | standard | Shadow password files with backup copies and password history (password hashes and change dates). |
| `linux.logs.auth` | Linux | minimal | Authentication, system, cron and audit logs including rotated files, login records (utmp, wtmp/btmp/lastlog, wtmpdb, lastlog2), logind sessions, faillock tallies and first sudo use per user. |
| `linux.logs.var_log` | Linux | standard | Everything below /var/log except the systemd journal. |
| `linux.logs.journal` | Linux | standard | Persistent and volatile systemd journal files. |
| `linux.persistence.cron` | Linux | minimal | cron, anacron, at, incron and BusyBox/Alpine job definitions and spool directories. |
| `linux.persistence.systemd` | Linux | minimal | Local, runtime, transient and generated systemd units and configuration, user units, lingering users and timer stamps (last run). |
| `linux.persistence.systemd_vendor` | Linux | standard | Vendor-supplied systemd units, drop-ins and generators below /usr/lib/systemd (planted units are found by comparing hashes with the package md5sums). |
| `linux.persistence.init` | Linux | minimal | SysV/OpenRC init scripts and runlevel links, rc.local, inetd/xinetd, system-wide shell startup files (bash, zsh, csh, fish), dynamic linker preload, kernel module, udev, binfmt, environment, motd, X session and autostart configuration. |
| `linux.system_info` | Linux | minimal | Host identity, release, time zone, mounts, name resolution, PAM, syslog, sysctl and package sources. |
| `linux.packages` | Linux | standard | Installed package inventory (dpkg status) and package manager logs. |
| `linux.packages.rpmdb` | Linux | full | RPM package database (can be 100 MB or more). |
| `linux.web_server_config` | Linux | standard | nginx and Apache httpd configuration. |
| `linux.live.kernel` | Linux | standard | Snapshot of kernel and network state from /proc (loaded modules, taint flags, mounts, TCP/UDP/raw/packet sockets, ARP and routes, core_pattern and modprobe hooks, binfmt_misc handlers). Per-process data needs a future process source. |
| `linux.persistence.package_hooks` | Linux | minimal | Package manager hooks and plugins that run as root on every apt, yum, dnf or pacman run. |
| `linux.persistence.pam_modules` | Linux | standard | PAM module libraries (backdoored pam_unix.so or rogue modules such as Plague; compare hashes with the package md5sums). |
| `linux.persistence.boot` | Linux | standard | Boot loader and initramfs configuration (GRUB, boot loader specification entries, kernel command line, initramfs-tools and dracut hooks). |
| `linux.persistence.network_hooks` | Linux | standard | Scripts run on network events (NetworkManager and networkd dispatchers, ifupdown, dhclient and PPP hooks). |
| `linux.persistence.dbus_polkit` | Linux | standard | polkit rules and D-Bus service activation files (root services started on demand, permissive authorization rules). |
| `linux.persistence.python_hooks` | Linux | standard | Python startup hooks (.pth files, sitecustomize and usercustomize). Legitimate .pth files exist; review their content. |
| `linux.packages.dpkg_info` | Linux | standard | dpkg maintainer scripts, md5sums (offline integrity check), conffiles, diversions, statoverrides and package status backups. |
| `linux.security_config` | Linux | standard | Audit, logging and access control configuration (auditd rules and plugins, logrotate, syslog-ng, SELinux, AppArmor, iptables/nftables/ufw/firewalld, fail2ban); shows defense evasion before encryption. |
| `linux.network_config` | Linux | standard | DHCP leases (IP address history) and NetworkManager and network-scripts configuration (no Wi-Fi or VPN secrets). |
| `linux.directory_services` | Linux | standard | Kerberos, realmd, Samba and LDAP client configuration (domain membership). SSSD configuration is not collected because it can hold bind passwords. |
| `linux.containers.config` | Linux | standard | Container configuration (Docker containers and daemon, containerd, Podman and Quadlet units, Kubernetes static pods and kubelet, k3s and RKE2 manifests); shows privileged containers, host mounts and container persistence. Environment variables in container configs can hold secrets. |
| `linux.containers.logs` | Linux | full | Container logs (Docker json-file and local drivers, Kubernetes pod logs, Podman). Can be very large. |
| `linux.cloud_agents` | Linux | standard | cloud-init user data and per-boot scripts, AWS SSM Run Command scripts and output, Azure run-command and custom script downloads (commands run with stolen cloud credentials). User data and command output can hold secrets. |
| `linux.web.app_config` | Linux | standard | PHP and PHP-FPM configuration, .htaccess and .user.ini files in web roots (auto_prepend_file and handler remaps used by webshells), Tomcat configuration (not tomcat-users.xml, which holds passwords) and compiled JSP files that survive deletion of the .jsp. |
| `linux.logs.apps` | Linux | standard | Application server logs outside /var/log (Tomcat, source-built nginx and Apache, XAMPP, RHEL PostgreSQL logs and configuration). |
| `linux.appliance.vcenter` | Linux | standard | VMware vCenter Server Appliance logs (SSO, vpxd, proxies, appliance management) and configuration. /var/log/vmware is a symbolic link on the appliance, so linux.logs.var_log does not reach them. |
| `linux.mail.spool` | Linux | standard | root's mailbox (output of cron jobs, often revealing miners and backdoors). |
| `linux.locate_db` | Linux | standard | locate databases (file names at the last updatedb run, including since deleted attacker tools). |
| `linux.desktop.user_activity` | Linux | standard | Recently used files, GVFS metadata, trash information, X session errors and AccountsService user records (desktop systems). |
| `linux.remote_access_tools` | Linux | standard | Logs of remote access tools on Linux (AnyDesk, TeamViewer, RustDesk, Splashtop); no configuration with access passwords. |
| `linux.staging_dirs` | Linux | full | Contents of /tmp, /var/tmp (including systemd private temporary directories of services) and /dev/shm, where droppers, miners, tunnels and exfiltration archives are staged. Unbounded; may contain malware. |
| `linux.binaries.local` | Linux | full | Locally installed binaries (/usr/local/bin, /usr/local/sbin) and out-of-tree kernel modules (extra, updates). 10 to 500 MB. |
| `linux.boot.images` | Linux | full | Kernel, initramfs and EFI boot images (bootkits, modified initramfs). 30 to 150 MB per image. |

## Linux and macOS

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `unix.shell_history` | Linux, macOS | minimal | Shell, REPL and tool histories of every user (bash, zsh, fish, python, php, ruby, mysql, psql, mongo, redis, less, vim, nano, wget). |
| `unix.shell_config` | Linux, macOS | standard | Per-user shell startup files and git configuration. |
| `unix.ssh` | Linux, macOS | minimal | SSH server/client configuration, host public keys, login scripts (sshrc, ~/.ssh/rc, ~/.ssh/environment), per-user authorized_keys and known_hosts (with backup copies), public keys and rhosts/equiv trust files (no private keys). |

## macOS

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `macos.system_info` | macOS | minimal | macOS version, setup date, local accounts files, hosts, sudoers, PATH, log, firewall (pf), NFS and network configuration, DHCP leases, login window preferences. |
| `macos.accounts` | macOS | standard | Local directory service user records (include password hash data), groups (admin, SSH and screen sharing access) and file shares. |
| `macos.persistence` | macOS | minimal | Launch agents and daemons, startup items, login/background items, launchd overrides, login hooks, re-opened apps, periodic and rc scripts, global shell startup files, cron/at jobs and emond rules. |
| `macos.logs` | macOS | minimal | Classic log files below /private/var/log and crash/diagnostic reports. |
| `macos.logs.all` | macOS | full | All application logs below /Library/Logs and the users' Library/Logs. |
| `macos.unified_logs` | macOS | standard | Unified logging tracev3 files and uuidtext (parse with 'log show --archive' after rebuilding a .logarchive). Often 1 GB or more. |
| `macos.fsevents` | macOS | standard | File system event logs (.fseventsd) of the system and data volumes. |
| `macos.quarantine_events` | macOS | standard | LaunchServices quarantine events database (downloaded files and their origin). |
| `macos.knowledgec` | macOS | standard | KnowledgeC databases (application usage, device state). TCC protected. |
| `macos.tcc` | macOS | standard | Transparency, Consent and Control databases (granted privacy permissions) and MDM privacy overrides. Requires Full Disk Access. |
| `macos.install_history` | macOS | standard | Software installation history and package receipts. |
| `macos.recent_items` | macOS | standard | Shared file lists (recent documents, applications, servers) and Finder preferences. |
| `macos.gatekeeper` | macOS | standard | Gatekeeper and syspolicyd databases (execution provenance of downloaded apps, Gatekeeper rules and rejects, kext approvals, notarization tickets). Needs root and Full Disk Access. |
| `macos.xprotect` | macOS | standard | XProtect behavior service database (processes that accessed protected data such as browser profiles, macOS 13+) and XProtect, XProtect Remediator and MRT versions. Needs root and Full Disk Access. |
| `macos.coreanalytics` | macOS | standard | CoreAnalytics aggregates (about a month of program executions, also of deleted binaries). The daily .core_analytics reports are part of macos.logs. |
| `macos.terminal_state` | macOS | standard | Terminal and iTerm2 window restoration state, which keeps the scrollback (commands and output) even when shell history was cleared. May contain secrets. |
| `macos.persistence.user_apps` | macOS | standard | Persistence through application settings and scripts (Dock, Terminal and iTerm2 startup commands, iTerm2 AutoLaunch scripts, Folder Actions, xbar plugins, Hammerspoon). |
| `macos.persistence.auth` | macOS | standard | Authentication persistence and credential capture points (PAM configuration and modules, authorization database, SecurityAgent and DirectoryServices plugins). |
| `macos.persistence.plugins` | macOS | standard | Info.plist and main executable of plugins loaded by system processes (QuickLook, Spotlight importers, screen savers, color pickers, preference panes, input methods, scripting additions, audio HAL drivers). |
| `macos.privileged_helpers` | macOS | standard | Privileged helper tools (root helpers installed with SMJobBless, referenced by launch daemons; common for adware and backdoors). |
| `macos.extensions` | macOS | standard | System extension database and Info.plist of system extensions (network filters, VPN, endpoint security) and kernel extensions. |
| `macos.managed_config` | macOS | standard | Managed preferences and configuration profile store (rogue profiles, forced browser extensions, proxies, root certificates, privacy grants). The store needs Full Disk Access. |
| `macos.preferences` | macOS | standard | System-wide preferences below /Library/Preferences (known Wi-Fi networks, Bluetooth, Time Machine, VPN and network extensions, remote management, deleted users, software update). |
| `macos.network_usage` | macOS | standard | Network usage database (bytes sent and received per process with first and last use; exfiltration and C2 evidence). Needs Full Disk Access. |
| `macos.remote_access_tools` | macOS | standard | Apple Remote Desktop caches, Screen Sharing and Microsoft Remote Desktop client targets, AnyDesk, TeamViewer, RustDesk and Splashtop logs (no access passwords). |
| `macos.login_records` | macOS | standard | Login records (utmpx), last sudo use per user and terminal, BSM audit trail and audit configuration (audit_warn is also a persistence hook). |
| `macos.devices` | macOS | standard | Paired Bluetooth devices and connected iOS devices. |
| `macos.app_inventory` | macOS | standard | Info.plist of installed applications and their embedded login items (name, bundle ID, version). |
| `macos.spotlight_shortcuts` | macOS | standard | Spotlight search shortcuts (what the user typed to open applications and files). |
| `macos.biome` | macOS | standard | Biome streams App.InFocus (foreground applications), App.WebUsage and App.MenuItem (selected menu items), the successor of KnowledgeC usage data. Needs Full Disk Access. |
| `macos.biome.all` | macOS | full | All Biome streams of users and the system (can be hundreds of MB). Needs Full Disk Access. |
| `macos.powerlog` | macOS | full | PowerLog database and archives (application and process energy use, sleep and wake timeline). Often 50 MB or more. |
| `macos.screentime` | macOS | full | Screen Time databases (per-application usage, only if Screen Time is enabled). Needs Full Disk Access. |
| `macos.icloud_drive` | macOS | full | iCloud Drive client and server databases (file inventory and sync state). |
| `macos.internet_accounts` | macOS | full | Configured internet accounts (Apple, Google, Exchange and others; no passwords). |
| `macos.install_history.bom` | macOS | full | Package receipt bill-of-materials files (which package installed which file). 100 MB or more. |

## Browsers (all platforms)

| Artifact | Platforms | Smallest profile | Description |
|---|---|---|---|
| `browser.chrome` | Windows, Linux, macOS | standard | Google Chrome history, bookmarks, preferences, shortcuts, top sites and extension manifests of all profiles. |
| `browser.edge` | Windows, Linux, macOS | standard | Microsoft Edge (Chromium) history, bookmarks, preferences and extension manifests of all profiles. |
| `browser.brave` | Windows, Linux, macOS | standard | Brave browser history, bookmarks and preferences of all profiles. |
| `browser.firefox` | Windows, Linux, macOS | standard | Mozilla Firefox history, downloads, form history, bookmarks backups, extensions and session state of all profiles. |
| `browser.safari` | macOS | standard | Safari history, downloads, bookmarks and last session. Requires Full Disk Access. |

