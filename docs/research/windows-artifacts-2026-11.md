# Relic Windows catalog: gap analysis and recommendations

> Research notes from 2026-11-07 that informed the catalog extension in this
> release. The catalog does not adopt every recommendation: credential
> stores (rclone/ngrok configs, keychains, cloud CLI credentials, RDP
> connection managers, crash dumps, NTDS.dit) stay out of the built-in
> catalog, and items that need engine features (per-source size caps,
> opt-in artifacts, a process source) are listed in `docs/design.md`.
> The authoritative paths are in `internal/catalog/data/*.yaml`.

Research date: 2026-11-07. Scope: Windows only, file sources only (schema v1).

Baseline: `internal/catalog/data/windows.yaml` + `browsers.yaml` (33 Windows artifacts).

Reference sets compared:
- KapeFiles @ `576dc9a` (2026-11-05): `KapeTriage` and `!SANS_Triage` were expanded recursively to their leaf
  `.tkape` targets (1,360 leaf entries). Other targets checked: `ServerTriage`, `Exchange`, `RemoteAdmin`,
  `Antivirus` and the `Apps/*` RMM targets.
- ForensicArtifacts @ `069e812` (2026-11-01): `windows.yaml`, `applications.yaml`, `antivirus.yaml`, `webservers.yaml`.
- Vendor documentation and DFIR write-ups, cited per item (Microsoft, Synacktiv, Huntress, Velociraptor exchange,
  Fox-IT/NCC, Securelist, Datto, N-able, NinjaOne, Kaspersky, ngrok, impacket source).

Notation:
- **Profile**: `minimal` < `standard` < `full`. An RMM/AV artifact costs only a few directory lookups when the
  tool is absent, so these artifacts are recommended for `standard` even though they are niche.
- **Conf**: confidence that the path is correct (high = authoritative source plus a second source or vendor doc).
- `%UserHome%` already covers systemprofile, LocalService and NetworkService (ProfileList). It does **not** cover
  `%SystemRoot%\SysWOW64\config\systemprofile`, the profile that 32-bit processes running as SYSTEM are redirected to.
- Patterns avoid `\` inside `{}` alternatives. If the parser accepts separators there, several lines can be merged.

---

## 0. Cross-cutting recommendations (engine/catalog level)

| # | Change | Why |
|---|---|---|
| X1 | Add `%SystemRoot%\SysWOW64\config\systemprofile` to the `%UserHome%` expansion when it exists | KAPE and ForensicArtifacts list it separately for PSReadLine, AnyDesk, CryptnetUrlCache, rclone, UltraViewer and Advanced IP Scanner. Many RMM agents are 32-bit and run as SYSTEM, so their children write here. One change fixes this gap for every per-user artifact. |
| X2 | Add an `opt_in: true` artifact flag (never selected by `*` or a profile, only by name) | Some artifacts must exist but must not ride along with `full`: NTDS.dit, hiberfil/pagefile, PST/OST, Recall, quarantined malware, `$Recycle.Bin` content (section 4). |
| X3 | Add a per-source `max_file_size` | Lets `standard` include folders such as `Windows\Temp`, WER and SYSVOL while skipping their rare multi-GB files. |
| X4 | Collect from all fixed NTFS volumes (`%Volume%` variable or a `volumes: all` option) for `$MFT`, `$J`, `$LogFile` and `$Recycle.Bin\*\$I*` | On file servers the encryption sweep happens on D:/E:. `$J` on the data volume is the encryption timeline. |
| X5 | Add `Windows.old` variants as one full-profile artifact (see 3.17) | KAPE duplicates hives, EVTX, Tasks and WBEM under `C:\Windows.old\Windows\...`. This matters after an in-place upgrade. |

---

## 1. Fixes to existing artifacts (do first; small, high value)

### F1 `windows.remote_access_tools`: AnyDesk and TeamViewer miss their most important files
Suggest splitting it into `windows.rmm.anydesk` and `windows.rmm.teamviewer` to match the naming below.

The current globs `*.{trace,conf}` (AnyDesk) and `%ProgramFiles%\TeamViewer\*.log` miss the **inbound connection records**.

Add for AnyDesk:
```
%ProgramData%\AnyDesk\connection_trace.txt
%ProgramData%\AnyDesk\file_transfer_trace.txt
%ProgramData%\AnyDesk\ad_*\*.trace
%UserHome%\AppData\Roaming\AnyDesk\connection_trace.txt
%UserHome%\AppData\Roaming\AnyDesk\file_transfer_trace.txt
%UserHome%\AppData\Roaming\AnyDesk\ad_*\*.trace
%UserHome%\AppData\Roaming\AnyDesk\chat\*.txt
%SystemRoot%\SysWOW64\config\systemprofile\AppData\Roaming\AnyDesk\*      # unless X1
```

Add for TeamViewer:
```
%ProgramFiles%\TeamViewer\Connections*.txt              # Connections_incoming.txt = target-side inbound sessions
%ProgramFiles(x86)%\TeamViewer\Connections*.txt
%UserHome%\AppData\Local\TeamViewer\Database\tvchatfilecache.db
%UserHome%\AppData\Local\TeamViewer\RemotePrinting\tvprint.db
```

Sensitivity: AnyDesk `service.conf`, which the current `*.conf` glob collects, holds the password hash, the salt and
the client **private key** (Synacktiv). Add an exclude for `service.conf` (keep `system.conf` and `user.conf`), or
document it. The existing `windows.ssh` artifact already promises "no private keys".

Sources: KapeFiles `Apps/AnyDesk.tkape` and `Apps/TeamViewerLogs.tkape`; ForensicArtifacts `AnyDeskLogFile` and
`TeamViewerConnectionLogFile`; Synacktiv "Legitimate RATs". Conf: high.

### F2 `windows.defender`: add MpCmdRun log, quarantine metadata and legacy SCEP logs
```
%SystemRoot%\Temp\MpCmdRun.log
%UserHome%\AppData\Local\Temp\MpCmdRun.log                 # covers LocalService/systemprofile via %UserHome%
%ProgramData%\Microsoft\Windows Defender\Quarantine\Entries\*     # RC4-encrypted metadata (path, threat, time), KB
%ProgramData%\Microsoft\Microsoft AntiMalware\Support\*.log       # SCEP / Server 2012 R2
```
MpCmdRun.log records attacker use of `MpCmdRun -RemoveDefinitions -All` and `-DownloadFile` (LOLBin).
`Quarantine\ResourceData\**` holds the encrypted quarantined malware itself. Put it in a separate opt-in artifact
(section 4).

Sources: KAPE `Antivirus/WindowsDefender.tkape`; ForensicArtifacts `MicrosoftAVLogs` and `MicrosoftAVQuarantine`;
Fox-IT/NCC "Windows Defender Quarantine Forensics". Conf: high.

### F3 `windows.firewall_logs`: include rotated logs
Change the glob to `%SystemRoot%\System32\LogFiles\Firewall\*.log*`. KAPE uses `pfirewall.*`. On rollover Windows
keeps `pfirewall.log.old`, which `*.log` misses. Conf: high for KAPE's glob; medium for the `.old` naming (from
memory, not doc-verified). The wider glob is harmless either way.

### F4 `windows.group_policy`: add the client-side GPP history cache
```
%ProgramData%\Microsoft\Group Policy\History\**
%UserHome%\AppData\Local\Microsoft\Group Policy\History\**
```
These are copies of the domain GPP files applied to this host (ScheduledTasks.xml, Files.xml, Services.xml,
Groups.xml). Ransomware pushed by GPO is visible on every endpoint, not only on the DC. Sensitive: legacy
`cpassword` in Groups.xml. Size: KB.

Source: KAPE `Windows/GroupPolicy.tkape` (in `!SANS_Triage`). Conf: high.

### F5 `windows.scheduled_tasks`: add PowerShell scheduled jobs
```
%UserHome%\AppData\Local\Microsoft\Windows\PowerShell\ScheduledJobs\**
%SystemRoot%\SysWOW64\config\systemprofile\AppData\Local\Microsoft\Windows\PowerShell\ScheduledJobs\**   # unless X1
%SystemRoot%\SysWOW64\Tasks\**
```
Source: KAPE `Windows/ScheduledTasks.tkape`. Conf: high.

### F6 `windows.powershell_history`: add the 32-bit SYSTEM profile (if X1 is not done)
`%SystemRoot%\SysWOW64\config\systemprofile\AppData\Roaming\Microsoft\Windows\PowerShell\PSReadLine\*_history.txt`.
Source: KAPE `Logs/PowerShellConsole.tkape`. Conf: high. History for service accounts in System32 is already covered
by `%UserHome%`.

### F7 `windows.recent_files`: add Desktop and Start Menu shortcuts
```
%UserHome%\Desktop\*.lnk
%UserHome%\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\**\*.lnk
%ProgramData%\Microsoft\Windows\Start Menu\Programs\**\*.lnk
```
Value: shortcut-modification persistence (T1547.009) and evidence of installed tools. Size: KB. Source: KAPE
`Windows/LNKFilesAndJumpLists.tkape`. Conf: high.

### F8 `windows.iis_logs`: add legacy and custom log roots
```
%SystemRoot%\System32\LogFiles\W3SVC*\*.log
%SystemDrive%\Resources\Directory\*\LogFiles\Web\W3SVC*\*.log     # Azure cloud services, niche
```
Move HTTPERR into its own standard artifact (3.1). Sources: KAPE `Logs/IISLogFiles.tkape`; ForensicArtifacts
`MicrosoftIISLogs`. Conf: high.

### F9 (low) `windows.registry.system`: other hives, full profile only
`%SystemRoot%\System32\config\{COMPONENTS,DRIVERS,BBI,ELAM}` with `.LOG1`/`.LOG2`. Source: KAPE
`RegistryHivesOther`. Low IR value.

---

## 2. Top 15 new artifacts, ranked for ransomware and intrusion IR

### 1. `windows.rmm.screenconnect`: category `remote_access`, profile standard
ConnectWise ScreenConnect client and server traces: commands run, files pushed, server sessions and users, and
App_Extensions webshells.

Client paths:
```
%SystemRoot%\Temp\ScreenConnect\**                         # run.ps1/.cmd from "Commands"/toolbox, per client version
%UserHome%\Documents\ConnectWiseControl\Temp\**            # files pushed and executed through the session
%ProgramData%\ScreenConnect Client*\user.config
%ProgramFiles(x86)%\ScreenConnect Client (*)\*.config      # server and relay; medium conf
%ProgramFiles%\ScreenConnect Client (*)\*.config
```

Server paths (MSP or self-hosted, CVE-2024-1709/1708):
```
%ProgramFiles(x86)%\ScreenConnect\App_Data\Session.db*
%ProgramFiles(x86)%\ScreenConnect\App_Data\*.xml           # User.xml (accounts), Role.xml
%ProgramFiles(x86)%\ScreenConnect\App_Extensions\**\*.{aspx,ashx}
%ProgramFiles(x86)%\ScreenConnect\web.config
(repeat the four server lines with %ProgramFiles%)
```

- Full profile only: `%UserHome%\Documents\ConnectWiseControl\Files\**` (transferred files, may be large).
- Value: the #1 abused RMM in ransomware and initial-access cases. It shows what was executed or dropped through the
  session and which operator accounts exist on a compromised server. Client session events are in the Application
  EVTX (already collected).
- Size: client KB–MB. Server `Session.db` is MB to several GB on large MSPs.
- Sensitive: `Session.db` holds MSP customer and session data; `User.xml` holds password hashes.
- Conf: high (Velociraptor `ScreenConnect` exchange artifact; KAPE `Apps/ScreenConnect.tkape`; Huntress; Sigma
  `file_event_win_remote_access_tools_screenconnect_remote_file`). Medium for the client `*.config` naming.

### 2. `windows.av.*`: third-party AV/EDR logs, category `security` (new), profile standard
Early detections (Cobalt Strike, Mimikatz, AdFind, SystemBC) often precede encryption by days, and the logs also show
tampering. One artifact per vendor, all paths from KAPE `Targets/Antivirus/*.tkape`. Conf: high unless noted.
```
windows.av.sophos       %ProgramData%\Sophos\*\Logs\**
                        %ProgramData%\Sophos\Logs\**
windows.av.symantec     %ProgramData%\Symantec\Symantec Endpoint Protection\*\Data\Logs\**
                        %ProgramData%\Symantec\Symantec Endpoint Protection\*\Data\registrationInfo.xml
                        %UserHome%\AppData\Local\Symantec\Symantec Endpoint Protection\Logs\**
windows.av.trellix      %ProgramData%\McAfee\Endpoint Security\Logs\**
                        %ProgramData%\McAfee\Endpoint Security\Logs_Old\**
                        %ProgramData%\McAfee\DesktopProtection\**
                        %ProgramData%\McAfee\VirusScan\**
                        %ProgramData%\McAfee\Agent\logs\**
                        %ProgramData%\McAfee\Agent\AgentEvents\**
windows.av.eset         %ProgramData%\ESET\ESET Security\Logs\**
                        %ProgramData%\ESET\ESET NOD32 Antivirus\Logs\**
                        %ProgramData%\ESET\RemoteAdministrator\Agent\EraAgentApplicationData\Logs\*
windows.av.bitdefender  %ProgramData%\Bitdefender\Endpoint Security\Logs\**
                        %ProgramData%\Bitdefender\Desktop\Profiles\Logs\**
windows.av.trendmicro   %ProgramFiles(x86)%\Trend Micro\Security Agent\Report\*.log
                        %ProgramFiles(x86)%\Trend Micro\Security Agent\ConnLog\*.log      (+ %ProgramFiles%)
windows.av.malwarebytes %ProgramData%\Malwarebytes\MBAMService\logs\mbamservice.log*
                        %ProgramData%\Malwarebytes\MBAMService\ScanResults\**
                        %ProgramData%\Malwarebytes\Malwarebytes Anti-Malware\Logs\mbam-log-*.xml
windows.av.sentinelone  %ProgramData%\Sentinel\logs\**                 (mostly binary/encrypted; low value)
windows.av.kaspersky    %ProgramData%\Kaspersky Lab\KES\Traces\*.log  (11.x)
                        %ProgramData%\Kaspersky Lab\KES.*\Traces\*.log (12.x)   -- medium conf
```
- Kaspersky traces exist only when tracing is enabled. Detections go to `Kaspersky Event Log.evtx`, which is
  already collected (Kaspersky support docs 124710).
- Full profile only (quarantined malware): `%ProgramData%\Symantec\Symantec Endpoint Protection\*\Data\Quarantine\**`,
  `%UserHome%\AppData\Local\ESET\ESET Security\Quarantine\**`, `%SystemRoot%\System32\Drivers\CrowdStrike\Quarantine\**`,
  `%ProgramFiles(x86)%\Trend Micro\*\Quarantine\*` (and the same under `%ProgramFiles%`).
- CrowdStrike is otherwise not file-based.
- Size: MB to a few hundred MB. Sophos and Trend can be large, so cap with X3. Not sensitive (some user paths).

### 3. `windows.rmm.splashtop`: category `remote_access`, profile standard
Splashtop Streamer logs: sessions with client IP, file transfers and chat. Atera's remote access is Splashtop.
```
%ProgramFiles(x86)%\Splashtop\Splashtop Remote\Server\log\*        # SPLog.txt, agent_log.txt, svcinfo/sysinfo.txt
%ProgramFiles%\Splashtop\Splashtop Remote\Server\log\*
%ProgramData%\Splashtop\Temp\log\*                                 # FTCLog.txt = file transfers with user + IP
%ProgramFiles(x86)%\Splashtop\Splashtop Remote\Splashtop Gateway\log\**
%ProgramData%\Splashtop\Splashtop Remote Client for ST*\*\log\**
```
- Do not collect `Server\db\SRAgent.sqlite3` (encrypted) or `%ProgramData%\Splashtop\Splashtop Remote Server\Credential\`
  (key material).
- The `Splashtop-Splashtop Streamer-*%4Operational.evtx` channels are already covered by `windows.eventlogs`.
- Size: MB. Conf: high (Synacktiv; KAPE `Apps/Splashtop.tkape`).

### 4. `windows.rmm.atera`: category `remote_access`, profile standard
Atera agent package logs. `AgentPackageRunCommandInteractive\log.txt` records remote commands **and their output**.
```
%ProgramFiles%\ATERA Networks\AteraAgent\**\*.{txt,log,ini,config,cfg}
%ProgramFiles(x86)%\ATERA Networks\AteraAgent\**\*.{txt,log,ini,config,cfg}
```
- Atera remote scripts run as random-named `.bat` files in `%SystemRoot%\Temp` (see 3.15).
- Size: MB. Conf: high (Synacktiv; KAPE `Apps/AteraAgent.tkape`).

### 5. `windows.rmm.<agent>`: MSP/RMM agent logs, category `remote_access`, profile standard
Attackers abuse an existing MSP agent (or install one) to push scripts and ransomware to every host.
```
windows.rmm.ninjaone   %ProgramData%\NinjaRMMAgent\*.txt
                       %ProgramData%\NinjaRMMAgent\logs\*               -- medium conf (vendor doc lists the folder
                                                                            and NinjaRMMAgent_<date>_<time>.txt; Windows
                                                                            subfolder layout unconfirmed)
windows.rmm.datto      %ProgramData%\CentraStage\AEMAgent\DataLog\**     # aemagent.log + Archives
                       %ProgramFiles(x86)%\CentraStage\log.txt           # CagService log (64-bit OS)
                       %ProgramFiles(x86)%\CentraStage\archives\*         -- archive dir name inferred, medium
                       %ProgramFiles%\CentraStage\log.txt                 # 32-bit OS
                       %ProgramData%\CentraStage\Logs\*                   # service log moved here on uninstall
windows.rmm.nable      %ProgramFiles(x86)%\N-Able Technologies\Windows Agent\log\*
                       %ProgramFiles(x86)%\N-Able Technologies\Windows Agent\config\*     -- medium conf
windows.rmm.kaseya     %ProgramData%\Kaseya\Log\**
                       %ProgramFiles(x86)%\Kaseya\*\agentmon.log*
                       %ProgramFiles%\Kaseya\*\agentmon.log*
                       %UserHome%\AppData\Local\Kaseya\Log\KaseyaLiveConnect\**
                       %SystemRoot%\Temp\KASetup.log
                       %UserHome%\AppData\Local\Temp\KASetup.log
                       %SystemDrive%\kworking\**\*.{log,txt,bat,cmd,ps1,vbs,crt}  -- agent working dir (REvil 2021
                                                                                    dropped agent.crt/agent.exe here);
                                                                                    medium conf, not in KAPE
windows.rmm.action1    %SystemRoot%\Action1\logs\*.log
                       %SystemRoot%\Action1\scripts\**
                       %SystemRoot%\Action1\rule_data\**
windows.rmm.meshagent  %ProgramFiles%\Mesh Agent\**\*.{log,msh}           # MeshCentral; also used by Tactical RMM
                       %ProgramFiles%\TacticalAgent\agent.log             # verified in amidaware/rmmagent main.go
windows.rmm.level      %ProgramFiles%\Level\*.log
windows.rmm.immybot    %ProgramData%\ImmyBot\Logs\**
                       %ProgramData%\ImmyBot\Scripts\**
                       %ProgramData%\ImmyBotAgentService\config.json
                       %SystemRoot%\Temp\ImmyBot\**
```
- Sources: KAPE `Apps/{Kaseya,Action1,MeshAgent,Level,ImmyBot}.tkape`; Datto RMM help "Agent log files"; N-able
  "Windows Agent Log Collection"; NinjaOne "Finding and collecting NinjaOne log files"; Tactical RMM source.
- Not verified, so omitted: Syncro, GoTo Resolve, BeyondTrust/Bomgar, the PDQ Deploy runner folder, and the Datto
  `CentraStage\Packages` component scripts. `$MFT` and the 7045 service-install events cover their presence.
- Size: KB to tens of MB each.

### 6. `windows.rmm.<tool>`: ad-hoc remote tools favoured by intruders, category `remote_access`, profile standard
```
windows.rmm.rustdesk     %UserHome%\AppData\Roaming\RustDesk\log\**        # incl. LocalService profile: log\server\
                         %UserHome%\AppData\Roaming\RustDesk\config\*      # IDs, relay server; encrypted password
windows.rmm.quick_assist %UserHome%\AppData\Local\Temp\QuickAssist\EBWebView\Default\{History,History-journal,Secure Preferences,Preferences,Network Action Predictor}
                         %UserHome%\AppData\Local\Temp\QuickAssist\EBWebView\Default\Network\DIPS*
                         %UserHome%\AppData\Local\Temp\QuickAssist\EBWebView\Default\Session Storage\*
                         (same three lines for Temp\RemoteHelp\...)        # Storm-1811 / Black Basta help-desk lures
windows.rmm.simplehelp   %ProgramData%\JWrapper-Remote Access\logs\*
                         %ProgramData%\SimpleHelp\logs\*
                         %UserHome%\AppData\Roaming\JWrapper-SimpleHelp Technician\logs\*
windows.rmm.logmein      %ProgramData%\LogMeIn\Logs\**
                         %UserHome%\AppData\Local\Temp\LogMeInLogs\**
windows.rmm.zoho_assist  %ProgramData%\ZohoMeeting\log\**
                         %ProgramData%\ZohoMeeting\*.conf
                         %UserHome%\AppData\Local\ZohoMeeting\log\**
                         %ProgramFiles(x86)%\ZohoMeeting\UnAttended\ZohoMeeting\logs\**    (+ %ProgramFiles%)
windows.rmm.ultraviewer  %UserHome%\AppData\Roaming\UltraViewer\**
                         %ProgramFiles(x86)%\UltraViewer\{UltraViewerService_log.txt,ConnectionLog.Log}
windows.rmm.supremo      %ProgramData%\SupremoRemoteDesktop\Log\*.log
windows.rmm.radmin       %SystemRoot%\System32\rserver30\Radm_log.htm
                         %SystemRoot%\SysWOW64\rserver30\Radm_log.htm
                         %SystemRoot%\System32\rserver30\CHATLOGS\*\*.htm
                         %SystemRoot%\SysWOW64\rserver30\CHATLOGS\*\*.htm
windows.rmm.remote_utilities %ProgramData%\Remote Utilities\Logs\rut_log_*.html
                         %ProgramFiles(x86)%\Remote Utilities - Host\Logs\rut_log_*.html
windows.rmm.vnc          %ProgramData%\RealVNC-Service\vncserver.log
                         %UserHome%\AppData\Local\RealVNC\{vncserver,vncviewer}.log
                         %ProgramData%\TightVNC\Server\Logs\*
windows.rmm.goto         %UserHome%\AppData\Roaming\GoTo\Logs\goto.log
windows.rmm.dwagent      %ProgramData%\DWAgent*\*.log*
windows.rmm.ammyy        %ProgramData%\Ammyy\**
```
- Sources: KAPE `Apps/*.tkape` (all lines); hackuponthegale "Quick Assist forensics" for the EBWebView file meanings;
  Gridinsoft (RustDesk `log\server`).
- Conf: high for KAPE paths. Medium for RustDesk `config` and Quick Assist file names.
- Size: KB–MB. Sensitive: chat logs, and RustDesk config (encrypted password).

### 7. `windows.exfil.tool_configs`: rclone, ngrok, cloudflared, MEGAsync; category `network`, profile standard
Configuration of exfiltration and tunnel tools: the attacker's Mega/S3/SFTP remote and tunnel tokens.
```
%UserHome%\AppData\Roaming\rclone\rclone.conf
%UserHome%\AppData\Local\rclone\rclone.conf
%UserHome%\.config\rclone\rclone.conf
%UserHome%\.rclone.conf
%UserHome%\AppData\Local\ngrok\ngrok.yml                    # ngrok v3 default
%UserHome%\.ngrok2\ngrok.yml                                # ngrok v2
%UserHome%\.cloudflared\*.{yml,yaml,json}                   # medium conf
%UserHome%\AppData\Local\Mega Limited\MEGAsync\logs\**      # medium conf; KAPE collects the whole folder
```
- Value: rclone and MEGAsync are the dominant ransomware exfiltration tools (The DFIR Report). The config names the
  destination account.
- Sensitive: contains credentials (mostly the attacker's). Document this explicitly, since browser credentials are
  intentionally excluded.
- Portable copies keep `rclone.conf` next to the exe. KAPE searches all of C:\ recursively for it, which conflicts
  with Relic's no-walk rule; rely on `$MFT` to locate them.
- Size: KB (MEGAsync logs MB).
- Sources: KAPE `Apps/RcloneConf.tkape`, `Apps/Megasync.tkape`; ngrok agent config docs.

### 8. `windows.wer`: Windows Error Reporting reports, category `logs`, profile standard
```
%ProgramData%\Microsoft\Windows\WER\**\*.{wer,xml,txt,csv}
%UserHome%\AppData\Local\Microsoft\Windows\WER\**\*.{wer,xml,txt,csv}
```
- Full profile only (`windows.wer.dumps`, sensitive):
  ```
  %ProgramData%\Microsoft\Windows\WER\**\*.{dmp,mdmp,hdmp}
  %UserHome%\AppData\Local\CrashDumps\*.dmp
  %SystemRoot%\MEMORY.DMP
  %SystemRoot%\Minidump\*.dmp
  ```
- Value: `Report.wer` (INI-style text) gives crash time, faulting module and full process path. Exploitation
  (w3wp, Exchange, spoolsv), crashing malware and LSASS dumping via WER ("Shtinkering") leave traces here.
  ReportArchive keeps history.
- Size: reports KB each (folder MB); dumps up to GB. Dumps may contain LSASS credentials.
- Conf: high. Sources: KAPE `Windows/WER.tkape` (in KapeTriage and !SANS_Triage); ForensicArtifacts `WindowsCrashDumps`.

### 9. `windows.ad.sysvol`: domain controller SYSVOL and NETLOGON, category `persistence`, profile standard
```
%SystemRoot%\SYSVOL\domain\Policies\**\*.{xml,ini,inf,pol,aas,cmt,ps1,psm1,bat,cmd,vbs,js,kix}
%SystemRoot%\SYSVOL\domain\scripts\**
```
- Full profile only: `%SystemRoot%\SYSVOL\domain\**`. Use `domain\`, not `sysvol\<fqdn>`, which is a junction
  (Relic does not follow junctions).
- Value: GPO-based ransomware deployment (immediate scheduled tasks, Files.xml payload copy, logon scripts), shown
  in Sophos "Identifying Group Policy attacks". Also covers GPP `cpassword` exposure.
- Size: typically < 50 MB. NETLOGON sometimes holds installers, hence the extension filter in standard plus X3.
- Sensitive: GPP cpassword.
- Conf: high (KAPE `Windows/ActiveDirectorySysvol.tkape`: `C:\Windows\SYSVOL` recursive). A custom SYSVOL location
  set during dcpromo needs registry-driven paths (roadmap).

### 10. `windows.exchange`: webshell directories and logs, category `server` (new)
Standard profile, `windows.exchange.webshell_dirs` (small, only exists on Exchange):
```
%ProgramFiles%\Microsoft\Exchange Server\V15\FrontEnd\HttpProxy\owa\auth\**\*.{aspx,ashx,asmx}
%ProgramFiles%\Microsoft\Exchange Server\V15\FrontEnd\HttpProxy\ecp\auth\**\*.{aspx,ashx,asmx}
%SystemDrive%\inetpub\wwwroot\aspnet_client\**\*.{aspx,ashx,asmx}
%SystemDrive%\ExchangeSetupLogs\ExchangeSetup.log
```
Full profile, `windows.exchange.logs` (GBs):
```
%ProgramFiles%\Microsoft\Exchange Server\V15\Logging\HttpProxy\**\*.log     # ProxyLogon/ProxyShell/ProxyNotShell
%ProgramFiles%\Microsoft\Exchange Server\V15\Logging\ECP\Server\*.log       # Set-OabVirtualDirectory abuse
%ProgramFiles%\Microsoft\Exchange Server\V15\Logging\OABGeneratorLog\*.log
%ProgramFiles%\Microsoft\Exchange Server\V15\TransportRoles\Logs\**\*.log   # message tracking: mail metadata PII
%SystemRoot%\Microsoft.NET\Framework64\v4.0.30319\Temporary ASP.NET Files\**\*.compiled
```
- Exchange is often installed on D:. Exchange sets the system variable `%ExchangeInstallPath%`
  (`...\V15\`, with a trailing backslash), so patterns can use `%ExchangeInstallPath%Logging\HttpProxy\**`. Check
  how Relic joins a variable that ends in `\` and handles a missing variable.
- Conf: high (Microsoft HAFNIUM blog 2021-03-02: HttpProxy, ECP\Server, OABGeneratorLog, owa\auth and aspnet_client
  paths; KAPE `ExchangeClientAccess`, `ExchangeTransport`, `ExchangeCve-2021-26855`, `ExchangeSetupLog`).

### 11. `windows.iis.config` and `windows.logs.httperr`: category `server`, profile standard
```
%SystemRoot%\System32\inetsrv\config\{applicationHost.config,administration.config,redirection.config}
%SystemDrive%\inetpub\history\CFGHISTORY_*\*.config         # automatic snapshots (every 2 min, last 10)
%SystemDrive%\inetpub\wwwroot\**\web.config
%SystemRoot%\System32\LogFiles\HTTPERR\*.log                # http.sys errors; WinRM/WSUS/Exchange even without IIS
```
- Value: malicious IIS native/managed modules and handlers (IIS backdoors such as Owowa and RGDoor), the time they
  were added (from the config history diff), webshell handler mappings, and exploit noise in HTTPERR.
- Size: KB–tens of MB.
- Sensitive: applicationHost.config holds encrypted app-pool/virtual-directory passwords.
- Conf: high. Sources: KAPE `Windows/IISConfiguration.tkape`, `Logs/IISLogFiles.tkape`; Microsoft IIS `configHistory`
  schema docs (default `%SystemDrive%\inetpub\history`, `CFGHISTORY_nnnnnnnnnn`).

### 12. `windows.powershell.transcripts` and `windows.powershell.profiles`: category `execution`/`persistence`, profile standard
Transcripts:
```
%UserHome%\Documents\PowerShell_transcript.*.txt                                       # Start-Transcript default
%UserHome%\Documents\20[0-9][0-9][01][0-9][0-3][0-9]\PowerShell_transcript.*.txt         # GPO transcription, no OutputDirectory
%UserHome%\AppData\Local\Microsoft_Corporation\powershell_ise.exe_StrongName*\*\AutoSaveFiles\*.ps1   # unsaved ISE scripts
```
Profiles (T1546.013):
```
%SystemRoot%\System32\WindowsPowerShell\v1.0\*profile.ps1
%SystemRoot%\SysWOW64\WindowsPowerShell\v1.0\*profile.ps1
%UserHome%\Documents\WindowsPowerShell\*profile.ps1
%UserHome%\Documents\PowerShell\*profile.ps1
%ProgramFiles%\PowerShell\7\*profile.ps1
%ProgramFiles%\PowerShell\7\powershell.config.json
```
- Value: full command input and output wherever transcription is enabled; profiles are a common persistence point.
- A GPO `OutputDirectory` (`HKLM\SOFTWARE\Policies\Microsoft\Windows\PowerShell\Transcription`) puts transcripts
  anywhere. That needs registry-driven paths (roadmap). KAPE also guesses `C:\PSTranscript\20*\`.
- Size: KB to hundreds of MB if transcription is enabled fleet-wide.
- Sensitive: transcripts can contain secrets.
- Conf: high (KAPE `Windows/PowerShellTranscripts.tkape`, `Logs/PowerShellConsole.tkape`,
  `Windows/PowerShell7Config.tkape`; ForensicArtifacts `WindowsPowerShellDefaultProfiles`; Microsoft Start-Transcript
  docs). Documents redirected to OneDrive (`%UserHome%\OneDrive*\Documents\...`) are not covered.

### 13. `windows.cryptnet_url_cache` and `windows.zone_identifier`: download provenance, category `network`, profile standard
```
%UserHome%\AppData\LocalLow\Microsoft\CryptnetUrlCache\MetaData\*
%SystemRoot%\SysWOW64\config\systemprofile\AppData\LocalLow\Microsoft\CryptnetUrlCache\MetaData\*    # unless X1
%UserHome%\Downloads\**\*:Zone.Identifier
%UserHome%\Desktop\**\*:Zone.Identifier
```
- Full profile only: `...\CryptnetUrlCache\Content\*` (the downloaded payloads).
- Value: `certutil -urlcache -f` (and other CryptAPI) downloads, with URL, time and size, including those made as
  SYSTEM. The Mark-of-the-Web HostUrl/ReferrerUrl shows where a tool such as AnyDesk.exe or a loader came from.
- Size: KB.
- Conf: high for the cache (KAPE `Windows/CertUtil.tkape`; ForensicArtifacts `WindowsCryptnetUrlCacheMetadata`).
  Medium for the Zone.Identifier lines: they need wildcard file name plus named-stream support in the engine (the
  docs suggest `*:Zone.Identifier` works). Small streams are also resident in `$MFT`.

### 14. `windows.notepad`: Windows 11 Notepad tab state and Notepad++ backups, category `user_activity`, profile standard
```
%UserHome%\AppData\Local\Packages\Microsoft.WindowsNotepad_8wekyb3d8bbwe\LocalState\TabState\*.bin
%UserHome%\AppData\Local\Packages\Microsoft.WindowsNotepad_8wekyb3d8bbwe\LocalState\WindowState\*.bin
%UserHome%\AppData\Roaming\Notepad++\backup\**
%UserHome%\AppData\Roaming\Notepad++\{session.xml,config.xml}
```
- Value: unsaved buffers. Operators stage commands, credential lists, IP lists and ransom-note drafts in Notepad.
  TabState also gives the saved path and its hash.
- Size: KB–MB. Sensitive: arbitrary user text.
- Conf: high (KAPE `Windows/Notepad.tkape`, part of KapeTriage; ogmini Notepad-State-Library confirms `WindowState`.
  Securelist's "WindowsState" spelling is wrong). KAPE `Apps/Notepad++.tkape`.

### 15. `windows.mssql.errorlog`: SQL Server error logs, category `server`, profile standard
```
%ProgramFiles%\Microsoft SQL Server\*\MSSQL\Log\ERRORLOG*
%ProgramFiles(x86)%\Microsoft SQL Server\*\MSSQL\Log\ERRORLOG*
%ProgramFiles%\Microsoft SQL Server\*\MSSQL\Log\SQLAGENT.*                 # medium conf
```
- Value: brute force against `sa` (error 18456 bursts), `xp_cmdshell`/`Ole Automation` enablement, CLR assembly
  loading. MSSQL is a recurring initial-access path (Mallox/TargetCompany, crypto miners).
- Size: a few MB (6 rollovers by default).
- Conf: high (ForensicArtifacts `MicrosoftSqlServerErrorLogs`; KAPE `Logs/MSSQLErrorLog.tkape`). Non-default data
  roots need registry-driven paths.

---

## 3. Second tier (recommended, lower value or narrower)

| # | Name, category, profile | Paths | Value | Size / sensitivity | Conf / source |
|---|---|---|---|---|---|
| 3.1 | `windows.ntfs.txf` file_system, standard, `method: raw` | `%SystemDrive%\$Extend\$RmMetadata\$TxfLog\$Tops:$T` | TxF log (MFTECmd parses it); covers TxF use such as Process Doppelgänging | MB | high, KAPE `$T.tkape` (in KapeTriage FileSystem) |
| 3.2 | `windows.syscache` execution, standard | `%SystemDrive%\System Volume Information\Syscache.hve` and `Syscache.hve.LOG*` | Execution/file evidence on Win7/2008R2-era and some servers | KB–MB | high, KAPE `Syscache.tkape` |
| 3.3 | `windows.lateral.impacket` execution, standard | `%SystemDrive%\__output*`, `%SystemRoot%\__[0-9]*`, `%SystemRoot%\[a-zA-Z][a-zA-Z][a-zA-Z][a-zA-Z][a-zA-Z][a-zA-Z][a-zA-Z][a-zA-Z].bat` | Left-over smbexec (`C$\__output_XXXXXXXX`, `%SYSTEMROOT%\XXXXXXXX.bat`) and wmiexec/dcomexec (`ADMIN$\__<epoch>`) output when the tool crashed | bytes; output can include command results | high (impacket master `examples/*.py`); KAPE `ImpacketSMBExec.tkape` walks all of C:\. atexec (`%SystemRoot%\Temp\XXXXXXXX.tmp`) is too generic to target |
| 3.4 | `windows.logs.debug` logs, standard | `%SystemRoot%\debug\*.log`, `%SystemRoot%\debug\netlogon.bak` | netlogon.log (Zerologon, failed secure channels, if DBFlag set), mrt.log (MSRT detections), NetSetup.LOG (domain join), PASSWD.LOG | KB–20 MB | high for netlogon (Microsoft docs, CrowdStrike); medium for mrt.log (from memory, not doc-verified) |
| 3.5 | `windows.dhcp_server` network, standard | `%SystemRoot%\System32\dhcp\DhcpSrvLog-*.log`, `%SystemRoot%\System32\dhcp\DhcpV6SrvLog-*.log` | IP-to-MAC/hostname mapping for the attack window (7-day rotation) | ~1 MB/day | high (Microsoft DHCP audit logging docs; KAPE `WindowsServerDNSAndDHCP`) |
| 3.6 | `windows.dns_server` network, full | `%SystemRoot%\System32\dns\dns.log`, `%SystemRoot%\System32\dns\*.dns` | DNS debug log (only if enabled) shows C2/exfil lookups by internal clients; zone files | up to 500 MB | high (Microsoft DNS debug log docs; KAPE). Custom log path needs the registry |
| 3.7 | `windows.backup.veeam` server, standard | `%ProgramData%\Veeam\Backup\Svc*VeeamBackup*.log` | Veeam B&R abuse (CVE-2023-27532 credential theft, backup deletion before encryption) | MB–100s of MB; full: `%ProgramData%\Veeam\Backup\**\*.log` | medium: path confirmed by Horizon3 and the Veeam community, file name is `Svc.VeeamBackup.log` or `Svc_VeeamBackup.log` depending on source, so the glob covers both. API calls are logged only at LoggingLevel 7 |
| 3.8 | `windows.appcompat.custom_shims` persistence, standard | `%SystemRoot%\AppPatch\Custom\**\*.sdb`, `%SystemRoot%\AppPatch\CustomSDB\*`, `%SystemRoot%\AppPatch\AppPatch64\Custom\*` | Shim persistence (FIN7-style sdbinst) | KB | high (KAPE `SDB.tkape`; ForensicArtifacts `WindowsApplicationCompatibilityInstalledShimDatabases`) |
| 3.9 | `windows.office.persistence` persistence, standard | `%UserHome%\AppData\Roaming\Microsoft\Word\STARTUP\*`, `%UserHome%\AppData\Roaming\Microsoft\Excel\XLSTART\*`, `%UserHome%\AppData\Roaming\Microsoft\AddIns\*`, `%UserHome%\AppData\Roaming\Microsoft\Templates\Normal.dotm`, `%UserHome%\AppData\Roaming\Microsoft\Outlook\VbaProject.OTM` | Office template, add-in and Outlook VBA persistence (T1137) | KB–MB | medium-high (MITRE T1137; only partly in KAPE via `OfficeAutosave`) |
| 3.10 | `windows.rdp.client_files` remote_access, standard | `%UserHome%\Documents\*.rdp` (hidden Default.rdp = last target), `%UserHome%\AppData\Local\Microsoft\Remote Desktop Connection Manager\*.settings`, `%UserHome%\AppData\Local\Devolutions\RemoteDesktopManager\Connections.log` | Outgoing RDP targets and admin jump tooling | KB; RDCMan `.rdg` and mRemoteNG `confCons.xml` hold encrypted creds, so full only | high for RDCMan/RDM (KAPE); medium for Default.rdp |
| 3.11 | `windows.vpn_clients` network, standard | FortiClient `%ProgramFiles%\Fortinet\FortiClient\logs\trace\**`, `%UserHome%\AppData\Local\FortiClient\logs\trace\**`; GlobalProtect `%ProgramFiles%\Palo Alto Networks\GlobalProtect\*.log*`, `%UserHome%\AppData\Local\Palo Alto Networks\GlobalProtect\PanGP*.log*`; Ivanti/Pulse `%ProgramData%\Pulse Secure\Logging\**`, `%ProgramFiles(x86)%\Pulse Secure\Logging\**`; OpenVPN `%UserHome%\OpenVPN\log\*.log`, `%UserHome%\OpenVPN\config\**`, `%ProgramFiles%\OpenVPN\config\**` | VPN session times and gateway; ties hosts to stolen VPN accounts | MB; OpenVPN configs can embed keys | high (KAPE `FortiClientVPN`, `PaloAlto`, `PulseSecure`, `OpenVPNClient`). Cisco Secure Client logs to EVTX (covered) |
| 3.12 | `windows.network_scanners` execution, standard | `%UserHome%\AppData\Local\Temp\Advanced IP Scanner 2\advanced_ip_scanner_*.bin`, `%SystemRoot%\Temp\Advanced IP Scanner 2\advanced_ip_scanner_*.bin`, `%UserHome%\advanced_ip_scanner_*.bin`, same three for `advanced_port_scanner` | Discovery-phase scanner remnants (scan results live in NTUSER; SoftPerfect `netscan.xml` sits next to the exe, so use `$MFT`) | KB | high (KAPE `AdvancedIPScanner`, `AdvancedPortScanner`) |
| 3.13 | `windows.capability_access` user_activity, standard | `%ProgramData%\Microsoft\Windows\CapabilityAccessManager\CapabilityAccessManager.db*` | Per-app mic/camera/location use with times (Win11) | MB | high (KAPE `CapabilityAccessManager.tkape`) |
| 3.14 | `windows.startupinfo` execution, standard | `%SystemRoot%\System32\WDI\LogFiles\StartupInfo\*.xml` | Processes started in the first 90 s after logon, with command lines | KB | high path (KAPE, ForensicArtifacts); population on current builds unverified |
| 3.15 | `windows.staging` execution, full (standard once X3 exists) | `%SystemRoot%\Temp\**\*.{exe,dll,ps1,bat,cmd,vbs,js,hta,msi,scr}`, `%UserHome%\AppData\Local\Temp\**\*.{same}`, `%ProgramData%\*.{exe,dll,ps1,bat,cmd,vbs}`, `%SystemDrive%\PerfLogs\**`, `%SystemDrive%\Users\Public\**\*.{exe,dll,ps1,bat,cmd}` | Attacker tooling and RMM-pushed scripts (Atera `.bat`, ScreenConnect, AdFind, SystemBC) | unbounded; may contain malware | high paths (ForensicArtifacts `WindowsTempDirectories`, KAPE `PerfLogs`) |
| 3.16 | `windows.onedrive` / `windows.cloud_sync` user_activity, full | `%UserHome%\AppData\Local\Microsoft\OneDrive\logs\**`, `%UserHome%\AppData\Local\Microsoft\OneDrive\settings\**`; Dropbox/Google Drive/Box per KAPE `CloudStorage_Metadata` | Exfil or insider via sanctioned sync | 10s–100s of MB; file lists | high (KAPE; restricting to logs and settings avoids the per-user OneDrive binaries) |
| 3.17 | `windows.old` registry/logs, full | `%SystemDrive%\Windows.old\Windows\System32\config\{SAM,SECURITY,SOFTWARE,SYSTEM,DEFAULT}*`, `%SystemDrive%\Windows.old\Windows\System32\winevt\Logs\*.evtx`, `%SystemDrive%\Windows.old\Windows\System32\Tasks\**` | Pre-upgrade state | 100s of MB | high (KAPE, throughout) |
| 3.18 | `windows.etl` logs, full | `%SystemRoot%\System32\WDI\LogFiles\*.etl*`, `%SystemRoot%\System32\LogFiles\WMI\**\*.etl*`, `%SystemRoot%\System32\SleepStudy\**`, `%ProgramData%\Microsoft\Windows\wfp\*.etl` | Boot/shutdown process traces (ShutdownCKCL/BootCKCL), WFP diagnostics | 10s–100s of MB | high (KAPE `EventTraceLogs`; ForensicArtifacts `WindowsEventTracingLogFiles`) |
| 3.19 | `windows.update_logs` system, full | `%SystemRoot%\Logs\CBS\CBS*.log`, `%SystemRoot%\Logs\WindowsUpdate\*.etl`, `%ProgramData%\USOShared\Logs\System\*.etl`, `%SystemRoot%\Panther\{setupact.log,MigLog.xml}` | Patch level, install/upgrade times | 10s–100s of MB | high (KAPE `WindowsUpdate`, `WindowsOSUpgradeArtifacts`) |
| 3.20 | `windows.web.tomcat` server, standard | `%ProgramFiles%\Apache Software Foundation\Tomcat*\logs\*`, `%ProgramFiles(x86)%\Apache Software Foundation\Tomcat*\logs\*`, `%ProgramFiles%\Apache Software Foundation\Tomcat*\conf\tomcat-users.xml` | Java app-server exploitation and manager-app webshell uploads | MB–GB; tomcat-users.xml holds passwords | high (ForensicArtifacts `TomcatLogFiles`, `TomcatPasswordFile`). Apache httpd/XAMPP have no fixed Windows path (KAPE walks C:\ for `access.log`) |
| 3.21 | `windows.adcs.certlog` server, full | `%SystemRoot%\System32\CertLog\*` | Issued certificates (ESC1-8 abuse) | MB–GB | medium (from memory; not in KAPE or ForensicArtifacts) |
| 3.22 | `windows.sticky_notes`, `windows.snipping` user_activity, full | `%UserHome%\AppData\Local\Packages\Microsoft.MicrosoftStickyNotes*\LocalState\plum.sqlite*`; `%UserHome%\AppData\Local\Packages\Microsoft.ScreenSketch_8wekyb3d8bbwe\TempState\**\*.png` | Occasionally useful for user/insider cases | MB, PII | high (KAPE) |

---

## 4. Opt-in only (needs X2; never via `full` or `*`)

| Name | Paths (`method: raw` where locked) | Why opt-in |
|---|---|---|
| `windows.ad.ntds` | `%SystemRoot%\NTDS\{ntds.dit,edb.chk}`, `%SystemRoot%\NTDS\edb*.log`, `%SystemRoot%\NTDS\*.jrs` | Together with the SYSTEM hive (in `minimal`) this is every domain credential. GBs. ForensicArtifacts `WindowsActiveDirectoryDatabaseFile`; KAPE `ActiveDirectoryNTDS`. |
| `windows.memory_files` | `%SystemDrive%\{hiberfil.sys,pagefile.sys,swapfile.sys}`, `%SystemRoot%\Minidump\*.dmp` | GBs; usually larger than the 4 GiB `max_file_size`. KAPE `MemoryFiles`. |
| `windows.outlook.mailstores` | `%UserHome%\AppData\Local\Microsoft\Outlook\*.{ost,pst,nst}`, `%UserHome%\Documents\Outlook Files\*.{pst,ost}`, `%UserHome%\AppData\Local\Microsoft\Windows\INetCache\Content.Outlook\**` | Mailbox content, GBs. The last path (opened attachments, phishing initial access) is small and could become a separate full-profile artifact. KAPE `OutlookPSTOST`. |
| `windows.recall` | `%UserHome%\AppData\Local\CoreAIPlatform.00\UKP\**` | Screenshots plus OCR of everything (Copilot+ PCs); highly sensitive. KAPE `WindowsCopilotRecall`; Securelist. |
| `windows.defender.quarantine_data` | `%ProgramData%\Microsoft\Windows Defender\Quarantine\ResourceData\**` | Encrypted live malware (decryptable with the public RC4 key). |
| `windows.recycle_bin.content` | `%SystemDrive%\$Recycle.Bin\*\$R*`, `%SystemDrive%\$Recycle.Bin\*\$R*\**` | Deleted user data, unbounded. KAPE `RecycleBin_DataFiles`. |
| `windows.sysmon_archive` | `%SystemDrive%\Sysmon\**` (default `ArchiveDirectory`, configurable) | Copies of every deleted file matching Sysmon event 23: attacker tools, but unbounded and may contain malware. Velociraptor `SysmonArchive`; Sysmon Community Guide. |
| `windows.search_index` | `%ProgramData%\Microsoft\Search\Data\Applications\Windows\{Windows.edb,Windows.db*,Windows-gather.db*}`, `...\Windows\*.{log,chk,jrs}`, `...\Windows\GatherLogs\**` | Shows files that existed (deleted tools, staged archives), but Windows.edb reaches GBs on file servers. Win11 uses SQLite `Windows.db` and `Windows-gather.db`; `Windows-usn.db` has no forensic value (Securelist, Stroz Friedberg). This could also simply be `full` if X3 caps it. |
| `windows.virtual_disks` | `%UserHome%\AppData\Local\Packages\*CanonicalGroupLimited*\LocalState\ext4.vhdx` and similar | WSL disks hold bash history and tooling, but GBs. |

---

## 5. Checked and not recommended (or already covered)

- **ConsoleHost_history for service accounts**: covered. `%UserHome%` includes systemprofile, LocalService and
  NetworkService. Only the SysWOW64 systemprofile is missing (X1/F6).
- **ShimCache, BAM, Sysmon configuration, Defender exclusions, PuTTY/WinSCP sessions, RDP MRU**: these live in the
  SYSTEM/SOFTWARE/NTUSER hives, which are already collected. The Sysmon config is the
  `SysmonDrv\Parameters\Rules` value.
- **PowerShell module and script-block logging**: EVTX (4103/4104), already covered.
- **Clipboard history**: not persisted to disk (memory only). Cloud-clipboard items appear in ActivitiesCache.db,
  which is already collected.
- **Windows Hello / NGC keys, DPAPI master keys, Credential Manager vaults, Dropbox `Protect\*` keys**: credential
  material (KAPE `WindowsHello` collects it). Omit for the same reason Relic omits browser `Login Data`; at most opt-in.
- **Microsoft Teams, Slack and other chat apps, Chocolatey/winget logs, SCCM client logs, .NET CLR UsageLogs, Office
  document cache, EventTranscript.db**: low triage value. Map them later from KAPE if requested. CLR UsageLogs are
  occasionally useful for execute-assembly; a small full-profile artifact from KAPE `NETCLRUsageLogs` would suffice.
- **KAPE targets that walk all of C:\** (`rclone.conf`, `netscan.xml`, `WinSCP.ini`, `*.rdg`, `*.MOF`, `*.VHD`,
  `__output`, Apache `access.log`): these conflict with Relic's "volumes are never walked" rule. Use `$MFT` to find
  them, then a follow-up targeted collection.
- **Drivers (`%SystemRoot%\System32\drivers\*.sys`, BYOVD)**: 100–300 MB of mostly benign files. The `$MFT` plus
  Amcache driver inventory or THOR is the better route.
- **Unverified, so not proposed with paths**: Syncro agent, GoTo Resolve, BeyondTrust/Bomgar Jump Client, the PDQ
  Deploy runner working folder, Datto `CentraStage\Packages`, and NinjaOne script folders.

---

## 6. Valuable non-file evidence (roadmap: needs new source types)

**Volatile/live state (`type: command` or native API):**
- Process list with PID/PPID, full command line, user, start time, image path, hash and signature.
- Network connections with owning PID (TCP/UDP, listening ports), DNS client cache, ARP cache, routing table and
  proxy settings.
- Logged-on and RDP sessions (`quser`/WTS API), SMB server sessions and open files (`Get-SmbSession`/`net session`),
  mapped drives, shares.
- Services (state, ImagePath, start type), loaded drivers, scheduled tasks as the Task Scheduler sees them (catches
  registry-only tasks), WMI event subscriptions (`__EventFilter`/consumers/bindings).
- Named pipes (Cobalt Strike default pipe names), mutants.
- Kerberos tickets (`klist sessions`) and local group membership (`net localgroup administrators`).

**Ransomware-specific state checks:**
- `vssadmin list shadows` (shadow copies present or deleted).
- `bcdedit /enum` (recovery disabled).
- BitLocker status (`manage-bde -status`; ShrinkLocker-style abuse).
- Defender state (`Get-MpComputerStatus`, `Get-MpPreference`: tamper protection, real-time off, exclusions).
- Audit policy (`auditpol /get /category:*`) and event-log sizes/retention, to judge what is missing.

**Memory:**
- Full RAM acquisition (WinPMem-style driver; GBs, opt-in).
- Selected process dumps (LSASS is credential-sensitive, so opt-in only).

**Registry-driven path resolution (still file collection, but paths come from the registry):**
- EVTX channels with a relocated `File` value.
- PowerShell transcription `OutputDirectory`.
- IIS site log directories (from applicationHost.config).
- `ExchangeInstallPath`, SQL Server instance roots, DNS debug and DHCP audit log paths, firewall log path.
- Sysmon `ArchiveDirectory`, SYSVOL/NTDS locations (Netlogon/NTDS parameters).
- Folder redirection (Documents in OneDrive or a network share).

**Volume features:**
- Volume Shadow Copy access: collect hives and EVTX from existing snapshots, which are often older than the
  attacker's log clearing.
- All fixed NTFS volumes (X4).
- An ADS enumeration mode (all `Zone.Identifier` streams).

**Registry key export** (selective keys without full hives): low priority, since Relic already collects all hives.

---

## 7. Sources
- KapeFiles (EricZimmerman/KapeFiles @ 576dc9a): https://github.com/EricZimmerman/KapeFiles. Targets cited by file
  name (`Targets/Compound/KapeTriage.tkape`, `!SANS_Triage.tkape`, `Windows/*.tkape`, `Apps/*.tkape`,
  `Antivirus/*.tkape`, `Logs/*.tkape`).
- ForensicArtifacts (ForensicArtifacts/artifacts @ 069e812): https://github.com/ForensicArtifacts/artifacts
  (`data/windows.yaml`, `applications.yaml`, `antivirus.yaml`, `webservers.yaml`, `tomcat.yaml`).
- Synacktiv, "Legitimate RATs: a comprehensive forensic analysis of the usual suspects" (TeamViewer, AnyDesk, Atera,
  Splashtop): https://synacktiv.com/publications/legitimate-rats-a-comprehensive-forensic-analysis-of-the-usual-suspects
- Velociraptor exchange: ScreenConnect https://docs.velociraptor.app/exchange/artifacts/pages/screenconnect, SysmonArchive https://docs.velociraptor.app/exchange/artifacts/pages/sysmonarchive, TabState https://docs.velociraptor.app/exchange/artifacts/pages/tabstate
- Huntress, ScreenConnect CVE-2024-1709/1708: https://huntress.com/blog/a-catastrophe-for-control-understanding-the-screenconnect-authentication-bypass
- Sigma, ScreenConnect temporary file: https://detection.fyi/sigmahq/sigma/windows/file/file_event/file_event_win_remote_access_tools_screenconnect_remote_file
- Microsoft, HAFNIUM targeting Exchange Servers: https://microsoft.com/security/blog/2021/03/02/hafnium-targeting-exchange-servers/
- Microsoft, IIS configHistory: https://learn.microsoft.com/en-us/previous-versions/iis/settings-schema/bb422402(v=vs.90)
- Microsoft, Start-Transcript: https://learn.microsoft.com/powershell/module/microsoft.powershell.host/start-transcript
- Microsoft, DNS debug log / DHCP audit logging: https://technet.microsoft.com/en-us/library/cc776445.aspx, https://technet.microsoft.com/en-us/library/cc758251.aspx
- Fox-IT/NCC, Windows Defender quarantine forensics: https://blog.fox-it.com/2023/12/14/reverse-reveal-recover-windows-defender-quarantine-forensics/
- Securelist, Forensic artifacts in Windows 11: https://securelist.com/forensic-artifacts-in-windows-11/117680/
- ogmini Notepad-State-Library: https://github.com/ogmini/Notepad-State-Library
- Stroz Friedberg/LevelBlue, Windows Search index (Win11 SQLite): https://levelblue.com/blogs/strozfriedberg/windows-search-index-the-forensic-artifact-youve-been-searching-for
- Quick Assist forensics: https://hackuponthegale.github.io/blog/dfir/QuickAssist1
- Datto RMM agent log files: https://rmm.datto.com/help/en/Content/5AGENT/LogFile.htm
- N-able Windows Agent log collection: https://documentation.n-able.com/N-central/troubleshooting/Content/kb/Windows-Agent-Log-Collection-and-Windows-Software-Probe-Log-Collection.htm
- NinjaOne log files: https://www.ninjaone.com/docs/endpoint-management/device-management-troubleshooting/finding-and-collecting-ninjaone-log-files/
- Tactical RMM agent source (`main.go`, `agent.log`): https://github.com/amidaware/rmmagent
- Impacket examples (wmiexec/smbexec/dcomexec/atexec output names): https://github.com/fortra/impacket/tree/master/examples
- Kaspersky KES trace files: https://support.kaspersky.com/KESWin/11.3.0/en-US/124710.htm
- Veeam CVE-2023-27532 log location: https://horizon3.ai/veeam-backup-and-replication-cve-2023-27532-deep-dive/, https://community.veeam.com/cyber-security-space-95/indicators-of-compromise-about-cve-2023-27532-4492
- ngrok agent config location: https://ngrok.com/docs/agent/config.md
- RustDesk paths: https://blog.gridinsoft.com/rustdesk-appeared-on-computer/
- Sophos, Identifying Group Policy attacks: https://www.sophos.com/de-de/blog/identifying-group-policy-attacks
- WER artifacts: https://psmths.gitbook.io/windows-forensics/artifacts-by-type/filesystem-artifacts/wer-files
- Netlogon debug log (Zerologon): https://www.crowdstrike.com/blog/how-to-detect-zerologon-attacks-in-your-windows-infrastructure/
