# Cybersecurity Harness — Engagement Rules

These rules are mandatory for all agents in this harness. Every agent must check scope before any action. No exceptions.

---

## 1. Authorization

- **Never attack a target not in the explicit authorized scope.** If no scope document exists, stop and ask the operator before proceeding.
- Obtain and document written authorization before beginning any engagement.
- If authorization is ambiguous, treat the target as out-of-scope until clarified.
- Shared/cloud infrastructure (AWS, GCP, Azure) requires separate cloud-provider authorization in addition to target owner authorization.

## 2. Scope Constraints

- Only interact with IP ranges, domains, and services explicitly listed in the scope definition.
- Wildcard scopes (e.g., `*.example.com`) do not include third-party services hosted on subdomains (SaaS, CDN providers) unless explicitly stated.
- Production systems: treat with extreme care. Prefer read-only probes; escalate to operator before any write/modify/delete action.
- Out-of-scope findings: document the finding and the reason it is out-of-scope; do not exploit.

## 3. No Destructive Actions Without Explicit Approval

- **Default posture: passive/read-only.** Active exploitation requires explicit operator approval per target, per vulnerability class.
- Never drop tables, delete files, crash services, or modify production data.
- DoS/DDoS techniques are prohibited unless operator explicitly enables them in an isolated test environment.
- Social engineering and phishing are out-of-scope unless explicitly authorized in writing.

## 4. Evidence Preservation

- Log all tool invocations with timestamps, arguments, and raw output.
- Store screenshots and responses for every confirmed finding.
- Preserve chain-of-custody: never modify raw evidence; only annotate copies.
- Evidence must be stored securely — encrypted at rest where possible.

## 5. Responsible Disclosure

- Do not share vulnerability details with anyone outside the authorized engagement team.
- If a critical 0-day is found, notify the operator immediately before including it in the written report.
- Follow coordinated disclosure timelines agreed upon with the client.
- Never publish or sell findings; never use findings for personal gain outside the engagement.

## 6. Rate Limiting and Operational Safety

- Respect rate limits. Aggressive scanning can cause unintended denial of service.
- Default: throttle active scans to ≤10 req/sec unless client explicitly approves higher rates.
- Run heavy scans (nuclei full templates, sqlmap --level 5) only during agreed maintenance windows or against isolated staging systems.
- Monitor target availability during active scans; stop immediately if the target becomes unresponsive.

## 7. Legal Compliance

- Computer fraud laws (CFAA, Computer Misuse Act, etc.) apply regardless of perceived authorization. Written authorization is your only defense.
- Do not exfiltrate real PII, credentials, or sensitive data beyond what is necessary to prove a finding. Truncate or hash sensitive data in reports.
- Comply with all applicable data protection regulations (GDPR, HIPAA, etc.) when handling data encountered during testing.
- If you discover evidence of an ongoing third-party attack or criminal activity, stop and notify the operator immediately.

## 8. Communication Protocol

- Report critical findings (RCE, credential exposure, active exploitation evidence) to the operator immediately — do not wait for the final report.
- Use severity tiers: Critical / High / Medium / Low / Informational.
- Never speculate about exploitability; mark unconfirmed findings as "Potential" until validated.

---

All agents must include the phrase `[SCOPE VERIFIED]` in their output header before executing any active probe or scan, confirming they have checked the authorized scope for the current target.
