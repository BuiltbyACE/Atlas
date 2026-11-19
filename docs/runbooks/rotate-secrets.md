# Rotate Secrets Runbook

**Source:** ATLAS.md §13.1 (Security baseline), §16.4 (Hosting)  
**Owner:** Internal CEO (technical lead), Both CEOs (approval for production changes)  
**Status:** Active  
**Last reviewed:** 2026-11-01  
**Rotation schedule:** Every 90 days for service credentials; immediately on suspected compromise

---

## Purpose

This runbook defines how to rotate secrets and credentials across the Atlas platform. Regular rotation limits the blast radius of compromised credentials and is required by the security baseline (§13.1).

---

## Secrets Inventory

| Secret Type | Location | Rotation Frequency | Rotated By |
|---|---|---|---|
| Database passwords (`atlas_api`, `atlas_worker`, etc.) | Secrets manager | 90 days | Internal CEO |
| Service identity credentials (`svc:orchestrator`, etc.) | Secrets manager | 90 days | Internal CEO |
| API keys (WhatsApp, GitHub, M-Pesa, email provider) | Secrets manager | 90 days | Integration owner |
| Encryption keys (MFA secrets, salary data) | Secrets manager / application-level | 180 days | Internal CEO |
| Session signing keys | Application config | 90 days | Internal CEO |
| Webhook signing secrets | Secrets manager | 90 days | Integration owner |
| TLS certificates | Certificate manager / Caddy | Per certificate expiry | Internal CEO |
| Cloud provider access keys | Cloud IAM | 90 days | Infrastructure owner |

---

## Secrets Management Decision [DECIDE]

Per ATLAS.md §13.1: *"Secrets in a secrets manager [DECIDE: e.g. Doppler, Infisical, cloud provider's manager, or SOPS-encrypted files for small scale]."*

**Current state:** [Enter your decision here — e.g., "SOPS-encrypted files in the repo" or "Doppler" or "AWS Secrets Manager"]

**Required action:** If not yet decided, both CEOs must decide and document the choice before the first rotation.

---

## Rotation Procedure (General)

### Pre-Rotation Checklist

- [ ] Both CEOs have approved the rotation (production secrets)
- [ ] Incident NOT in progress (do not rotate during active SEV1/SEV2)
- [ ] Staging environment available for testing
- [ ] Rollback plan prepared (old secret value recorded securely)
- [ ] All applications using the secret identified
- [ ] Downtime window scheduled (if secret change requires app restart)

### Step 1: Generate New Secret

```bash
# Example: Generate a new database password
openssl rand -base64 32

# Example: Generate a new signing key
openssl rand -hex 64
```

### Step 2: Store New Secret in Secrets Manager

Store the new secret with a version label (e.g., `db_password_v2`, `db_password_v3`).

### Step 3: Update Applications

Update each application that uses the secret:

**For Docker Compose:**
```bash
# Update .env or secrets file
docker-compose up -d --force-recreate atlas-api
```

**For Kubernetes:**
```bash
kubectl delete secret atlas-db-credentials
kubectl create secret generic atlas-db-credentials --from-literal=PASSWORD='new-value'
kubectl rollout restart deployment atlas-api
```

### Step 4: Verify Each Application

| Application | Verification |
|---|---|
| atlas-api | `GET /api/v1/health` returns healthy |
| atlas-worker | Worker processes jobs without auth errors |
| atlas-orchestrator | Can connect to database and read outbox |
| atlas-web | Login works, session creation succeeds |

### Step 5: Revoke Old Secret

After all applications are verified working with the new secret:

1. Mark old secret as revoked in secrets manager
2. Do NOT delete immediately — keep for 7 days as emergency rollback
3. Delete after 7 days with no issues

### Step 6: Audit

Write an audit entry recording:
- Which secret was rotated
- When
- By whom
- Old secret version, new secret version
- Applications updated

---

## Secret-Specific Procedures

### Database Passwords (`atlas_api`, `atlas_worker`, `atlas_orchestrator`)

**Risk:** High — all applications depend on database access.

**Procedure:**
1. Generate new password in secrets manager
2. Update database user password via SQL:
   ```sql
   ALTER ROLE atlas_api WITH PASSWORD 'new-password';
   ```
3. Update application secrets
4. Restart applications one at a time (zero downtime)
5. Verify each application can connect
6. Revoke old password after 7 days

### Service Identity Credentials (`svc:orchestrator`)

**Risk:** High — orchestrator commands Go's internal API.

**Procedure:**
1. Generate new credential hash
2. Update `core.service_identities` table with new `credential_hash`
3. Update orchestrator secrets
4. Restart orchestrator
5. Verify outbox consumer is processing events

### Webhook Signing Secrets (WhatsApp, GitHub, M-Pesa)

**Risk:** Medium — forged webhooks could create fake records.

**Procedure:**
1. Generate new signing secret
2. Update provider dashboard (WhatsApp Business, GitHub, M-Pesa Daraja)
3. Update Atlas secrets manager
4. Restart atlas-api
5. Test with a test webhook delivery

### Encryption Keys (MFA Secrets, Salary Data)

**Risk:** Critical — loss means permanent data loss.

**Procedure:**
1. **Never delete old keys** — they are needed to decrypt existing data
2. Add new key to keyring
3. Re-encrypt existing data with new key (background job)
4. Keep old keys indefinitely for decryption of historical data
5. Document key IDs in documentation

### TLS Certificates

**Risk:** Medium — expired certificates cause outages.

**Procedure:**
1. Monitor certificate expiry (alert at 30, 14, 7 days)
2. Obtain new certificate from CA
3. Update Caddy or load balancer configuration
4. Reload service (zero downtime with Caddy)
5. Verify HTTPS works from external client

---

## Emergency Rotation (Compromised Credential)

If a credential is suspected compromised:

1. **Declare SEV1 incident** — see incident-response.md
2. **Immediate rotation** — skip approval wait if CEO unreachable (document retroactively)
3. **Revoke all sessions** — force all users to re-authenticate
4. **Rotate the compromised credential** — follow procedure above with expedited timeline
5. **Audit** — review audit logs for unauthorized access using the compromised credential
6. **Notify** — if personal data may have been accessed, follow data breach procedure (Kenya DPA 2019, 72-hour notification)

---

## Rotation Schedule

| Secret | Frequency | Next Rotation | Owner |
|---|---|---|---|
| Database passwords | 90 days | [Enter date] | Internal CEO |
| Service identity credentials | 90 days | [Enter date] | Internal CEO |
| WhatsApp webhook secret | 90 days | [Enter date] | Commercial CEO |
| GitHub webhook secret | 90 days | [Enter date] | Integration owner |
| M-Pesa webhook secret | 90 days | [Enter date] | Finance lead |
| Email provider API key | 90 days | [Enter date] | Internal CEO |
| TLS certificates | Per expiry | [Enter date] | Internal CEO |
| Encryption keys | 180 days | [Enter date] | Internal CEO |

---

## Audit Trail

Every rotation writes to:
- `audit.events` — action: `core.secret.rotated`, entity: secret name, before/after: version IDs
- `core.service_identities` — `rotated_at` timestamp updated
- Rotation log in secrets manager (who, when, from IP)

---

## Related Documents

- `docs/policies/approval-matrix.md` — both-CEO approval for production changes
- `docs/policies/data-retention.md` — backup and encryption key retention
- `docs/ATLAS.md §13.1` — security baseline (secrets management)
- `docs/ATLAS.md §6.1` — identity and authentication (service identities, sessions)
- `docs/ATLAS.md §13.3` — threat model (compromised integration credential)
