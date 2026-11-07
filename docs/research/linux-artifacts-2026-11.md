# Relic: Linux artifact gap analysis (research, 2026-11-07)

> Research notes from 2026-11-07 that informed the catalog extension in this
> release. The catalog does not adopt every recommendation: credential
> stores (rclone/ngrok configs, keychains, cloud CLI credentials, RDP
> connection managers, crash dumps, NTDS.dit) stay out of the built-in
> catalog, and items that need engine features (per-source size caps,
> opt-in artifacts, a process source) are listed in `docs/design.md`.
> The authoritative paths are in `internal/catalog/data/*.yaml`.

Scope: Linux, plus `unix.*` items that are also valid on macOS. Baseline: `internal/catalog/data/linux.yaml`
before this release (13 Linux + 3 `unix.*` artifacts), after `sshrc`, `~/.ssh/rc` and
`~/.ssh/environment` were added to `unix.ssh`; 4.7 lists only the remaining delta.

References compared: UAC `artifacts/` (github.com/tclahr/uac `main`), ForensicArtifacts
`artifacts/data/*.yaml`, Velociraptor `artifacts/definitions/Linux`, CyLR, PANIX module/revert scripts
(github.com/Aegrah/PANIX; same author as Elastic's Linux persistence series), Sandfly, Nextron (Plague),
Mandiant/Broadcom (ESXi, vCenter), SANS/AWS (SSM).

Paths are in Relic syntax. "UAC x" = `artifacts/x.yaml`; "FA X" = ForensicArtifacts artifact name.

## 1. Key findings

1. **Debian 12+ fresh installs have no `auth.log` or `syslog`.** rsyslog was demoted to optional, so the
   journal is the only log. Because `linux.logs.journal` is `full`-only, `minimal`/`standard` collect no
   sshd/sudo evidence on those hosts besides wtmp/btmp. Debian 13 also moves to **wtmpdb/lastlog2** (SQLite).
2. **`/proc` works with today's file source** (section 2). A cheap process, socket and kernel snapshot is
   possible now, without a new source type.
3. **Persistence gaps** compared with PANIX, Elastic and UAC: systemd generators, transient units,
   `/run/user`, `~/.local/share/systemd/user`, linger and vendor drop-ins; PAM module binaries;
   package-manager hooks and dpkg maintainer scripts; GRUB/BLS and initramfs hooks; network hooks
   (NetworkManager, ifupdown, dhclient); polkit, D-Bus, binfmt.d, Python `.pth`; BusyBox/Alpine cron,
   OpenRC `local.d`, (x)inetd; and RHEL **`/etc/bashrc`** (only Debian's `bash.bashrc` is collected today).
4. **No server-workload coverage:** containers (Docker, containerd, Podman/Quadlet, Kubernetes, k3s),
   PHP/Tomcat configuration, `.htaccess`/`.user.ini` and compiled JSP, cloud agents (cloud-init, SSM Run
   Command output, Azure run-command), exfil and tunnel tools (rclone, ngrok, cloudflared), and the staging
   dirs `/tmp`, `/var/tmp`, `/dev/shm`.
5. **`linux.logs.var_log` (`/var/log/**`) skips symlinked directories.** On vCenter (VCSA),
   `/var/log/vmware` → `/storage/log/vmware` (Broadcom KB 381722), so all vCenter logs are lost; on ESXi,
   `/var/log/*.log` are symlinks too. Both are top ransomware and espionage targets (UNC3886, BRICKSTORM).
6. **Engine issues that affect Linux artifacts** (no code changed):
   - **Dedup is by path, not (dev, inode).** Symlinked aliases cause duplicate copies when two literal
     patterns reach one file. Examples: merged-/usr (`/lib`→`/usr/lib`), `/var/run`→`/run`,
     `/etc/init.d`→`rc.d/init.d`, `/usr/share/tomcat/conf`→`/etc/tomcat`.
   - **`/var/log/lastlog` is sparse.** Its apparent size is (max UID + 1) × 292 bytes, which is hundreds
     of GB on SSSD/AD hosts (UIDs around 1.5e9). The file is skipped as oversize, or up to 4 GiB of zeros
     are read. Suggest SEEK_DATA/SEEK_HOLE.
   - **No per-source `max_file_size`.** UAC caps `/tmp` at 10 MiB per file. Without a per-source cap,
     staging dirs and webroots cannot go into `standard`.
   - **Inode flags and capabilities are not recorded.** Proposed: record ext4/xfs inode flags
     (immutable/append-only; `chattr +i` on crontabs/`authorized_keys` is common in miner campaigns) and
     the `security.capability` xattr.
   - Positive, worth documenting: the binary is static (`CGO_ENABLED=0`), so `ld.so.preload` userland
     rootkits cannot hide files from Relic. LKM rootkits still can.

## 2. Reading /proc and /sys pseudo-files today

Based on reading `internal/source/os_unix.go`, `internal/collector/files.go` and `internal/pathspec/resolve.go`.

**What works:** `openOS` accepts these files (`S_IFREG`), and `run.copy` reads to EOF without trusting
`st_size`, so size-0 procfs and 4096-byte sysfs files are stored completely. The `O_NOATIME` fallback and
`O_NONBLOCK` are harmless here.

**Side effects:** the record `size` is 0 while `bytes_stored` is the real count; dry-run estimates 0 bytes;
the `max_total_size` pre-check uses 0, so the limit can be overshot slightly. `Changed()` is stable.

**Symlinks:**
- `/proc/self` and `/proc/thread-self` are skipped when matched by a wildcard (good).
- **`/proc/[0-9]*/exe` has a literal last component, so it is followed and copies the binary once per
  process.** That is hundreds of MB; do not ship it before inode dedup or hash-only exists.
- `cwd` resolves to a directory: it is skipped, but `symlink_target` is kept.
- `fd/*` are wildcard symlinks: skipped, but recorded with their target (lsof-lite in the manifest).

**Never allow `/proc/**` or `/sys/**`** (add a catalog lint): reading `/proc/kmsg` is destructive,
`/proc/kcore` reports about 128 TiB, `/proc/*/mem` returns EIO, `pagemap` is huge, `task/` duplicates every
process, and debugfs is unsafe.

**Process churn:** processes that exit mid-run give ENOENT/ESRCH and are recorded as `failed`. Map that to
"skipped: process exited".

**Volatility:** procfs reads are not atomic, so collect procfs first. `environ` and other users' `maps`
need root; `hidepid=` hides other users' processes from non-root.

## 3. Top 15 for server intrusion / ransomware / webshell IR

| # | Artifact | New/ext | Profile | Why |
|---|---|---|---|---|
| 1 | `linux.logs.journal` → standard; extend `linux.logs.auth` | ext | standard / minimal | Only log on Debian 12+; wtmpdb, lastlog2, utmp |
| 2 | `linux.live.procfs` (+ `_detail`) | new | minimal / standard | Processes, sockets, LKM taint, `core_pattern`, binfmt |
| 3 | `linux.persistence.systemd` + `systemd_vendor` | ext | minimal / standard | Generators, transient, user units, linger |
| 4 | `linux.staging_dirs` | new | full (standard once capped) | Droppers, miners, tools in `/tmp`, `/dev/shm` |
| 5 | `linux.web.app_config` + `linux.logs.apps` | new | standard | `.htaccess`/`.user.ini`/php.ini, Tomcat, logs outside `/var/log` |
| 6 | `linux.accounts` / `.shadow` | ext | minimal / standard | `passwd-`/`shadow-` diff shows the last account change |
| 7 | `unix.ssh` | ext | minimal | `authorized_keys*` variants, `*.pub`, rhosts/equiv |
| 8 | `linux.persistence.pam_modules` | new | standard | PAM backdoors (Plague, PANIX) |
| 9 | `linux.containers.config` | new | standard | Privileged/host-ns containers, static pods, Quadlet |
| 10 | `linux.persistence.package_hooks` + `linux.packages.dpkg_info` | new | minimal / standard | apt/dnf hooks, maintainer scripts, md5sums |
| 11 | `linux.persistence.init` + `.cron` | ext | minimal | RHEL `/etc/bashrc`, inittab, rc?.d, xinetd, OpenRC, BusyBox |
| 12 | `linux.appliance.vcenter` | new | standard | vCenter logs are skipped today (symlink) |
| 13 | `linux.cloud_agents` | new | standard | cloud-init per-boot scripts, SSM / Azure command output |
| 14 | `linux.persistence.boot` | new | standard | GRUB/BLS `init=` hijack, initramfs/dracut hooks |
| 15 | `linux.security_config` | new | standard | auditd rules, logrotate, firewall, SELinux (evasion) |

**Strategic alternative:** `linux.etc` collects all of `/etc` minus secrets, as UAC `files/system/etc` does
in `ir_triage` (5–40 MB). In `standard` it would cover most of the `/etc` items below. The targeted
artifacts stay useful for `minimal` and for documentation.

## 4. Detailed recommendations (top 15)

### 4.1 `linux.logs.journal` (profile change) + `linux.logs.auth` (extend) — logs — high

Move `linux.logs.journal` to `standard`, late in the profile order. Add to `linux.logs.auth` (minimal):

```yaml
- '/run/utmp'                        # current sessions (UAC files/system/utmp: /var/run/utmp)
- '/var/lib/wtmpdb/wtmp.db'          # wtmpdb (Debian 13+, openSUSE); /var/log/wtmp.db already matches wtmp*
- '/var/lib/lastlog/lastlog2.db'     # lastlog2 - path medium confidence
- '/var/log/cron*'                   # RHEL cron execution log (FA LinuxCronLogs)
- '/run/systemd/sessions/*'          # logind sessions incl. remote host (UAC files/system/systemd)
- '/run/faillock/*'                  # pam_faillock tallies (RHEL 8+, default /var/run/faillock)
- '/var/{lib,db}/sudo/lectured/*'    # mtime = first sudo per user (UAC live_response/system/sudo_lectured)
```

- **Value:** sshd, sudo and su evidence on journal-only distributions, and Y2038-safe login records.
- **Size:** journal 100 MB–4 GiB (journald caps at 10% of the file system, max 4 GiB); everything else is KB.
- **Sensitive:** PII (usernames, IPs).
- **Sources:** Thomas-Krenn and debian-user (Debian 12); Debian 13 release notes §5.1.9; wtmpdb README.

### 4.2 `linux.live.procfs` (new) — new category `volatile` (or `system`) — minimal, collect first — high

```yaml
- '/proc/[0-9]*/{cmdline,comm,status,stat,cgroup,loginuid,sessionid}'
- '/proc/{cmdline,version,uptime,stat,modules,mounts,swaps,filesystems}'
- '/proc/1/mountinfo'
- '/proc/net/{tcp,tcp6,udp,udp6,raw,raw6,packet,unix,arp,route,ipv6_route,dev}'
- '/proc/sys/kernel/{tainted,modules_disabled,core_pattern,modprobe,randomize_va_space,kptr_restrict,yama/ptrace_scope}'
- '/proc/sys/net/ipv4/ip_forward'
- '/proc/sys/fs/binfmt_misc/*'                        # exclude: /proc/sys/fs/binfmt_misc/register
```

Companion `linux.live.procfs_detail` (standard, sensitive): `'/proc/[0-9]*/{environ,maps}'`.

- **Value:** `/proc/net/packet` and `raw` show sniffer backdoors (BPFDoor); `maps` shows `(deleted)` and
  `memfd:` payloads; `environ` of attacker shells holds `SSH_CONNECTION` (source IP) and `LD_PRELOAD`;
  `core_pattern`, `modprobe` and binfmt_misc are persistence hooks; `tainted` together with `modules`
  shows out-of-tree LKMs.
- **Size:** 1–5 MB. The detail artifact adds 5–30 MB; `environ` holds secrets (container env vars).
- **Sources:** UAC `live_response/process/procfs_information`, `network/procfs_information`,
  `system/binfmt_misc`, `system/coredump`; FA `linux_proc.yaml` (LinuxProcArp, LinuxProcMounts,
  LinuxKernelModuleTaintStatus); Sandfly (deleted binaries, `/dev/shm`, `memfd`).

### 4.3 `linux.persistence.systemd` / `systemd_vendor` (extend) — persistence — minimal / standard — high

Add to `linux.persistence.systemd`:

```yaml
- '/run/systemd/{transient,generator,generator.early,generator.late,system.control}/**'
- '/run/user/*/systemd/{transient,generator,generator.early,generator.late,user,user.control}/**'
- '%UserHome%/.local/share/systemd/user/**'
- '/etc/xdg/systemd/user/**'
- '/usr/local/lib/systemd/{system,user,system-generators,user-generators}/**'
- '/run/systemd/{system,user}-generators/*'
- '/var/lib/systemd/linger/*'          # loginctl enable-linger: user units run without a login
- '/var/lib/systemd/timers/*'          # stamp-*.timer mtime = last run of Persistent= timers
```

Change `systemd_vendor` to the patterns below and move it to `standard`:

```yaml
- '/usr/lib/systemd/{system,user}/**'                  # adds *.d drop-ins and *.wants link records
- '/usr/lib/systemd/{system,user}-generators/*'        # PANIX drops generators here
```

Non-merged-/usr hosts (Debian ≤ 10, upgraded Ubuntu ≤ 18.04) also need `'/lib/systemd/system/**'`. Add it
only with inode dedup, otherwise merged hosts collect everything twice.

- **Value:** generators run as root before units load; `systemd-run` transient units and timers leave
  nothing in `/etc`; vendor-dir plants are exposed by hashing against dpkg md5sums (4.10); symlink records
  in `*.wants` show which units are enabled.
- **Size:** about 1–5 MB.
- **Sensitive:** no.
- **Sources:** UAC `files/system/systemd` (transient, `.local/share/systemd`, `/usr/local/lib/systemd`); FA
  LinuxSystemdServices and LinuxSystemdTimers (all generator, transient and `/run/user` dirs); PANIX
  `revert_generator_persistence.sh`; Pepe Berba's persistence series, part 5 (generators).

### 4.4 `linux.staging_dirs` (new) — file_system — full now; standard once per-source size caps exist — high

```yaml
- '/tmp/**'           # incl. /tmp/systemd-private-*/tmp (PrivateTmp of apache, php-fpm, nginx)
- '/var/tmp/**'       # incl. /var/tmp/systemd-private-*
- '/dev/shm/**'
```

- **Value:** the main drop location for webshell second stages, miners, tunnels and exfil archives.
  Webshells running under services with PrivateTmp write into the `systemd-private-*` directories.
- **Size:** unbounded. With a 10 MiB per-file cap it is usually under 200 MB; oversize files still get a
  skip record with size and mtime.
- **Sensitive:** can contain user data.
- **Sources:** UAC `files/system/{tmp,var_tmp,dev_shm,run_shm}` (all `max_file_size: 10485760`, in
  `ir_triage`); Sandfly on `/dev/shm`.

### 4.5 `linux.web.app_config` (new) — persistence — standard — high (Tomcat variants medium)

```yaml
- '/etc/php/**'                                        # Debian/Ubuntu php.ini, fpm pool.d, mods-available
- '/etc/{php.ini,php-fpm.conf}'                        # RHEL
- '/etc/{php.d,php-fpm.d}/*'
- '/var/www/**/{.htaccess,.user.ini}'                  # auto_prepend_file, AddHandler/AddType remaps
- '/srv/**/{.htaccess,.user.ini}'
- '%UserHome%/public_html/**/{.htaccess,.user.ini}'    # UserDir / cPanel
- '/etc/tomcat*/{server.xml,web.xml,context.xml,tomcat-users.xml,catalina.properties}'
- '/{opt,usr/share,usr/local,var/lib}/*tomcat*/conf/{server.xml,web.xml,context.xml,tomcat-users.xml}'
- '/var/cache/tomcat*/Catalina/**/*_jsp.{java,class}'  # compiled JSP survives deletion of the .jsp
- '/{opt,usr/share,usr/local}/*tomcat*/work/Catalina/**/*_jsp.{java,class}'
```

- **Value:** webshell persistence via `auto_prepend_file` and `.htaccess` handler remaps, Tomcat manager
  credentials, and JSP webshells that remain after cleanup.
- **Size:** 1–10 MB. The `**` walk reads every webroot directory, which is slow on very large webroots.
- **Sensitive:** `tomcat-users.xml` holds plaintext passwords; FPM pools may hold secrets.
- **Sources:** FA TomcatPasswordFile and WordpressConfigFile (`/var/www/**/wp-config.php`); PANIX
  `setup_web_shell` (`/var/www/html`).

**4.5b `linux.logs.apps` (new) — logs — standard — medium.** Covers exploit and webshell traces outside
`/var/log`.

```yaml
- '/{opt,usr/share,usr/local}/*tomcat*/logs/*'        # FA TomcatLogFiles (catalina.out, access_log*)
- '/usr/local/{nginx,apache2}/logs/*'                 # source builds, official httpd image
- '/opt/lampp/logs/*'                                 # XAMPP
- '/var/lib/pgsql/{data,*/data}/{log,pg_log}/*'       # RHEL/PGDG PostgreSQL (FA PostgreSQLLogFiles)
- '/var/lib/pgsql/{data,*/data}/{postgresql.conf,pg_hba.conf}'
```

`catalina.out` can reach GBs, so this artifact needs a size cap.

### 4.6 `linux.accounts` / `linux.accounts.shadow` (extend) — accounts — minimal / standard — high

```yaml
# linux.accounts
- '/etc/{passwd-,group-,subuid,subgid,shells,doas.conf}'
- '/var/backups/{passwd,group}.bak'                   # Debian cron.daily/passwd
- '/etc/security/*.conf'                              # access, limits, pam_env, faillock, namespace ...
- '/etc/security/{limits.d,namespace.d}/*'
# linux.accounts.shadow
- '/etc/{shadow-,gshadow-}'
- '/var/backups/{shadow,gshadow}.bak'
- '/etc/security/opasswd'                             # previous hashes (pam_pwhistory)
```

- **Value:** shadow-utils writes the `-` copies on every change. Diffing them shows the most recent UID-0
  or system-account backdoor; the `.bak` files are a daily baseline.
- **Size:** KB.
- **Sensitive:** the shadow artifact holds password hashes.
- **Sources:** FA UnixShadowBackupFile and LoginPolicyConfiguration (`/etc/security/access.conf`); PANIX
  `backdoor_user`, `passwd_user` and `password_change`.

### 4.7 `unix.ssh` (extend; remaining delta) — remote_access — minimal — high

```yaml
- '%UserHome%/.ssh/{authorized_keys*,known_hosts*,*.pub}'     # replaces the explicit names
- '/etc/ssh/{ssh_known_hosts,ssh_host_*_key.pub}'
- '/etc/{hosts.equiv,shosts.equiv}'
- '%UserHome%/.{rhosts,shosts}'
```

- **Value:** backup copies (`authorized_keys.bak`, `authorized_keys~`, `known_hosts.old` written by
  `ssh-keygen -R`) keep keys that were removed. Host public keys allow fingerprint correlation, and the
  rhosts/equiv files enable trust-based logins.
- **Size:** KB.
- **Sensitive:** no private keys are collected.
- **Sources:** UAC `files/ssh/authorized_keys` (`authorized_keys*`), `known_hosts` (`known_hosts*`),
  `public_keys` (`*.pub`), `rc` and `files/system/rhosts`; FA SSHHostPubKeys and SSHKnownHostsFiles.

### 4.8 `linux.persistence.pam_modules` (new) — persistence — standard — high

```yaml
- '/usr/{lib,lib64}/security/*.so'          # RHEL/Fedora/SUSE (lib64), Arch (lib)
- '/usr/lib/*-linux-gnu/security/*.so'      # Debian/Ubuntu multiarch
```

Legacy non-merged hosts also need `'/lib/*-linux-gnu/security/*.so'` and `'/{lib,lib64}/security/*.so'`.
Add these only with inode dedup. `/etc/pam.d` is already collected by `linux.system_info`.

- **Value:** a backdoored `pam_unix.so` or a rogue module gives a universal password and credential
  logging. Compare the hashes with package md5sums (4.10).
- **Size:** 2–6 MB.
- **Sensitive:** no.
- **Sources:** PANIX `revert_pam.sh` (all module dirs above); Nextron Plague (2025: check `/lib/security`
  and `/etc/pam.d`).

### 4.9 `linux.containers.config` (new) — new category `containers` — standard — high (Docker, k8s), medium (Podman)

```yaml
- '/etc/docker/daemon.json'
- '/var/lib/docker/containers/*/{config.v2.json,hostconfig.json,hosts,resolv.conf,hostname}'
- '/var/lib/docker/image/*/repositories.json'
- '%UserHome%/.local/share/docker/containers/*/{config.v2.json,hostconfig.json}'   # rootless
- '/etc/containerd/config.toml'
- '/var/lib/containerd/io.containerd.metadata.v1.bolt/meta.db'
- '/run/containerd/io.containerd.runtime.v2.task/*/*/{config.json,options.json,log.json}'
- '/etc/containers/{containers.conf,registries.conf,storage.conf,policy.json}'
- '/{etc,usr/share}/containers/systemd/**'                                       # Podman Quadlet
- '%UserHome%/.config/containers/systemd/**'
- '/var/lib/containers/storage/{db.sql,libpod/bolt_state.db}'
- '/var/lib/containers/storage/overlay-containers/{containers.json,*/userdata/config.json}'
- '%UserHome%/.local/share/containers/storage/{db.sql,libpod/bolt_state.db}'
- '/etc/kubernetes/manifests/*'                                                  # static pods
- '/var/lib/kubelet/config.yaml'
- '/var/lib/rancher/{k3s,rke2}/server/manifests/**'                              # auto-deployed
- '/etc/rancher/{k3s,rke2}/{config.yaml,registries.yaml}'
```

- **Value:** shows privileged, host-PID or host-network containers, host mounts used for escape, miner
  images, static-pod and k3s manifest persistence, and Quadlet units (which are a generator).
- **Size:** 1–50 MB (`meta.db` and `db.sql` dominate).
- **Sensitive:** env vars in `config.v2.json` often hold secrets.
- **Sources:** FA DockerContainerConfig and ContainerdConfig (`meta.db`, runtime.v2.task files); FA
  KubernetesKubeletConfiguration and KubernetesKubeletPodManifest; Datadog and Fedora (Podman `db.sql` /
  `bolt_state.db`); PANIX `malicious_docker_container`.

**Companions:**

- `linux.containers.logs` (full; unbounded without log-opts):

  ```yaml
  - '/var/lib/docker/containers/*/*-json.log*'
  - '/var/lib/docker/containers/*/local-logs/*'
  - '/var/log/pods/**'                                                    # FA KubernetesKubeletPodLogs
  - '/var/lib/containers/storage/overlay-containers/*/userdata/ctr.log'   # medium confidence
  ```

- `linux.containers.credentials` (full, sensitive). Never collect `/etc/kubernetes/pki`.

  ```yaml
  - '%UserHome%/.docker/config.json'
  - '%UserHome%/.kube/config'
  - '/etc/kubernetes/{admin,super-admin,kubelet,controller-manager,scheduler}.conf'
  - '/etc/rancher/{k3s,rke2}/{k3s,rke2}.yaml'
  ```

### 4.10 `linux.persistence.package_hooks` (new, minimal) + `linux.packages.dpkg_info` (new, standard) — high

```yaml
# linux.persistence.package_hooks - persistence, < 1 MB
- '/etc/apt/apt.conf'
- '/etc/apt/apt.conf.d/*'                         # DPkg::Post-Invoke, APT::Update::Pre-Invoke
- '/etc/yum/pluginconf.d/*'
- '/usr/lib/yum-plugins/*'
- '/etc/dnf/plugins/*'
- '/usr/lib/python3*/site-packages/dnf-plugins/*.py'
- '/etc/dnf/libdnf5-plugins/**'                   # dnf5 "actions" plugin (Fedora 41+), medium
- '/etc/pacman.d/hooks/*'
- '/usr/share/libalpm/hooks/*'
# linux.packages.dpkg_info - system, 5-25 MB
- '/var/lib/dpkg/info/*.{preinst,postinst,prerm,postrm,config,triggers,md5sums,conffiles}'
- '/var/lib/dpkg/{diversions,statoverride,status-old}'
- '/var/backups/dpkg.{status,diversions,statoverride}*'
- '/var/lib/apt/extended_states'
```

- **Value:** Package-manager hooks run as root on every apt or dnf run; maintainer scripts of malicious
  `.deb` packages persist; `statoverride` and `diversions` let backdoors (for example an SUID binary)
  survive upgrades; md5sums allow offline debsums checks; the `dpkg.status.N` backups give a per-day
  package history.
- **Optional:** `'/var/lib/dpkg/info/*.list'` in `full`. RPM scriptlets are in `linux.packages.rpmdb`.
- **Sources:** UAC `files/packages/{apt,dnf,yum}`; PANIX `revert_package_manager_persistence.sh`
  (`/etc/apt/apt.conf.d/01python-upgrades`, `/etc/dnf/plugins/*.conf`,
  `/usr/lib/python3*/site-packages/dnf-plugins/*.py`, `/usr/lib/yum-plugins/*.py`); PANIX
  `revert_malicious_package.sh` (`/var/lib/dpkg/info`).

### 4.11 `linux.persistence.init`, `linux.persistence.cron`, `unix.shell_config` (extend) — persistence — minimal — high

```yaml
# linux.persistence.init
- '/etc/{bashrc,zshrc,zshenv,zprofile,zlogin,zlogout,csh.cshrc,csh.login,kshrc,inittab,inetd.conf,xinetd.conf}'
- '/etc/{zsh,xinetd.d,bash_completion.d,local.d}/*'    # local.d: OpenRC *.start scripts (Alpine, Gentoo)
- '/etc/fish/{config.fish,conf.d/*}'
- '/etc/rc{0,1,2,3,4,5,6,S}.d/*'                        # links recorded with targets = enabled scripts
- '/etc/rc.d/rc{0,1,2,3,4,5,6}.d/*'
- '/etc/runlevels/*/*'                                  # OpenRC
- '/etc/{default,sysconfig}/*'                          # /etc/default/grub, /etc/sysconfig/init (BRICKSTORM)
- '/{usr/lib,run}/udev/rules.d/*'
- '/{usr/lib,run}/{modules-load.d,modprobe.d}/*'
- '/{etc,run,usr/lib}/binfmt.d/*'
- '/{etc,usr/lib}/environment.d/*'
- '%UserHome%/.config/environment.d/*'
- '/etc/X11/{Xsession.d,xinit/xinitrc.d}/*'
- '%UserHome%/.config/{autostart-scripts,plasma-workspace/env}/*'   # KDE login scripts
# linux.persistence.cron
- '/etc/crontabs/*'                                     # BusyBox / Alpine crond
- '/etc/periodic/**'                                    # Alpine run-parts
- '/etc/incron.d/*'
- '/var/spool/incron/*'
# unix.shell_config
- '%UserHome%/.{bash_aliases,xinitrc,xsession,xsessionrc,xprofile}'
- '%UserHome%/.config/git/config'
- '%UserHome%/.config/fish/conf.d/*'
```

- **Value:** every entry runs code at boot or login. `/etc/bashrc` is RHEL's equivalent of
  `bash.bashrc` and is missing today.
- **Size:** 2–8 MB (vendor udev rules dominate).
- **Sensitive:** no.
- **Sources:** FA BashShellConfigurationFile, ZShellConfigurationFile and CShellConfigurationFile;
  LinuxSysVInit and LinuxXinetd; CyLR (`/etc/xinetd.conf`, `/etc/insserv.conf`); UAC `files/system/udev`
  and `xdg_autostart` (`.config/autostart-scripts`); PANIX `revert_lkm.sh` (`/usr/lib/modules-load.d`);
  Broadcom KB 427833 (`/etc/sysconfig/init`).

### 4.12 `linux.appliance.vcenter` (new) — logs — standard — medium

```yaml
- '/storage/log/vmware/{sso,vpxd,vpxd-svcs,rhttpproxy,envoy,vsphere-ui,applmgmt,vmon,sca}/**'
- '/etc/vmware/**'
- '/etc/sysconfig/init'
```

The `full` variant collects `'/storage/log/vmware/**'`.

- **Value:** SSO logins, API and VM operations (snapshot and delete before encryption), and proxy access
  logs. All of these are silently skipped today.
- **Size:** 100 MB to several GB. Verify the subdirectory names on VCSA 7 and 8.
- **Sensitive:** PII (usernames, IPs).
- **Sources:** Broadcom KB 381722 (the symlink); FA vSphereClientLogsDirectory; Broadcom KB 427833 and
  the Mandiant BRICKSTORM defender guide.

### 4.13 `linux.cloud_agents` (new) — execution — standard — high (cloud-init, SSM), medium (Azure)

```yaml
- '/etc/cloud/cloud.cfg'
- '/etc/cloud/cloud.cfg.d/*'
- '/var/lib/cloud/instances/*/{user-data.txt,vendor-data.txt,cloud-config.txt}'
- '/var/lib/cloud/instances/*/scripts/**'
- '/var/lib/cloud/scripts/**'                                   # per-boot / per-instance / per-once
- '/etc/amazon/ssm/*'
- '/var/lib/amazon/ssm/*/document/orchestration/**'             # Run Command scripts + stdout/stderr
- '/var/lib/waagent/{run-command,run-command-handler,custom-script}/download/**'
```

- **Value:** commands run with stolen cloud credentials, including ransomware deployment through SSM.
  `per-boot` scripts persist. The agent logs in `/var/log` are already covered by `var_log`.
- **Size:** KB to tens of MB.
- **Sensitive:** user-data and command output often contain secrets. Never collect
  `/var/lib/amazon/ssm/Vault`.
- **Sources:** UAC `files/system/aws_ssm_agent` and `azure_vm_agent` (`/var/lib/waagent/run-command/download`);
  SANS "Living off the Cloud" and AWS knowledge center (orchestration layout); Cado (orchestration
  output is kept by default).

### 4.14 `linux.persistence.boot` (new) — persistence — standard — high

```yaml
- '/etc/default/grub'
- '/etc/{default/grub.d,grub.d}/*'
- '/boot/{grub,grub2}/{grub.cfg,grubenv,custom.cfg,user.cfg}'
- '/boot/efi/EFI/*/{grub.cfg,grubenv}'
- '/boot/loader/entries/*.conf'                # BLS (RHEL 8+, Fedora): kernel cmdline, init=
- '/etc/kernel/cmdline'
- '/etc/initramfs-tools/**'                    # Debian hooks, scripts, conf.d, modules
- '/usr/share/initramfs-tools/{hooks,scripts}/**'
- '/etc/dracut.conf'
- '/etc/dracut.conf.d/*'
- '/usr/lib/dracut/modules.d/**'
```

Companion `linux.boot.images` (full) collects `'/boot/{initrd.img,vmlinuz}-*'`, `'/boot/initramfs-*.img'`
and `'/boot/efi/EFI/**/*.efi'` (30–150 MB per image; UEFI bootkits such as Bootkitty).

- **Value:** `init=` hijacks and initramfs hooks run before any userland security tool.
- **Size:** 2–8 MB without images.
- **Sensitive:** no.
- **Sources:** UAC `files/system/boot` and `dracut`; FA LinuxGrubConfiguration and LinuxInitrdFiles; CyLR;
  PANIX `revert_grub.sh` (`/etc/default/grub.d/99-panix.cfg`, `/etc/grub.d/99_panix.cfg`) and
  `revert_initramfs.sh` (`/usr/lib/dracut/modules.d/99panix`).

### 4.15 `linux.security_config` (new) — system — standard — high

```yaml
- '/etc/audit/{auditd.conf,audit.rules,audit-stop.rules}'
- '/etc/audit/{rules.d,plugins.d}/*'           # plugins.d entries execute binaries at auditd start
- '/etc/audisp/**'                             # RHEL 7
- '/etc/logrotate.conf'
- '/etc/logrotate.d/*'                         # rotate 0 / postrotate scripts (tampering + persistence)
- '/etc/{syslog.conf,syslog-ng/**}'
- '/etc/selinux/config'
- '/sys/fs/selinux/enforce'
- '/sys/kernel/security/apparmor/profiles'
- '/etc/apparmor.d/**'
- '/etc/{iptables,nftables,firewalld,ufw,fail2ban}/**'
- '/etc/{nftables.conf,default/ufw}'
- '/etc/sysconfig/{iptables,ip6tables,nftables.conf}'
```

- **Value:** shows typical pre-ransomware defense evasion: disabled audit rules, SELinux set to
  permissive, shortened log rotation and firewall openings. Live rulesets need a command source (section 6).
- **Size:** 1–5 MB.
- **Sensitive:** no.
- **Sources:** FA LinuxRsyslogConfigs, LinuxSyslogNgConfigs, UFWConfigFiles and LinuxSystemdJournalConfig;
  CyLR (`/etc/logrotate.conf`).

## 5. Further recommendations (lower rank)

Each entry gives profile, confidence, size and sensitivity, then the patterns.

### Persistence

- **`linux.persistence.network_hooks`** (new) — standard, high; KB.
  - Value: scripts that run on network events.
  - Sources: PANIX `revert_network_manager.sh`; FA LinuxIfUpDownScripts.

  ```yaml
  ['/{etc,usr/lib}/NetworkManager/dispatcher.d/**', '/etc/network/{if-pre-up.d,if-up.d,if-down.d,if-post-down.d,interfaces.d}/*',
   '/{etc,usr/lib}/networkd-dispatcher/**', '/etc/dhcp/{dhclient-enter-hooks.d,dhclient-exit-hooks.d,dhclient.d}/*',
   '/etc/dhcp/{dhclient-enter-hooks,dhclient-exit-hooks,dhclient.conf}', '/etc/ppp/ip-{up,down}.d/*']
  ```

- **`linux.persistence.desktop_bus`** (new) — standard, high; under 2 MB.
  - Value: root D-Bus activation services and permissive polkit rules.
  - Sources: PANIX `revert_polkit.sh` and `revert_dbus.sh`; UAC `files/system/{polkit,dbus}`.

  ```yaml
  ['/etc/polkit-1/**', '/usr/share/polkit-1/rules.d/*', '/var/lib/polkit-1/**', '/etc/dbus-1/**',
   '/usr/share/dbus-1/{system.d,session.d,system-services,services}/*', '%UserHome%/.local/share/dbus-1/services/*']
  ```

- **`linux.persistence.python_hooks`** (new) — standard, medium; KB.
  - Value: Python startup hooks (MITRE T1546.018). Legitimate `.pth` files exist, so review content.

  ```yaml
  ['/usr/{lib,lib64,local/lib}/python3*/{site,dist}-packages/{*.pth,sitecustomize.py,usercustomize.py}',
   '/usr/{lib,lib64,local/lib}/python3*/{sitecustomize,usercustomize}.py', '/etc/python3*/sitecustomize.py',
   '%UserHome%/.local/lib/python3*/site-packages/{*.pth,usercustomize.py}']
  ```

- **`linux.persistence.git`** (new) — full, medium.
  - Value: hooks, plus `core.pager`, `core.fsmonitor` and `core.hooksPath` in git configs.
  - Caution: the walks are costly, and `%UserHome%` includes `/var/www`.
  - Sources: PANIX git module; UAC `files/applications/git` searches `*/.git/hooks/*` from `/`.

  ```yaml
  ['%UserHome%/**/.git/{config,hooks/*}', '/{srv,opt,var/www}/**/.git/{config,hooks/*}', '/etc/gitconfig']
  # exclude: ['**/*.sample']
  ```

### Logs and system

- **`linux.mail.spool`** (new) — root mailbox in standard, all mailboxes in full; medium; PII.
  - Patterns: `'/var/spool/mail/root'` (standard), `'/var/spool/mail/*'` (full).
  - Value: cron output mailed to root often reveals miner or backdoor jobs.
  - Debian links `/var/spool/mail` to `../mail`, so the one path covers both layouts.
- **`linux.locate_db`** (new) — standard, medium; 5–50 MB.
  - Patterns: `'/var/lib/{plocate/plocate.db,mlocate/mlocate.db}'`, `'/var/cache/locate/locatedb'`
    (FA LocateDatabase).
  - Value: file names from the last `updatedb`, including deleted attacker tools.
- **`linux.crash_dumps`** (new) — full, high; large; contains process memory.
  - Patterns: `'/var/crash/*'`, `'/var/lib/systemd/coredump/*'`, `'/var/spool/abrt/**'`
    (UAC `memory_dump/coredump`).
  - Value: exploit attempts crash services.
- **`linux.network_config`** (new) — standard, medium; KB.
  - Value: DHCP lease history (IP over time) for correlation with external logs.
  - Sources: FA LinuxNetworkManager; UAC `files/system/networkmanager`.

  ```yaml
  ['/var/lib/dhcp/*.leases', '/var/lib/dhclient/*', '/var/lib/NetworkManager/{*.lease,timestamps,seen-bssids}',
   '/run/systemd/netif/leases/*', '/etc/NetworkManager/{NetworkManager.conf,conf.d/*}', '/etc/sysconfig/network-scripts/*']
  ```

- **`linux.network_config.secrets`** (new) — full; sensitive (Wi-Fi PSKs, VPN keys).
  - Patterns: `'/etc/NetworkManager/system-connections/*'`, `'/etc/wireguard/*'`, `'/etc/openvpn/**'`.
- **`linux.directory_services`** (new) — standard, medium.
  - Sensitive: `sssd.conf` and `nslcd.conf` may contain bind passwords.
  - Put `'/var/lib/sss/db/*'` (cached credentials) and `'/etc/krb5.keytab'` in `full`.
  - Sources: FA LinuxKerberosConfiguration and SambaConfigFile.

  ```yaml
  ['/etc/{krb5.conf,realmd.conf,nslcd.conf}', '/etc/krb5.conf.d/*', '/etc/sssd/{sssd.conf,conf.d/*}',
   '/etc/samba/smb.conf', '/etc/{ldap,openldap}/ldap.conf']
  ```

- **`linux.binaries.local`** (new) — full, medium; 10–500 MB.
  - Patterns: `'/usr/local/{bin,sbin}/*'` (binary drops, PANIX system-binary wrappers) and
    `'/usr/lib/modules/*/{extra,updates}/**'` (out-of-tree and DKMS LKMs).
  - Never collect `/lib/modules` wholesale (300 MB–1 GB per kernel).

### Users and tools

- **`linux.remote_access_tools`** (new; mirrors `windows.remote_access_tools`) — standard, medium.
  - Value: exfil destinations and remote-access traces.
  - Sensitive: `rclone.conf`, the ngrok authtoken, cloudflared tunnel JSON.
  - Sources: UAC `files/applications/{rclone,anydesk,teamviewer,rustdesk,splashtop}`.
  - TeamViewer `/var/log/teamviewer*` and AnyDesk `/var/log/anydesk.trace` are already in `var_log`.

  ```yaml
  ['%UserHome%/.config/rclone/rclone.conf', '%UserHome%/.anydesk/*', '/etc/anydesk/*',
   '%UserHome%/.local/share/teamviewer*/logfiles/*.log', '%UserHome%/.local/share/logs/RustDesk/**',
   '%UserHome%/.config/rustdesk/*', '/opt/splashtop*/{log,config}/*', '%UserHome%/.config/ngrok/ngrok.yml',
   '%UserHome%/.ngrok2/ngrok.yml', '/etc/cloudflared/*', '%UserHome%/.cloudflared/*']
  ```

- **`unix.cloud_cli`** (new) — standard, high; not sensitive.
  - Patterns: `'%UserHome%/.aws/config'`, `'%UserHome%/.azure/{azureProfile.json,config,commands/*}'`,
    `'%UserHome%/.config/gcloud/{active_config,configurations/*,logs/**}'`.
  - Value: the Azure and gcloud CLIs log every command, which shows how stolen cloud credentials were used.
- **`unix.cloud_cli.credentials`** (new) — full, high; secrets.
  - Patterns: `'%UserHome%/.aws/{credentials,cli/cache/*,sso/cache/*}'`,
    `'%UserHome%/.azure/msal_token_cache.*'`,
    `'%UserHome%/.config/gcloud/{credentials.db,access_tokens.db,legacy_credentials/**}'`.
  - Prefer a metadata-only mode (section 6): Relic preserves atime, so attacker access is visible
    without storing the secrets.
- **`unix.shell_history`** (extend) — minimal, high.
  - Sources: UAC `files/shell/bash` (`.bash_history~`, `.bash_history.*`) and `files/applications/php`;
    FA RootUserShellHistory (`.zhistory`).

  ```yaml
  ['%UserHome%/.{zhistory,php_history,irb_history,dbshell,bash_history-*,bash_history.*}',
   '%UserHome%/.mongodb/mongosh/mongosh_repl_history', '%UserHome%/.local/share/{nano/search_history,mc/history}']
  ```

- **`linux.desktop.user_activity`** (new) — standard, high; PII. Relevant on admin and developer workstations.
  - Sources: UAC `files/system/{linux_mru,trash_info,gvfs_metadata,xsession_errors}`.

  ```yaml
  ['%UserHome%/.local/share/{recently-used.xbel,gvfs-metadata/*}', '%UserHome%/.local/share/Trash/info/*.trashinfo',
   '%UserHome%/.xsession-errors*', '/var/lib/AccountsService/users/*']
  ```

- **`unix.ai_agent_history`** (new) — full, low priority; contains secrets.
  - Paths: `%UserHome%/.claude/**`, `%UserHome%/.codex/**` and similar.
  - Value: UAC added about 20 AI-tool artifacts in 2025–26 after supply-chain malware abused local AI CLIs.

### Strategic and niche

- **`linux.etc`** (new) — standard; medium confidence on the profile, high on the value; 5–40 MB.
  - Sensitive: contains application secrets, for example `/etc/mysql/debian.cnf`.
  - Source: UAC `files/system/etc` (in `ir_triage`, excludes only shadow files).

  ```yaml
  paths: ['/etc/**']
  exclude: ['/etc/{shadow,shadow-,gshadow,gshadow-,krb5.keytab,ipsec.secrets}', '/etc/ssh/ssh_host_*_key',
            '/etc/{ssl/private,pki/tls/private,letsencrypt/archive,letsencrypt/keys,kubernetes/pki,wireguard,NetworkManager/system-connections}/**',
            '/etc/udev/hwdb.bin']
  ```

- **`linux.web.webroot_scripts`** (new) — full, high; 10 MB to GBs.
  - Value: the webshell bodies themselves.
  - Needs a per-source size cap before it can move to `standard`.

  ```yaml
  ['{/var/www,/srv,/usr/share/nginx/html,/opt/lampp/htdocs}/**/*.{php,php3,php4,php5,php7,phtml,phar,inc,jsp,jspx,jspf,asp,aspx,cgi,pl,py,sh}',
   '%UserHome%/public_html/**/*.{php,phtml,phar,cgi,pl}', '/{opt,usr/share,var/lib}/*tomcat*/webapps/**/*.{jsp,jspx,jspf}']
  ```

- **`linux.esxi`** (new) — low confidence that Relic can run there; medium confidence on the paths.
  - Relic reports ESXi as `linux`, but ESXi 8 enables the `execInstalledOnly` runtime option by default,
    which blocks non-VIB binaries (Broadcom KB 344815, Truesec).
  - Synacktiv found that Velociraptor cannot run on ESXi either.
  - Validate Relic on ESXi 7 first, or package it as a VIB.
  - `local.sh` persistence was used by the Juniper-reported Python backdoor, ESXiArgs and Gentlemen
    (MITRE T1037.004).
  - Sources: FA `esxi.yaml` (`/var/run/log/*`, `hostd.log`, `shell.log`); Synacktiv (`/scratch/log`,
    `vmsyslog.conf`, `locker.conf`, `firewall/service.xml`); Mandiant VIRTUALPITA (malicious VIBs).

  ```yaml
  ['/var/run/log/*', '/scratch/log/*', '/etc/rc.local.d/*', '/etc/vmware/hostd/{vmInventory.xml,hostsvc.xml}',
   '/etc/vmware/{esx.conf,locker.conf}', '/etc/vmware/rhttpproxy/endpoints.conf', '/etc/vmware/firewall/*',
   '/etc/vmsyslog.conf', '/etc/ssh/keys-*/authorized_keys', '/var/db/esximg/{vibs,profiles}/*']
  # /scratch/log usually duplicates /var/run/log (needs inode dedup); /var/db/esximg is low confidence
  ```

### Checked and already covered

`/etc/ld.so.preload`, `ld.so.conf.d`, `/etc/environment`, `bash.bashrc`, `profile.d`, `update-motd.d`,
`/etc/xdg/autostart`, `~/.config/autostart`, `rc.local`, `/etc/init/*.conf`, `/etc/sudoers.d`,
`sshd_config.d`, `authorized_keys2`, `/var/spool/cron/**` (incl. Debian `atjobs`), `/var/spool/at`,
`modules-load.d`, `modprobe.d`, `/etc/udev/rules.d`, `resolv.conf`, `nsswitch.conf`, `fstab`, `crypttab`,
`exports`, `hosts.allow`/`deny`, `machine-id`, `pam.d`, `rsyslog.d`, `apt/history.log`, `audit.log*`,
`lastlog`, `faillog`, `/var/log/wtmp.db`, the viminfo/lesshst/wget/python/node/sqlite/redis histories,
`/var/log/{nginx,apache2,httpd,mysql,postgresql,samba,mail*,sa,sudo-io}` (via `var_log`), rpmdb (full).

## 6. Non-file evidence for the roadmap (needs new source types)

1. **Process source.** Collect pid, ppid, uid, start time, exe path, the `(deleted)` and `memfd:` flags,
   cmdline, cwd and namespaces. Hash `/proc/<pid>/exe` with (dev, inode) dedup and copy deleted binaries.
   Sources: UAC `hash_running_processes`; Sandfly.
2. **Socket-to-process mapping** (the equivalent of `ss -tulpanoe`): join `/proc/net/*` inodes with
   `readlink(/proc/<pid>/fd/*)`. Also list open and deleted files per process (lsof).
3. **Kernel integrity.** Compare `/proc/modules` with `/sys/module/*` and kallsyms to find hidden LKMs.
   Enumerate eBPF programs, maps and links through `bpf()` and the `/sys/fs/bpf` pins (BPFDoor, ebpfkit).
   Sources: UAC `kernel_modules`, `ebpf`, `bpftool`.
4. **File system timeline (bodyfile).** A stat walk recording MAC(B) times, mode, uid, SUID/SGID, the
   `security.capability` xattr, immutable/append-only flags, hidden files and world-writable dirs.
   Sources: UAC `bodyfile` and the `system/*` finders (in `ir_triage`); Velociraptor
   `Linux.Forensics.ImmutableFiles` and `Linux.Sys.SUID`.
5. **Metadata-only records.** Hashes and times without content, for credential stores (cloud CLI
   credentials, keytabs, SSH private keys). This shows access through atime while keeping secrets out of
   the archive.
6. **Live configuration state:** `nft list ruleset` / `iptables-save` (netlink), `systemctl list-units
   --all` / `list-timers` (D-Bus; includes transient units), `auditctl -l`, `getenforce`, `sysctl -a`,
   `ip addr/route/neigh` (netlink).
7. **Containers through API sockets.** Query `/run/docker.sock`, the containerd socket and the Podman API
   natively for list, inspect and diff. Container upper-dir diffs (`/var/lib/docker/overlay2/*/diff`) are
   where webshells written inside containers live.
8. **Kernel ring buffer.** Read `/dev/kmsg` non-blocking; unlike `/proc/kmsg`, this is non-destructive.
9. **Offline package verification.** debsums and `rpm -Va` equivalents from the collected md5sums or rpmdb
   plus file hashes.
10. **Memory.** Physical acquisition (AVML-style, UAC `memory_dump/avml`) and per-process memory strings
    (UAC `process_memory_strings`), opt-in or `full`.
11. **Search primitives.** Name patterns from `/`, max depth, file-type and ELF-magic/executable filters,
    and per-source size caps (UAC has `name_pattern`, `max_depth`, `file_type` and `max_file_size` per
    artifact). These are needed for git hooks, webroots and staging dirs.

## 7. Sources

- UAC: https://github.com/tclahr/uac/tree/main/artifacts (read: `files/system/*`, `files/logs/*`, `files/ssh/*`, `files/shell/{bash,common}`, `files/packages/*`, `files/applications/{git,php,rclone,anydesk,teamviewer,rustdesk,splashtop}`, `live_response/{process,network,system,containers}/*`, `memory_dump/coredump`, `chkrootkit/*`, `profiles/ir_triage.yaml`)
- ForensicArtifacts: https://github.com/ForensicArtifacts/artifacts/tree/main/artifacts/data (`linux`, `linux_proc`, `unix_common`, `shell`, `docker`, `containerd`, `kubernetes`, `esxi`, `webservers`, `tomcat`, `database_servers`, `config_files`)
- Velociraptor Linux artifacts: https://github.com/Velocidex/velociraptor/tree/master/artifacts/definitions/Linux ; CyLR: https://github.com/orlikoski/CyLR
- PANIX: https://github.com/Aegrah/PANIX ; Elastic series: https://www.elastic.co/security-labs/primer-on-persistence-mechanisms , https://www.elastic.co/security-labs/sequel-on-persistence-mechanisms , https://www.elastic.co/security-labs/approaching-the-summit-on-persistence , https://www.elastic.co/security-labs/the-grand-finale-on-linux-persistence
- Pepe Berba, "Hunting for Persistence in Linux": https://pberba.github.io/
- Sandfly: https://sandflysecurity.com/blog/deleted-process-binary-attack-on-linux , https://sandflysecurity.com/blog/linux-process-running-from-dev-shm-ram-disk-attack , https://sandflysecurity.com/blog/bpfdoor-an-evasive-linux-backdoor-technical-analysis/
- Nextron Plague: https://www.nextron-systems.com/2025/08/01/plague-a-newly-discovered-pam-based-backdoor-for-linux/
- MITRE: https://attack.mitre.org/techniques/T1546/018 , https://attack.mitre.org/techniques/T1037/004/
- Debian logging: https://www.thomas-krenn.com/en/wiki/Replacement_of_/var/log/syslog_by_journalctl_in_Debian_12 , https://www.debian.org/releases/trixie/release-notes/index.html , https://sources.debian.org/src/wtmpdb/0.75.0-5/README.md
- ESXi: https://synacktiv.com/publications/vmware-esxi-forensic-with-velociraptor , https://knowledge.broadcom.com/external/article/344815/esxi-80-prevents-the-execution-of-nonins.html , https://www.truesec.com/hub/blog/esxi-8-0-and-execinstalledonly-the-good-the-bad-and-the-ugly , https://cloud.google.com/blog/topics/threat-intelligence/esxi-hypervisors-malware-persistence , https://cymulate.com/threats/a-custom-python-backdoor-for-vmware-esxi-servers/ , https://zerolabs.rubrik.com/blog/under-hood-deconstructing-gentlemen-esxi-ransomware
- vCenter: https://knowledge.broadcom.com/external/article/381722/vpxd-along-with-other-services-not-start.html , https://knowledge.broadcom.com/external/article/427833 , https://cloud.google.com/blog/topics/threat-intelligence/vsphere-brickstorm-defender-guide
- AWS SSM: https://www.sans.org/blog/living-off-the-cloud , https://cadosecurity.com/ipc-you-how-the-cado-platform-reveals-attacker-command-outputs
- Docker/Podman: https://www.dbi-services.com/blog/all-about-how-docker-containers-and-data-are-stored-on-its-host/ , https://docs.datadoghq.com/containers/guide/podman-support-with-docker-integration/ , https://discussion.fedoraproject.org/t/podman-5-7-boltdb-to-sqlite-migration/171172

**Verification status** of paths not confirmed against a fetched source (check before shipping):

- Medium confidence: lastlog2 path, dnf5 `libdnf5-plugins`, Podman `userdata/ctr.log`,
  `networkd-dispatcher`, KDE `plasma-workspace/env`, VCSA subdirectory names, `/var/db/esximg`.
- High confidence (upstream manuals): systemd unit and generator search paths (`systemd.unit(5)`,
  `systemd.generator(7)`), Quadlet directories (`podman-systemd.unit(5)`), initramfs-tools and dhclient
  hook directories, k3s auto-deploy manifests, `/var/lib/systemd/{linger,timers}`, BusyBox `/etc/crontabs`,
  OpenRC `/etc/local.d`.
