# Incident Response Runbook

**Source:** ATLAS.md §13.3 (Threat model highlights), §14 (Observability and reliability), §16  
**Owner:** Both CEOs (incident commander), Internal CEO (technical lead)  
**Status:** Active  
**Last reviewed:** 2026-11-01

---

## Purpose

This runbook defines how Atlas responds to incidents: security breaches, system outages, data integrity issues, and service disruptions. It ensures incidents are detected, triaged, resolved, and reviewed consistently.

---

## Severity Levels

| Level | Definition | Examples | Response Time | Escalation |
|---|---|---|---|---|
| **SEV1** | Critical outage or security breach affecting all users | Atlas API down, data breach, authentication failure | Immediate (24x7) | L2 → Internal CEO → Both CEOs |
| **SEV2** | Major degradation affecting multiple users | API slow > 5s p95, outbox lag > 60s, WhatsApp webhook failing | Within 1 hour | L1 → L2 → Domain CEO |
| **SEV3** | Minor degradation affecting few users | Single module read-only, non-critical email delay | Within 4 hours | L1 → Project lead |
| **SEV4** | Cosmetic or low-impact issue | Typo in UI, minor styling issue | Next business day | L0 → Owner |

---

## Roles and Responsibilities

| Role | Responsibility |
|---|---|
| **Incident Commander** | Overall coordination, decision authority, stakeholder communication (CEO or delegate) |
| **Technical Lead** | Investigation, mitigation, deployment of fixes (Internal CEO or senior engineer) |
| **Communications Lead** | Internal and external stakeholder updates (Commercial CEO or delegate) |
| **Scribe** | Timeline documentation, post-incident review notes |
| **On-Call Engineer** | First response, initial triage, execution of mitigation steps |

---

## Incident Lifecycle

```
1. Detect       → 2. Triage       → 3. Mitigate       → 4. Resolve       → 5. Review
   (alert)       (SEV assigned)   (stop the bleeding) (service restored) (post-incident review)
```

---

## Step 1: Detect

### Detection Sources

| Source | What It Detects | Who Receives |
|---|---|---|
| System checks (HTTP/TCP/TLS/DNS/heartbeat) | System down, check failing | Systems module incidents |
| Outbox lag alert (newest seq − consumer cursor > 60s) | Elixir consumer stuck | Orchestrator on-call |
| API error rate spike (> 5% 5xx) | Backend errors | On-call engineer |
| Security alert (failed login burst, MFA bypass attempt) | Potential breach | Security lead + CEOs |
| User report | Issue not caught by monitoring | Support → on-call |

### Alert Routing

- SEV1/SEV2: WhatsApp + in-app + email to on-call + Internal CEO immediately
- SEV3: In-app + email to on-call
- SEV4: In-app only, reviewed next business day

---

## Step 2: Triage

### SEV Assessment Checklist

```
□ How many users are affected? (all / multiple / single)
□ Is data integrity at risk? (yes / no)
□ Is authentication or authorisation broken? (yes / no)
□ Can users read data? (yes / no)
□ Can users write data? (yes / no)
□ Is there evidence of unauthorised access? (yes / no)
```

### Severity Assignment

| If any of these... | Then SEV |
|---|---|
| Data breach, full outage, auth broken | SEV1 |
| Major degradation, outbox lag > 60s, webhook failures | SEV2 |
| Single module degraded, minor email delay | SEV3 |
| Cosmetic issue | SEV4 |

### Initial Actions (First 15 Minutes)

1. **Declare the incident** — Create an incident record in Atlas (systems module) with severity, title, description, and commander
2. **Open incident channel** — `incident:{id}` Phoenix Channel for responders + CEOs
3. **Assign scribe** — Document timeline from this point forward
4. **Notify stakeholders** — WhatsApp + in-app + email per severity
5. **Start mitigation** — Begin containment while investigating

---

## Step 3: Mitigate

### Containment Strategies by Incident Type

| Incident Type | Immediate Containment |
|---|---|
| **API outage** | Failover to secondary instance if available; otherwise, put API in maintenance mode |
| **Database outage** | Promote read replica if available; otherwise, maintenance mode |
| **Security breach** | Revoke compromised sessions/tokens; disable affected accounts; rotate credentials |
| **Data integrity issue** | Stop writes to affected tables; take snapshot for forensic analysis |
| **Outbox consumer stuck** | Restart Elixir orchestrator; check consumer cursor; manually advance if needed |
| **External provider failure** (WhatsApp, payments) | Switch to fallback provider if available; queue messages for later delivery |

### Do NOT

- Delete data to "fix" an issue
- Modify production data directly without audit trail
- Deploy untested code during an incident
- Share credentials in chat channels

---

## Step 4: Resolve

### Resolution Checklist

```
□ Root cause identified and documented
□ Fix deployed and verified
□ Service restored to normal operation
□ All affected users notified
□ Incident record updated with resolution details
□ Post-incident review scheduled (within 3 working days for SEV1/SEV2)
```

### Post-Resolution Actions

1. **Mark incident resolved** in Atlas with `resolved_at` timestamp
2. **Create post-incident review task** — due within 3 working days for SEV1/SEV2
3. **Notify affected clients** if client-facing systems were impacted
4. **Document lessons learned** — what worked, what didn't, what to improve

---

## Step 5: Post-Incident Review

### Review Template

```
1. Summary — What happened, when, and impact
2. Timeline — Key events from detection to resolution
3. Root cause — Why it happened (5 Whys analysis)
4. What went well — Effective detection, mitigation, communication
5. What went wrong — Delays, gaps, missed signals
6. Action items — Specific, owned, dated improvements
7. Prevention — How to prevent recurrence
```

### Action Item Tracking

- Each action item becomes an Atlas task with owner and due date
- High-priority items (from SEV1/SEV2) reviewed by both CEOs
- Recurring issues escalate to L3 (both CEOs)

---

## Communication Templates

### Internal Notification (SEV1/SEV2)

```
Subject: [SEV1/SEV2] Incident {id}: {title}

Atlas is experiencing a {severity} incident.

What: {description}
Impact: {users affected, functionality degraded}
Started: {timestamp}
Commander: {name}
Channel: incident:{id} (Phoenix Channel)

We are investigating and will provide updates every {interval}.
```

### Client Notification (if client-facing impact)

```
Subject: Important: {service} service update

We are currently experiencing issues with {service}. Our team is investigating and working to restore normal operation as quickly as possible.

We will provide updates as we have them.

Thank you for your patience.
—SafariStack
```

---

## Escalation Matrix

| Level | Trigger | Notify | Response Time |
|---|---|---|---|
| L0 | On-call engineer | On-call engineer | Immediate |
| L1 | Unacknowledged after E1 (24h) | Project lead / line manager | Within 1 hour |
| L2 | Overdue by E2 (48h) or blocker flagged | Domain CEO | Within 30 minutes |
| L3 | Cross-domain blocker, L2 unresolved after E3 | Both CEOs | Immediate (24x7) |

---

## Security Incident Specifics

If the incident involves a potential data breach:

1. **Contain immediately** — Revoke sessions, disable accounts, rotate credentials
2. **Preserve evidence** — Do not delete logs; take database snapshot
3. **Assess scope** — What data was accessed? Which users? Which systems?
4. **Legal review** — Kenya Data Protection Act notification requirements (72 hours for certain breaches)
5. **Notify affected users** — If personal data was compromised, notify within legally required timeframe
6. **Document everything** — Every action taken, by whom, and when

---

## Related Documents

- `docs/policies/approval-matrix.md` — both-CEO approval for significant changes
- `docs/policies/data-retention.md` — audit log retention (7 years)
- `docs/ATLAS.md §13.3` — threat model highlights
- `docs/ATLAS.md §14` — observability and reliability (SLOs: API availability 99.5%, outbox lag p99 < 10s)
- `docs/ATLAS.md §9.5` — incident response workflow
