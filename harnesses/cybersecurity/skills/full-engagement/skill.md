---
name: full-engagement
description: Full penetration test engagement orchestrator. Coordinates all 12 agents across the complete kill chain — external recon through post-exploitation and reporting. Master orchestrator for comprehensive engagements.
invocation: /full-engagement <target> [--scope <scope-file>] [--type <external|internal|hybrid>] [--cloud <provider>] [--ad <domain>] [--output <dir>]
---

# Full Penetration Test Engagement Orchestrator

## Overview

Master orchestrator for full-spectrum penetration testing engagements. Coordinates all 12 agents across the complete kill chain. Phases run sequentially by default; parallel execution enabled where phases are independent and operator authorizes simultaneous testing.

## Kill Chain

```
Scope → External Recon → OSINT → Scanning → Web Testing → Network/AD → Cloud → PrivEsc → Post-Exploit → Reporting
```

All 12 agents participate. Phase ordering enforced — no exploitation before recon, no post-exploit before initial access confirmed.

## Inputs

| Param | Required | Description |
|-------|----------|-------------|
| `target` | yes | Primary target (domain, IP range, or org name) |
| `--scope` | yes | Scope file (REQUIRED for full engagement — targets, exclusions, rules) |
| `--type` | no | Engagement type: `external` (default), `internal`, `hybrid` |
| `--cloud` | no | Cloud provider(s) in scope: `azure`, `aws`, `gcp`, or comma-separated |
| `--ad` | no | Active Directory domain if in scope (e.g., corp.local) |
| `--output` | no | Output dir (default: `./full-engagement-output/`) |
| `--skip` | no | Comma-separated phases to skip (e.g., `--skip cloud,ad`) |

## Output

- `master-findings.json` — all findings unified, deduplicated, CVSS-scored
- `attack-narrative.md` — kill chain narrative from initial access to maximum impact
- `evidence/` — phase-organized evidence tree
- `pentest-report.md` — full professional report
- `remediation-summary.md` — engineering task tracker
- `compliance-gap-report.md` — framework compliance mapping

---

## Phase 0: Scope Verification

**MANDATORY — full engagement requires explicit written authorization.**

```
SCOPE CHECK — FULL ENGAGEMENT:
1. REQUIRE --scope file — NO exceptions for full engagements
2. Parse scope file:
   - Authorized targets (IPs, CIDRs, domains, cloud accounts, AD domains)
   - Excluded targets (out-of-scope systems, third-party services)
   - Authorized attack techniques (confirm: social engineering? phishing? DoS?)
   - Maximum authorized impact level (confirm: DA takeover? data access? cloud owner?)
   - Rules of engagement (testing window, rate limits, notification contacts)
   - Emergency stop contact (who to call if something breaks)
3. Validate scope file has been signed/approved — ask operator to confirm
4. Distribute scope constraints to ALL agents before any active testing
5. Save evidence/00-scope/scope-definition.md
6. Print engagement summary:

ENGAGEMENT AUTHORIZED:
  Target: $TARGET
  Type: $TYPE
  Scope: $N_HOSTS hosts, $N_DOMAINS domains
  Cloud: $CLOUD_PROVIDERS (or N/A)
  AD: $AD_DOMAIN (or N/A)
  Window: $TESTING_WINDOW
  Emergency contact: $CONTACT
  Proceeding with Phase 1 in 30 seconds. Ctrl+C to abort.
```

**Failure**: No --scope file or operator declines confirmation → HALT.

---

## Phase 1: External Reconnaissance

**Agent**: `recon`

```
SPAWN recon agent:
  task: "External reconnaissance on $TARGET"
  constraints: Scope: $SCOPE — passive and semi-passive only in this phase
  focus:
    - DNS enumeration (subdomains, MX, TXT records, zone transfer)
    - WHOIS and registrar information
    - ASN and IP range discovery
    - Technology fingerprinting (Shodan, Censys, passive)
    - SSL/TLS certificate transparency logs (crt.sh)
    - Web presence mapping (subdomains → live hosts)
    - Email addresses and employee names (for phishing scope awareness)
    - GitHub/GitLab/Bitbucket: exposed code, secrets, infrastructure clues
    - Cloud provider detection (AWS/Azure/GCP ranges in use)
  output:
    - evidence/01-recon/dns-enum.md
    - evidence/01-recon/ip-ranges.md
    - evidence/01-recon/tech-stack.md
    - evidence/01-recon/cloud-presence.md
    - evidence/01-recon/recon-summary.json
```

---

## Phase 2: OSINT

**Agent**: `dorker`

```
SPAWN dorker agent:
  task: "OSINT and Google/Shodan dorking for $TARGET"
  input: evidence/01-recon/recon-summary.json
  constraints: Scope: $SCOPE — passive intelligence gathering only
  focus:
    Google dorks:
      site:$DOMAIN filetype:pdf confidential
      site:$DOMAIN inurl:admin OR inurl:login OR inurl:dashboard
      site:$DOMAIN ext:env OR ext:config OR ext:bak
      "powered by" $TECH site:$DOMAIN
      intext:"$COMPANY" filetype:xlsx OR filetype:csv
    Shodan:
      org:"$COMPANY" → exposed services, banners, versions
      hostname:$DOMAIN port:8080 OR port:8443 → dev/staging exposure
      ssl:"$DOMAIN" → all SSL certs → hidden subdomains
    GitHub:
      $DOMAIN api_key OR secret OR password
      $COMPANY internal OR private OR confidential
    Data breach checks (HaveIBeenPwned API, DeHashed if authorized):
      Email domains → leaked credentials (passwords masked in report)
  output:
    - evidence/02-osint/google-dorks.md
    - evidence/02-osint/shodan-results.md
    - evidence/02-osint/github-exposure.md
    - evidence/02-osint/breach-exposure.md (masked)
    - evidence/02-osint/osint-summary.json
```

---

## Phase 3: Automated Scanning

**Agent**: `scanner`

```
SPAWN scanner agent:
  task: "Automated vulnerability scanning of $TARGET attack surface"
  input: evidence/01-recon/ + evidence/02-osint/
  constraints:
    - Scope: $SCOPE
    - Respect rate limits from scope file
    - Alert operator if scan generates significant traffic
  tools: nmap, nuclei, nikto, ZAP, nessus/openvas (if available)
  scope:
    - Port scan all in-scope IPs (full port range for external; top-1000 for internal ranges)
    - Service fingerprinting on open ports
    - Web vulnerability scanning on all discovered web services
    - Nuclei templates: CVE, misconfigurations, exposures, default-logins
    - SSL/TLS weakness scanning
    - Default credential testing (common services: SSH, FTP, SMB, RDP, web admin)
  output:
    - evidence/03-scan/port-scan-results.xml
    - evidence/03-scan/web-scan-results.md
    - evidence/03-scan/nuclei-findings.json
    - evidence/03-scan/scan-summary.json
```

---

## Phase 4: Web Application Testing

**Agent**: `web-exploiter` (primary) + `scanner` (reviewer)

```
Invoke sub-skill: /pentest-web for each web application in scope
  input: evidence/03-scan/web-scan-results.md → extract web targets
  FOR EACH web application target:
    Execute pentest-web phases 1-5 (recon already done → start at Phase 2)
    output: evidence/04-web/$HOST/

  web-exploiter agent handles exploitation
  scanner agent handles validation
  Max 3 exploitation attempts per finding
  
COLLECT → evidence/04-web/web-findings-summary.json
```

---

## Phase 5: Network and Active Directory

**Conditional**: only if internal network or --ad flag provided.

```
IF --ad $DOMAIN provided OR internal network in scope:
  Invoke sub-skill: /pentest-ad $DOMAIN --dc $DC_IP
    input: evidence/03-scan/ (DC identified from scan)
    ad-operator handles all AD phases
    post-exploit handles per-host post-exploitation
    output: evidence/05-ad/

ELSE IF internal network only (no AD):
  SPAWN recon + scanner agents:
    task: "Internal network lateral movement mapping"
    focus: service exploitation, credential reuse, network segmentation gaps
    output: evidence/05-network/

COLLECT → evidence/05-ad-network/summary.json
```

---

## Phase 6: Cloud Identity and Infrastructure

**Conditional**: only if --cloud flag provided.

```
IF --cloud $PROVIDERS provided:
  FOR EACH provider in $PROVIDERS:
    Invoke sub-skill: /pentest-cloud $PROVIDER $ACCOUNT_ID
      cloud-identity handles all cloud phases
      vuln-researcher assists with CVE research
      output: evidence/06-cloud/$PROVIDER/

COLLECT → evidence/06-cloud/cloud-findings-summary.json
```

---

## Phase 7: Privilege Escalation

**Agent**: `privesc-windows` and/or `privesc-linux`

```
FOR EACH host where initial access was obtained (web-exploiter / AD / cloud compute):
  Invoke sub-skill: /privesc $HOST
    OS auto-detected
    privesc-windows → Windows hosts
    privesc-linux → Linux hosts
    Escalation attempts per authorized scope
    output: evidence/07-privesc/$HOST/

COLLECT → evidence/07-privesc/privesc-summary.json
```

---

## Phase 8: Post-Exploitation

**Agent**: `post-exploit`

```
FOR EACH host where escalation was achieved (or confirmed access):
  SPAWN post-exploit agent:
    task: "Full post-exploitation on $HOST — maximum authorized impact demonstration"
    input: evidence/07-privesc/$HOST/
    constraints:
      - Scope: $SCOPE
      - Operator re-authorizes each host for post-exploitation
    phases:
      - System enumeration
      - Credential harvesting (Mimikatz / LaZagne / secretsdump as applicable)
      - Lateral movement mapping (document paths, execute only if per-host authorized)
      - Persistence mechanism enumeration (document, do not install)
      - C2 viability assessment (document, do not deploy)
      - Data exfiltration path assessment (document volume/methods, do not copy PII)
      - LOLBins/LOLBas available
      - Anti-forensics awareness
    output: evidence/08-post-exploit/$HOST/

COLLECT → evidence/08-post-exploit/post-exploit-summary.json
```

---

## Phase 9: Vulnerability Research

**Agent**: `vuln-researcher`

```
SPAWN vuln-researcher agent:
  task: "Deep CVE and vulnerability research supporting all prior phases"
  input: evidence/ (all phases — specifically versions, service banners, tech stack)
  focus:
    - CVEs for identified software versions not caught by automated scanning
    - Zero-day research context (are any identified services known-vulnerable?)
    - PoC availability and exploitability assessment for scanner-only findings
    - Supply chain risk indicators
    - Vendor security advisories for identified products
  output: evidence/09-vuln-research/
    - cve-analysis.md
    - unvalidated-findings-research.md (elevates scanner-only findings with research backing)
```

*Note: vuln-researcher runs in parallel with post-exploitation (Phase 8) where operator authorizes concurrent testing.*

---

## Phase 10: Reporting

**Agent**: `report-writer`

```
SPAWN report-writer agent:
  task: "Generate full engagement penetration test report"
  input: evidence/ (all phases — master-findings.json)
  
  Pre-report: deduplicate findings across all phases
    - Same vuln found in web + AD → merge with both detection methods noted
    - CVSS scoring for all confirmed findings
    - Compliance mapping (frameworks from scope file)
  
  output:
    - pentest-report.md (full report)
    - remediation-summary.md (engineering tracker)
    - compliance-gap-report.md (framework gaps)
    - attack-narrative.md (kill chain story: initial → DA/root/cloud-owner)
    - master-findings.json
```

---

## Agent Assignment Matrix

| Phase | Primary Agent | Supporting Agent |
|-------|--------------|-----------------|
| 0: Scope | (orchestrator) | — |
| 1: Recon | recon | — |
| 2: OSINT | dorker | recon |
| 3: Scanning | scanner | — |
| 4: Web Testing | web-exploiter | scanner |
| 5: AD/Network | ad-operator | recon, post-exploit |
| 6: Cloud | cloud-identity | vuln-researcher |
| 7: PrivEsc | privesc-windows / privesc-linux | — |
| 8: Post-Exploit | post-exploit | — |
| 9: Vuln Research | vuln-researcher | — |
| 10: Reporting | report-writer | — |

---

## Error Handling

| Scenario | Action |
|----------|--------|
| Phase produces zero findings | Log as clean, continue — document as scope coverage |
| Agent blocked / rate limited | Back off, note in report, continue other phases |
| Scope violation detected by any agent | HALT all activity, notify operator immediately |
| System instability caused by testing | Stop phase, notify emergency contact, restore state |
| Cloud API costs exceed estimate | Pause cloud phases, notify operator |
| Credentials expire mid-engagement | Request refresh from operator; halt affected phases |

## Evidence Preservation

All agents MUST:
1. Timestamp every action and finding
2. Never overwrite prior phase evidence
3. Cross-reference findings with phase + finding ID (e.g., `PHASE4-F-003`)
4. Capture tool version and exact command line for every tool used
5. Hash all evidence files at conclusion (SHA-256) for chain of custody

## Completion

```
FULL ENGAGEMENT COMPLETE
Target: $TARGET
Engagement type: $TYPE
Phases completed: $N/10
Testing window: $START → $END

FINDINGS SUMMARY:
  Critical: X
  High: Y
  Medium: Z
  Low: W
  Informational: V

Highest impact achieved: [e.g., "Domain Admin via Kerberoasting → privilege escalation → DCSync"]

Deliverables:
  Full report: $OUTPUT_DIR/pentest-report.md
  Remediation tracker: $OUTPUT_DIR/remediation-summary.md
  Compliance gaps: $OUTPUT_DIR/compliance-gap-report.md
  Attack narrative: $OUTPUT_DIR/attack-narrative.md
```
