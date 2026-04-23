# Claudio Cybersecurity Harness

A comprehensive penetration testing and security assessment harness for Claudio. Covers the full PTES methodology — asset reconnaissance, service enumeration, vulnerability scanning, exploitation, static code analysis, post-exploitation, and professional report generation — using a coordinated team of specialized agents backed by industry-standard open-source tools.

## Quick Install

```sh
claudio harness install gh:user/claudio-cybersecurity
```

Or from a local clone:

```sh
claudio harness install ./harnesses/cybersecurity
```

## Setup

Install all required tools (detects macOS, Debian/Ubuntu, Fedora/RHEL, Arch):

```sh
./setup.sh
# or, from any directory:
.claudio/harnesses/cybersecurity/setup.sh
```

The script is idempotent — safe to run multiple times. It checks each tool before installing and prints a summary of installed / already-present / failed items.

## Available Skills

| Skill | Description |
|---|---|
| `/pentest` | Full PTES engagement orchestrator — runs all phases from scoping through reporting |
| `/pentest-web` | OWASP Top 10 web app pentest — focused on web-specific vulnerabilities |
| `/pentest-api` | REST/GraphQL API security audit — tests OWASP API Security Top 10 |
| `/pentest-recon` | Reconnaissance-only workflow — passive + active recon, comprehensive asset inventory |
| `/pentest-network` | Network and infrastructure pentest — services, protocols, segmentation, lateral movement |
| `/pentest-code-review` | Source code security audit — SAST, manual review, dependency audit, secrets scanning |
| `/pentest-report` | Report generation from existing findings — consolidates, scores, formats professional report |

## Agent Roster

| Agent | Model | Role |
|---|---|---|
| `recon` | sonnet | Asset discovery and reconnaissance. Maps attack surface via OSINT, port scanning, service fingerprinting, and technology identification. |
| `enumerator` | haiku | Enumeration — directory brute-force, endpoint discovery, API surface mapping, parameter fuzzing. |
| `scanner` | sonnet | Vulnerability scanning via template-based detection and DAST. Runs nuclei, OWASP ZAP, nikto. |
| `exploiter` | opus | Exploitation validation — SQLi, XSS, SSRF, auth bypass, IDOR. Every finding validated with PoC before reporting. |
| `code-auditor` | sonnet | Static analysis — OWASP Top 10 code patterns, dependency vulns, secrets exposure via Semgrep and Snyk. |
| `post-exploit` | sonnet | Post-exploitation — privilege escalation paths, lateral movement, persistence, sensitive data exposure. |
| `reporter` | sonnet | Report generation — CVSS scoring, executive summary, technical findings, remediation plan. |

## Required API Keys

Both are optional but unlock additional capabilities:

| Key | Used by | Get it |
|---|---|---|
| `SHODAN_API_KEY` | `recon` agent (Shodan MCP) | https://account.shodan.io |
| `SNYK_TOKEN` | `code-auditor` agent (Snyk MCP) | https://app.snyk.io/account |

Set in your shell profile or in `.claudio/settings.json` under `env`:

```json
{
  "env": {
    "SHODAN_API_KEY": "your-key-here",
    "SNYK_TOKEN": "your-token-here"
  }
}
```

## Tool Reference

| Tool | Agents that use it | Install method |
|---|---|---|
| `nmap` | recon, scanner | package manager |
| `subfinder` | recon | `go install` |
| `httpx` | recon, enumerator | `go install` |
| `whatweb` | recon | brew / gem |
| `ffuf` | enumerator | brew / `go install` |
| `feroxbuster` | enumerator | brew / cargo |
| `nuclei` | scanner | `go install` |
| `nikto` | scanner | package manager |
| `sqlmap` | exploiter | package manager |
| `OWASP ZAP` | scanner | brew cask / zaproxy.org |
| `jq` | all agents | package manager |
| `linpeas` | post-exploit | auto-downloaded at runtime |
| `semgrep` | code-auditor | MCP server |
| `snyk` | code-auditor | MCP server (`SNYK_TOKEN` required) |
| `shodan` | recon | MCP server (`SHODAN_API_KEY` required) |
| `Node.js 18+` | ZAP integration | nvm or package manager |

## Notes

- **linpeas** — the `linpeas-audit` plugin downloads linpeas at runtime from GitHub. No manual install needed.
- **Go tools** (`subfinder`, `httpx`, `nuclei`, `ffuf`) — require Go installed. `setup.sh` will warn if Go is missing. Binaries land in `$(go env GOPATH)/bin` — ensure that directory is in your `PATH`.
- **OWASP ZAP** — on Linux, if not available via package manager, download the installer from https://www.zaproxy.org/download/ or run `snap install zaproxy --classic`.
- All tools are free and open-source. No Burp Suite Pro required.
