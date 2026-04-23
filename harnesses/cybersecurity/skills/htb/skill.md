---
name: htb
description: HackTheBox machine orchestrator. Systematic approach from initial scan to root flag. Auto-detects OS and routes to correct privilege escalation agent.
invocation: /htb <target-ip> [--name <machine-name>] [--difficulty <easy|medium|hard|insane>] [--output <dir>]
---

# HackTheBox Machine Orchestrator

## Overview

Systematic HTB machine pwn workflow. Six phases from nmap to root flag. Auto-detects Linux vs Windows and routes to correct privesc agent. All evidence collected per phase.

## Phases

```
Nmap Scan → Service Enum → Foothold → User Flag → Privilege Escalation → Root Flag
```

## Inputs

| Param | Required | Description |
|-------|----------|-------------|
| `target-ip` | yes | Target machine IP (e.g., 10.10.10.x) |
| `--name` | no | Machine name (for output organization) |
| `--difficulty` | no | Expected difficulty (adjusts enum depth) |
| `--output` | no | Output dir (default: `./htb-output/`) |

## Output

- `nmap/` — all nmap scan results
- `enum/` — service enumeration output
- `exploits/` — exploit code + payloads used
- `evidence/` — screenshots, flag files, proof
- `user.txt` — user flag
- `root.txt` — root flag
- `writeup.md` — full attack path documentation

## Agent Assignment Matrix

| Phase | Primary Agent | Fallback Agent | Condition |
|-------|--------------|----------------|-----------|
| Nmap Scan | **recon** | — | Always |
| Web Enum | **web-exploiter** | **dorker** | HTTP/HTTPS ports open |
| Service Enum | **enumerator** | **recon** | Non-web services |
| AD Enum | **ad-operator** | **adcs-attacker** | SMB + Kerberos (88/445) |
| ADCS Abuse | **adcs-attacker** | **ad-operator** | AD CS detected |
| Email Testing | **email-spoofer** | — | SMTP (25/587) detected |
| Foothold (Web) | **web-exploiter** | **rce-hunter** | Web vuln found |
| Foothold (Service) | **exploiter** | **rce-hunter** | Service vuln found |
| User Flag | **post-exploit** | — | Shell obtained |
| Privesc (Linux) | **privesc-linux** | **post-exploit** | Linux detected |
| Privesc (Windows) | **privesc-windows** | **post-exploit** | Windows detected |
| Privesc (AD) | **ad-operator** | **adcs-attacker** | Domain-joined Windows |
| Root Flag | **post-exploit** | — | Root/SYSTEM obtained |
| Report | **report-writer** | — | Always (final phase) |

---

## Phase 0: Setup and Scope Verification (MANDATORY)

```
1. Create output directory structure:
   mkdir -p $OUTPUT/{nmap,enum,exploits,evidence,loot}

2. Scope check — HTB machines are isolated, but verify:
   [SCOPE VERIFIED: $TARGET_IP — HackTheBox lab machine]

3. Add target to /etc/hosts if machine name known:
   echo "$TARGET_IP $MACHINE_NAME.htb" >> /etc/hosts

4. Set target vars:
   export TARGET=$TARGET_IP
   export MACHINE=$MACHINE_NAME
```

---

## Phase 1: Nmap Scan

**Agent:** `recon`

```
TASK → recon:
  "Run comprehensive nmap scan against $TARGET.

  1. Quick TCP scan — top 1000 ports:
     nmap -sC -sV -oA $OUTPUT/nmap/initial $TARGET

  2. Full TCP port scan:
     nmap -p- --min-rate 5000 -oA $OUTPUT/nmap/alltcp $TARGET

  3. UDP top 20:
     nmap -sU --top-ports 20 -oA $OUTPUT/nmap/udp $TARGET

  4. Targeted scripts on open ports:
     nmap -sC -sV -p$OPEN_PORTS -oA $OUTPUT/nmap/targeted $TARGET

  Save results to $OUTPUT/nmap/. Report all open ports + services."
```

**OS Detection Logic:**

```
IF port 135/139/445 open AND (port 88 open OR 'Windows' in service banner):
  → OS = Windows
ELIF 'SSH' on port 22 AND no Windows indicators:
  → OS = Linux
ELSE:
  → Check nmap OS detection (-O) or service banners
  → Default Linux if ambiguous

SAVE: export DETECTED_OS=linux|windows
```

---

## Phase 2: Service Enumeration

Route based on open ports:

### Web Services (80, 443, 8080, 8443, etc.)

**Agent:** `web-exploiter`

```
TASK → web-exploiter:
  "Enumerate web services on $TARGET.

  1. Tech stack fingerprint:
     whatweb http://$TARGET
     curl -sI http://$TARGET

  2. Directory brute:
     gobuster dir -u http://$TARGET -w /usr/share/seclists/Discovery/Web-Content/raft-medium-directories.txt -o $OUTPUT/enum/gobuster.txt

  3. Vhost enum:
     gobuster vhost -u http://$TARGET -w /usr/share/seclists/Discovery/DNS/subdomains-top1million-5000.txt

  4. If CMS detected → run CMS-specific scanner (wpscan, droopescan, joomscan)

  5. Check for known CVEs in identified versions

  Save all output to $OUTPUT/enum/."
```

### SMB (139, 445)

**Agent:** `enumerator` (or `ad-operator` if Kerberos present)

```
TASK → enumerator:
  "Enumerate SMB on $TARGET.

  1. Anonymous access:
     smbclient -L //$TARGET -N
     crackmapexec smb $TARGET -u '' -p ''
     crackmapexec smb $TARGET -u 'guest' -p ''

  2. Share enumeration:
     smbmap -H $TARGET -u '' -p ''

  3. RPC enumeration:
     rpcclient -U '' -N $TARGET -c 'enumdomusers; enumdomgroups; querydispinfo'

  4. If creds obtained later → re-enumerate with creds

  Save to $OUTPUT/enum/smb/."
```

### SMTP (25, 587)

**Agent:** `email-spoofer`

```
TASK → email-spoofer:
  "Enumerate SMTP on $TARGET.

  1. Banner grab + EHLO:
     swaks --to test@$MACHINE.htb --server $TARGET --quit-after EHLO

  2. VRFY user enumeration:
     smtp-user-enum -M VRFY -U /usr/share/seclists/Usernames/Names/names.txt -t $TARGET

  3. Check for open relay:
     swaks --to external@test.com --from admin@$MACHINE.htb --server $TARGET --quit-after RCPT

  Save to $OUTPUT/enum/smtp/."
```

### DNS (53)

**Agent:** `recon`

```
TASK → recon:
  "Enumerate DNS on $TARGET.

  1. Zone transfer attempt:
     dig axfr $MACHINE.htb @$TARGET

  2. Reverse lookup:
     dig -x $TARGET @$TARGET

  3. Subdomain brute:
     gobuster dns -d $MACHINE.htb -r $TARGET -w /usr/share/seclists/Discovery/DNS/subdomains-top1million-5000.txt

  Save to $OUTPUT/enum/dns/."
```

### Kerberos (88)

**Agent:** `ad-operator`

```
TASK → ad-operator:
  "Enumerate Kerberos on $TARGET.

  1. User enumeration:
     kerbrute userenum -d $MACHINE.htb --dc $TARGET /usr/share/seclists/Usernames/xato-net-10-million-usernames.txt

  2. AS-REP roast (no auth):
     GetNPUsers.py $MACHINE.htb/ -dc-ip $TARGET -usersfile users.txt -no-pass

  Save to $OUTPUT/enum/kerberos/."
```

---

## Phase 3: Foothold

**Agent:** `web-exploiter` (web) or `exploiter` (service)

```
TASK → web-exploiter or exploiter:
  "Obtain initial foothold on $TARGET.

  Based on enumeration findings:
  1. Identify most promising attack vector
  2. Search for public exploits: searchsploit, GitHub
  3. Prepare exploit — modify target IP, callback IP/port
  4. Start listener:
     nc -lvnp 4444  (or rlwrap nc -lvnp 4444)
  5. Execute exploit
  6. Stabilize shell:
     python3 -c 'import pty;pty.spawn(\"/bin/bash\")'  (Linux)
     OR use ConPtyShell / rlwrap for Windows

  Save exploit code to $OUTPUT/exploits/.
  Document exact steps in $OUTPUT/evidence/foothold.md."
```

---

## Phase 4: User Flag

**Agent:** `post-exploit`

```
TASK → post-exploit:
  "Retrieve user flag from $TARGET.

  Linux:
    find /home -name user.txt 2>/dev/null
    cat /home/*/user.txt

  Windows:
    dir C:\Users\*\Desktop\user.txt /s
    type C:\Users\<user>\Desktop\user.txt

  Save flag to $OUTPUT/user.txt.
  If current user cannot read flag → need lateral movement:
    - Check other users' home dirs
    - Look for creds in config files, databases, environment vars
    - Check for sudo / SUID / scheduled tasks / services"
```

---

## Phase 5: Privilege Escalation

### Route based on DETECTED_OS:

#### Linux → `privesc-linux`

```
TASK → privesc-linux:
  "Escalate privileges to root on $TARGET (Linux).

  1. Run linpeas.sh → save to $OUTPUT/enum/linpeas.txt
  2. Check sudo -l
  3. Find SUID binaries: find / -perm -4000 2>/dev/null
  4. Check crontabs: cat /etc/crontab; ls -la /etc/cron.*
  5. Check capabilities: getcap -r / 2>/dev/null
  6. Check writable paths, config files, SSH keys
  7. Check kernel version for known exploits

  Exploit identified vector. Obtain root shell.
  Save evidence to $OUTPUT/evidence/privesc.md."
```

#### Windows → `privesc-windows`

```
TASK → privesc-windows:
  "Escalate privileges to SYSTEM/Administrator on $TARGET (Windows).

  1. Run winPEAS.exe → save output
  2. Check whoami /priv — look for SeImpersonate, SeBackup, etc.
  3. Check services: sc query state=all
  4. Check scheduled tasks: schtasks /query /fo LIST /v
  5. Check installed software versions for known CVEs
  6. If SeImpersonate → PrintSpoofer / GodPotato / JuicyPotatoNG
  7. Check AlwaysInstallElevated, unquoted service paths, DLL hijacking

  Obtain SYSTEM shell.
  Save evidence to $OUTPUT/evidence/privesc.md."
```

#### Domain-Joined Windows → `ad-operator` + `adcs-attacker`

```
IF domain-joined (check: systeminfo | findstr "Domain"):

  TASK → ad-operator:
    "Enumerate AD attack paths. Run BloodHound, check Kerberoastable accounts,
    ACL abuse paths, delegation misconfigs."

  IF AD CS detected (certutil -config - -ping OR certipy find):
    TASK → adcs-attacker:
      "Enumerate and exploit AD CS misconfigurations (ESC1-ESC8).
      Target: $MACHINE.htb domain."
```

---

## Phase 6: Root Flag

**Agent:** `post-exploit`

```
TASK → post-exploit:
  "Retrieve root flag from $TARGET.

  Linux:
    cat /root/root.txt

  Windows:
    type C:\Users\Administrator\Desktop\root.txt

  Save flag to $OUTPUT/root.txt.
  Take screenshot of whoami + flag as proof."
```

---

## Completion Summary

```
TASK → report-writer:
  "Generate HTB writeup for $MACHINE.

  Include:
  - Machine info (name, IP, OS, difficulty)
  - Attack path summary (one paragraph)
  - Phase-by-phase walkthrough with exact commands
  - User flag: <hash>
  - Root flag: <hash>
  - Key takeaways / what made this machine interesting
  - Tools used

  Save to $OUTPUT/writeup.md."
```

## Error Handling

| Situation | Action |
|-----------|--------|
| Nmap shows no open ports | Retry with `-Pn` (host may block ICMP). Check VPN connection. |
| Web enum finds nothing | Try bigger wordlist, different extensions (`-x php,asp,txt`), check for vhosts |
| Foothold exploit fails | Verify versions match, check firewall rules, try alternative exploit |
| Shell dies immediately | Use staged payload, try different port, encode payload |
| Privesc vectors exhausted | Re-enumerate with new creds, check for kernel exploits, look for credential files |
| AD attack blocked | Check if AV/EDR interfering, try OPSEC-safe alternatives |
| Machine seems broken | `ping $TARGET` + check HTB forums. Reset machine if needed. |

## Evidence Preservation

- Every command output saved to `$OUTPUT/` with timestamp
- Screenshots of key moments (foothold, flag captures)
- Exploit source code preserved in `$OUTPUT/exploits/`
- All credentials found logged in `$OUTPUT/loot/creds.txt`
- Final writeup generated by report-writer agent
