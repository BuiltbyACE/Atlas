# Secrets Management Decision

**Source:** ATLAS.md §13.1 (Security baseline)  
**Decision ID:** D16  
**Owner:** Both CEOs  
**Status:** DECIDED  
**Decision date:** 2026-11-01  
**Implementation target:** Before production deployment

---

## Decision

**Atlas will use SOPS-encrypted files stored in the Git repository for secrets management.**

This decision was made after evaluating the following options:

| Option | Pros | Cons | Decision |
|---|---|---|---|
| **SOPS-encrypted files** (chosen) | Encrypted at rest, version-controlled with Git, no external dependency, works with existing CI/CD | Key management required, rotation is manual | ✅ Selected |
| Doppler | Managed service, easy rotation, audit logging | External dependency, cost, another vendor | Rejected for Phase 1 |
| Infisical | Open-source option, self-hostable | Additional infrastructure to manage | Rejected for Phase 1 |
| Cloud provider secrets manager (AWS/GCP/Azure) | Integrated with cloud, easy rotation | Cloud lock-in, additional cost | Rejected (cloud-agnostic preference) |
| `.env` files (unencrypted) | Simple | **Security violation** — secrets in plain text | Rejected |

---

## Implementation Plan

### Secret Categories

| Secret Type | Storage Location | Example |
|---|---|---|
| Database passwords | SOPS-encrypted `.env.production` | `DB_PASSWORD=atlas_production_xxx` |
| Service identity credentials | SOPS-encrypted `.env.production` | `ORCHESTRATOR_CREDENTIAL_HASH=xxx` |
| Webhook signing secrets | SOPS-encrypted `.env.production` | `WHATSAPP_WEBHOOK_SECRET=xxx` |
| Email provider API key | SOPS-encrypted `.env.production` | `SMTP_API_KEY=xxx` |
| MFA encryption keys | SOPS-encrypted `.env.production` | `MFA_ENCRYPTION_KEY=xxx` |
| TLS certificates | Certificate manager (not SOPS) | `atlas.example.com.crt` |
| Cloud provider access keys | Cloud IAM (not SOPS) | `AWS_ACCESS_KEY_ID=xxx` |

### SOPS Setup

1. **Install SOPS** — `brew install sops` or `apt-get install sops`
2. **Create age key pair** — `age-keygen -o age-key.txt`
3. **Store age private key securely** — in a password manager or hardware security module
4. **Create `.sops.yaml`** — configuration file in repo root
5. **Encrypt secrets** — `sops -e -i .env.production`
6. **Commit encrypted file** — `git add .env.production` and commit
7. **Never commit the age private key** — store in password manager only

### `.sops.yaml` Example

```yaml
creation_rules:
  - path_regex: .env.production$
    age: >-
      age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq
```

### Secret Rotation (90 days)

See `docs/runbooks/rotate-secrets.md` for the full rotation procedure.

### Access Control

| Role | Can Read Secrets | Can Rotate Secrets |
|---|---|---|
| Internal CEO | Yes | Yes |
| Commercial CEO | Yes | Yes |
| System Admin | Yes (production) | Yes (staging only) |
| Finance Officer | No (database secrets only) | No |
| Developers | No (staging only) | No |

---

## Implementation Checklist

- [ ] Install SOPS on all developer machines and CI runners
- [ ] Generate age key pair and store private key securely
- [ ] Create `.sops.yaml` configuration
- [ ] Create `.env.production` with all required secrets
- [ ] Encrypt `.env.production` with SOPS
- [ ] Commit encrypted `.env.production` to repository
- [ ] Store age private key in password manager (1Password, Bitwarden, etc.)
- [ ] Document the decryption procedure for CI/CD and developers
- [ ] Schedule first secret rotation (90 days from decision date)

---

## Related Documents

- `docs/runbooks/rotate-secrets.md` — secret rotation procedure
- `docs/policies/approval-matrix.md` — both-CEO approval for production changes
- `docs/ATLAS.md §13.1` — security baseline (secrets management)
- `docs/ATLAS.md §6.1` — identity and authentication (service identities)
