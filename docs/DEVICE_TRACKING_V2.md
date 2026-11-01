# Device & Location Tracking (Hardened) — V2 Upgrade Proposal

> **Status:** Deferred / Future release (v2). Not part of the current Phase 3 (CRM & contracts) scope.  
> **Owner:** Internal & Technical CEO (author) · Commercial CEO (co-owner)  
> **Stack:** Next.js + TypeScript · Go · Elixir/OTP · PostgreSQL  
> **Related:** `docs/ATLAS.md` §2 (Engineering Principles), `docs/PHASE2.md`, `docs/PHASE3.md`  
> **Target:** SafariStack Solutions internal operating system (Atlas)

---

## 0. Purpose of this document

This document is a **proposed design only**. It captures the architecture, trade-offs, privacy considerations, and rollout plan for a hardened device and location tracking system that can run as an Atlas v2 upgrade after the core platform is stable and deployed.

It is intentionally **not** scheduled for immediate development. The team should treat it as a reference for a future decision gate, not as an active backlog item.

---

## 1. Why this exists

Atlas is the single source of truth for SafariStack operations. As the platform matures, leadership has asked whether Atlas can:

- Detect when a login or session originates from an unrecognized or unexpected device.
- Detect when a user's claimed location is inconsistent with network and system signals (e.g., VPN masking).
- Support NGO field workers whose physical location matters for duty of care and compliance.

Standard HTTP headers (`X-Forwarded-For`, `User-Agent`) are easily spoofed or altered by VPNs and proxies, so they are insufficient for this purpose. This proposal outlines a hybrid approach that combines browser telemetry, an optional native Go agent, hardware fingerprinting, and coordination between the Go API and Elixir orchestrator.

---

## 2. Goals

| Goal | Description |
|------|-------------|
| **Device recognition** | Identify whether a login attempt comes from a previously enrolled machine, using a cryptographic hardware fingerprint (hash, not raw serials). |
| **VPN-aware location signals** | Gather data points a VPN tunnel cannot fully alter: WebRTC local IPs, Wi-Fi BSSID scan (native agent only), system timezone, network latency. |
| **Go + Elixir coordination** | Go ingests and validates telemetry; Elixir applies business logic (match fingerprint to person, flag mismatch, alert or revoke). |
| **Privacy by design** | Store only hashed fingerprints, never raw hardware serials. Gate the entire feature behind a typed setting and explicit consent. |
| **Non-disruptive default** | Feature disabled by default. Enable per-department or per-person after consent. |

---

## 3. Non-goals (explicitly out of scope for v2)

- Continuous real-time location surveillance of all employees.
- Wi-Fi triangulation without a native agent and OS-level permissions.
- Replacement of existing authentication (email/phone + TOTP).
- gRPC mTLS between Go and Elixir unless the outbox pattern proves insufficient (see §7).
- Any feature that conflicts with Atlas principle #8: *"Accountability without unnecessary surveillance."*

---

## 4. Design principles (from `docs/ATLAS.md`)

This feature must comply with the existing Atlas engineering principles:

1. **Reliable data** — Fingerprint hashes are deterministic; telemetry is append-only and audited.
2. **Security by design** — Telemetry endpoints are authenticated; payloads are integrity-protected where possible.
3. **Accountability without unnecessary surveillance** — The feature is opt-in/consent-based, purpose-limited, and retention-bounded.
4. **Automation with control** — Mismatches trigger alerts for human review by default; auto-revoke is opt-in.
5. **Minimise personal data** — Hash before store; no raw MACs, serials, or Wi-Fi BSSIDs in the database.
6. **Dogfood relentlessly** — The team uses the feature internally before broader rollout.

---

## 5. Strategy to defeat VPN masking

A VPN changes the public IP seen by a web server, but it cannot alter everything. This proposal uses three signal layers:

### 5.1 WebRTC network traversal (browser)

- **Mechanism:** A lightweight frontend script (Next.js) uses WebRTC STUN requests to discover the browser's local private IP addresses and, in some configurations, the true public IP.
- **VPN resistance:** Partial. Modern browsers let users disable WebRTC; some VPNs actively block or leak-proof STUN.
- **Reliability:** Low-to-medium as a standalone signal; useful as one input to a confidence score.

### 5.2 Wi-Fi BSSID scan + geolocation (native Go agent only)

- **Mechanism:** A native Go binary (`atlas-agent`) scans nearby Wi-Fi access point BSSIDs (MAC addresses) and queries a geolocation service (e.g., Google Geolocation API) to estimate physical position within a few meters.
- **VPN resistance:** High. The VPN tunnel does not change nearby Wi-Fi MAC addresses.
- **Reliability:** High in urban areas; low in rural areas; requires OS-level Wi-Fi access (often admin/root).

### 5.3 Timezone & latency cross-referencing (browser + agent)

- **Mechanism:** Compare the VPN-geolocated IP against:
  - System clock / `Intl.DateTimeFormat` timezone.
  - Round-trip latency to known edge endpoints.
- **VPN resistance:** Medium. Easy to spoof individually, but hard to spoof all three consistently.
- **Reliability:** Medium; best used as a consistency check, not proof.

### 5.4 Confidence scoring

Rather than a binary "VPN / not VPN" decision, the system computes a **location confidence score** (0–100):

| Signal | Weight | Notes |
|--------|--------|-------|
| Fingerprint matches enrolled device | High | Strong identity signal |
| Timezone matches expected region | Medium | Easy to check, easy to spoof |
| Latency profile matches expected ISP | Medium | Requires baseline measurements |
| WebRTC local IP matches expected subnet | Low | Browser-dependent |
| Wi-Fi BSSID geolocation matches expected area | High | Native agent only |
| VPN IP contradicts all other signals | Negative | Triggers flag |

---

## 6. Machine fingerprinting (hardware tracking)

### 6.1 Inputs (Go)

| Source | Windows | Linux | macOS | Spoofability |
|--------|---------|-------|-------|--------------|
| OS machine ID | `MachineGuid` registry | `/etc/machine-id` | `IOPlatformUUID` | Medium (admin-changeable) |
| Host ID / kernel | `gopsutil` host | `gopsutil` host | `gopsutil` host | Medium |
| CPU / motherboard UUID | WMI | DMI/SMBIOS | `ioreg` | Medium (root-changeable) |
| TPM-bound ID (optional) | TPM 2.0 | TPM 2.0 | Secure Enclave | Low (requires hardware) |

### 6.2 Hashing (Go)

Raw hardware identifiers are **never stored**. The agent/client combines:

```
raw = machine_id + host_id + kernel_version + user_id + app_salt
fingerprint = SHA-256(raw)
```

The `app_salt` prevents cross-application tracking leaks (the same machine cannot be correlated across unrelated services).

### 6.3 What the fingerprint proves

- **Same machine:** If two logins produce the same hash, they almost certainly came from the same enrolled device.
- **Not identity proof:** It tells you "this is the same machine as before," not "this is Employee X." It must be bound to a person through an enrollment step.

---

## 7. Architecture: Go backend + Elixir orchestrator

### 7.1 Recommended pattern (extend existing outbox)

Atlas already uses a **PostgreSQL transactional outbox** between Go and Elixir. The recommended integration is:

```
Browser / Native Agent
        │ HTTPS + signed JWT / session cookie
        ▼
Go API (telemetry ingestion)
        │ validate session + JWT
        │ compute fingerprint hash
        │ write device_telemetry row + outbox event in same DB tx
        ▼
PostgreSQL outbox (core.device.telemetry.received.v1)
        ▼
Elixir outbox consumer
        │ business logic: fingerprint → person?
        │ location consistency check
        │ alert / flag / optional revoke
        ▼
Realtime signal (user:{account_id} topic) + audit log
```

**Why this pattern:** It reuses existing infrastructure, avoids introducing a new synchronous dependency, and survives Elixir downtime (events queue in PostgreSQL).

### 7.2 Alternative: gRPC over mTLS (not recommended for v1/v2)

The original blueprint suggested gRPC + mutual TLS between Go and Elixir. This is technically possible but adds operational burden (cert issuance, rotation, trust stores) and creates a synchronous coupling that can fail closed. Only consider this if the outbox pattern proves insufficient for low-latency requirements.

### 7.3 Communication security

- **Client → Go:** Authenticated via existing session/JWT. Telemetry endpoint is rate-limited. Real TCP `RemoteAddr` is used, not `X-Forwarded-For` (trusted-proxy middleware behind Caddy).
- **Go → Elixir:** Via outbox (database-transactional), no new network protocol required.
- **Optional endpoint integrity:** Provide each enrolled device with a provisioning key stored in the OS keychain; the agent signs telemetry payloads. Go verifies the signature before trusting the fingerprint. This defeats remote injection but not a root user on the device.

---

## 8. Data model (PostgreSQL, Go schema)

```sql
-- Enrolled device fingerprints (one per person+device)
CREATE TABLE core.device_fingerprints (
    id            UUID PRIMARY KEY,
    person_id     UUID NOT NULL REFERENCES core.people(id),
    fingerprint   VARCHAR(64) NOT NULL UNIQUE,  -- SHA-256 hex, never raw serials
    device_name   VARCHAR(255),                  -- user-friendly label, e.g. "Laptop-01"
    enrolled_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    consent_given BOOLEAN NOT NULL DEFAULT FALSE,
    consent_at    TIMESTAMPTZ
);

-- Append-only telemetry events (bounded by retention policy)
CREATE TABLE core.device_telemetry (
    id              UUID PRIMARY KEY,
    person_id       UUID REFERENCES core.people(id),
    fingerprint     VARCHAR(64),
    vpn_indicated   BOOLEAN,                        -- public IP geolocation vs expected
    reported_lat    DOUBLE PRECISION,
    reported_lon    DOUBLE PRECISION,
    confidence      SMALLINT,                       -- 0-100
    flags           JSONB,                          -- e.g. {"timezone_mismatch": true}
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes
CREATE INDEX idx_device_fingerprints_person ON core.device_fingerprints(person_id);
CREATE INDEX idx_device_telemetry_created ON core.device_telemetry(created_at DESC);
CREATE INDEX idx_device_telemetry_fingerprint ON core.device_telemetry(fingerprint, created_at DESC);
```

### Retention policy

Telemetry is append-only and grows with every check. A retention job (Elixir Oban or Go worker) should purge rows older than a configured window (recommended: 30–90 days).

---

## 9. API contracts

### 9.1 Go API

```
POST /api/v1/telemetry/device-check
Authorization: Bearer <session JWT>

Request body:
{
  "fingerprint": "sha256-hex",
  "timezone": "Africa/Nairobi",
  "webrtc_local_ips": ["192.168.1.45", "10.0.2.15"],
  "latency_ms": {"edge_us_east": 45, "edge_eu_west": 210},
  "wifi_bssids": ["aa:bb:cc:dd:ee:ff"],       -- native agent only
  "device_name": "Field Laptop"
}

Response:
{
  "fingerprint": "sha256-hex",
  "known_device": true,
  "confidence_score": 78,
  "flags": ["timezone_mismatch"],
  "action": "flag_for_review"   -- or "allow", "revoke_session"
}
```

### 9.2 Elixir outbox event

```
core.device.telemetry.received.v1
{
  "person_id": "uuid",
  "fingerprint": "sha256-hex",
  "confidence_score": 78,
  "flags": ["timezone_mismatch"],
  "received_at": "ISO8601"
}
```

---

## 10. Verification workflow (Elixir business logic)

1. **Handshake:** Employee logs in. Frontend script or Go agent hashes hardware IDs and captures network state.
2. **Go ingestion:** Validates session, reads real TCP connection info, computes latency, hashes fingerprint, writes telemetry + outbox event.
3. **Elixir orchestration:**
   - Is this fingerprint registered to this person?
   - Is the reported location consistent with the person's expected region/country?
   - Is the confidence score above the configured threshold?
4. **Outcome:**
   - **Match:** Allow session, update `last_seen_at`.
   - **Mismatch, low confidence:** Flag for review; emit `core.device.alert.v1`; notify manager via realtime.
   - **Mismatch, high confidence + auto-revoke enabled:** Revoke session (via Go authz), alert both CEOs.

---

## 11. Security & spoofing-resistance assessment

| Claim | Reality |
|-------|---------|
| "X-Forwarded-For cannot be trusted" | Correct. Use `conn.RemoteAddr()` from the TLS-terminated connection. Behind Caddy, use PROXY protocol or trusted-proxy middleware. |
| "Machine ID is un-spoofable" | Overstated. Root/admin users can change `/etc/machine-id`, registry GUIDs, etc. Treat as strong signal, not cryptographic proof. |
| "SHA-256 prevents storing raw serials" | Correct and required. Hash before persist. |
| "mTLS prevents spoofed payloads" | mTLS secures the channel, not the payload. A compromised agent can still send valid telemetry over mTLS. Use per-device signing keys for endpoint integrity. |
| "WebRTC bypasses VPN" | Not reliable. WebRTC can be disabled; corporate VPNs often route or block STUN. |

---

## 12. Privacy, legal, and ethics (critical)

This is the highest-risk section and must be resolved **before** any code is written.

### 12.1 Conflict with existing principles

Atlas principle #8 states: *"Accountability without unnecessary surveillance."* Continuous hardware fingerprinting + Wi-Fi triangulation + location tracking is surveillance by definition. This feature must be **narrowly scoped** and **consent-based**.

### 12.2 Legal considerations

- Location data and hardware identifiers are personal data under GDPR, CCPA, and many labor laws.
- Employee monitoring typically requires:
  - Written consent or works-council agreement
  - Purpose limitation (specific, disclosed purpose — not general productivity surveillance)
  - Data minimization and retention limits
  - Right to access and delete
- Revoking a session because a VPN IP says Iceland but a clock says Nairobi could block legitimate travelers or remote workers.

### 12.3 Recommended safeguards

- **Feature flag:** Disabled by default (`core.device_tracking.enabled`).
- **Consent:** Explicit consent captured per person before enrollment; stored as `consent_given` + `consent_at`.
- **Purpose:** Login anomaly / duty of care only — not productivity monitoring.
- **Data minimization:** Store only hashed fingerprints; purge telemetry after 30–90 days.
- **Outcome default:** Flag for human review; auto-revoke only with explicit opt-in.
- **Transparency:** Employees can view what telemetry is stored about them and request deletion.

### 12.4 Decision gate

A legal and labor-review sign-off is required before implementation. This is likely the longest lead item in the project.

---

## 13. Deployment & operational impact

### 13.1 What it touches

| Layer | Change |
|-------|--------|
| Next.js web | New telemetry capture script; consent UI; settings page |
| Go API | New route, new DB tables, new outbox events, rate limiting |
| Elixir orchestrator | New outbox consumer, new alerting logic, new realtime signal |
| PostgreSQL | New schema/tables; ongoing append-only write load |
| Infrastructure | Optional agent distribution; optional cert management if gRPC mTLS is ever added |

### 13.2 Resource exhaustion risks

| Resource | Risk | Mitigation |
|----------|------|------------|
| PostgreSQL write IOPS | Append-only telemetry can bloat | Retention policy; batch inserts; separate telemetry table |
| Go API CPU | Per-request validation + hashing | Rate-limit telemetry endpoint; async outbox write |
| Employee device battery | Wi-Fi scanning + continuous telemetry | Scan on-demand or at login only; configurable interval |
| Support load | False-positive session revocations | Start with "flag only"; allowlist known travel |

### 13.3 Would it affect existing systems?

Containably. It adds new tables and routes. Existing modules are unaffected if the outbox pattern is used. If gRPC mTLS is added later, it introduces a new synchronous dependency — use circuit breakers and a queue fallback.

---

## 14. Complexity and timeline estimates

### 14.1 Effort (one engineer)

| Component | Weeks |
|-----------|-------|
| Browser telemetry script | 1–1.5 |
| Native Go agent (Wi-Fi, cross-platform) | 2–3 |
| Go API ingestion + schema | 1–1.5 |
| Elixir consumer + business logic | 1–1.5 |
| Provisioning + enrollment flow | 1–1.5 |
| Privacy/consent UI + audit | 1 |
| Testing (cross-platform, false positives) | 1–2 |
| **Total** | **9–16 engineer-weeks** |

With one engineer: **~2–4 months**.  
With two engineers: **~6–10 weeks**.

### 14.2 Complexity rating

| Dimension | Rating | Notes |
|-----------|--------|-------|
| Technical | Medium-High | Cross-platform native code is hardest; spoofing is an arms race |
| Operational | High | Agent deployment, updates, telemetry retention, false positives |
| Legal / ethical | Very High | Could add weeks/months; requires consent framework |

---

## 15. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| VPN not reliably detected | High | Medium | Use confidence scoring, not binary decisions |
| False-positive session lockouts | Medium | High | Default to "flag for review"; allowlist travel |
| Employee trust / morale | High | High | Narrow scope, explicit consent, transparency |
| Telemetry table bloat | Medium | Medium | Retention policy from day one |
| Native agent deployment burden | High | Medium | Consider browser-only MVP first |
| Legal non-compliance | Medium | Very High | Legal review before code; consent framework |

---

## 16. Open decisions

| ID | Decision | Owner | Status |
|----|----------|-------|--------|
| D-T1 | Browser-only MVP vs native agent from day one | Both CEOs | Open |
| D-T2 | Auto-revoke on mismatch vs flag-only | Both CEOs | Open |
| D-T3 | Outbox pattern vs gRPC mTLS | Technical CEO | Open (outbox recommended) |
| D-L1 | Legal / labor review requirement | Commercial CEO | Open (required before code) |
| D-R1 | Telemetry retention window (days) | Technical CEO | Open (suggested 30–90) |
| D-C1 | Confidence-score threshold for alerts | Both CEOs | Open |

---

## 17. Suggested rollout (phased)

### Phase A — MVP (browser-only, flag-only)
- WebRTC + timezone + latency telemetry via Next.js script.
- Hashed fingerprint storage + outbox integration.
- Feature flagged; default off.
- "Flag for review" only — no auto-revoke.
- Consent UI + audit.

**Timeline:** 3–4 weeks.  
**Deploy after:** Phase 3 (CRM/contracts) backend is stable.

### Phase B — Full (native agent + auto-revoke option)
- Go agent with Wi-Fi BSSID scan + geolocation.
- Per-device provisioning keys + signed payloads.
- Auto-revoke as opt-in setting.
- gRPC mTLS evaluation (optional).

**Timeline:** +4–6 weeks after MVP.  
**Deploy after:** MVP has run in production for 30+ days with acceptable false-positive rate.

---

## 18. Appendix — How to revisit this document

When the team is ready to evaluate this for v2:

1. Confirm the threat model (credential sharing? account takeover? geo-enforcement? field-staff duty of care?).
2. Complete legal and labor review (D-L1).
3. Decide browser-only vs native agent (D-T1).
4. Create an ADR based on this document.
5. Add items to the Phase backlog with owners and dates.
6. Implement behind a feature flag; default off.

---

*End of document. This is a proposal for a future v2 upgrade and is not scheduled for implementation in the current release cycle.*
