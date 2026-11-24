# Staging Environment

**Source:** ATLAS.md §16.1 (Environments)  
**Owner:** Internal CEO (technical lead)  
**Status:** To be provisioned  
**Target:** Before production deployment

---

## Purpose

The staging environment is a pre-production verification environment used for CEO acceptance testing, smoke tests, and validation of releases before production deployment. It mirrors production as closely as possible but uses synthetic or anonymized data.

---

## Environment Specification

| Component | Production | Staging |
|---|---|---|
| **Purpose** | Live company operations | Pre-production verification |
| **Data** | Real | Synthetic/anonymized |
| **Integrations** | Live (WhatsApp, M-Pesa, etc.) | Sandbox accounts |
| **Database** | Managed PostgreSQL with PITR | Managed PostgreSQL |
| **Object Storage** | S3-compatible production bucket | S3-compatible staging bucket |
| **Email** | Production SMTP | Sandbox/Mailpit |
| **WhatsApp** | WhatsApp Business API | WhatsApp sandbox/test number |
| **Domain** | `atlas.example.com` | `staging.atlas.example.com` |
| **TLS Certificate** | Production CA certificate | Staging/self-signed certificate |

---

## Provisioning Checklist

### Infrastructure

- [ ] Provision VM or managed container platform for staging
- [ ] Provision managed PostgreSQL with PITR
- [ ] Provision S3-compatible object storage bucket
- [ ] Configure Caddy reverse proxy for `staging.atlas.example.com`
- [ ] Obtain TLS certificate (Let's Encrypt staging or self-signed)
- [ ] Configure DNS for `staging.atlas.example.com`

### Applications

- [ ] Deploy atlas-api to staging
- [ ] Deploy atlas-worker to staging
- [ ] Deploy atlas-orchestrator to staging
- [ ] Deploy atlas-web to staging
- [ ] Configure environment variables for staging
- [ ] Run database migrations on staging database
- [ ] Seed synthetic data (see below)

### Integrations (Sandbox)

- [ ] WhatsApp Business sandbox number configured
- [ ] M-Pesa sandbox credentials configured
- [ ] Email sandbox/SMTP configured (or Mailpit)
- [ ] GitHub webhook pointing to staging
- [ ] External uptime monitor pointing to staging

### Data

- [ ] Create synthetic users (CEOs, finance officer, sales rep, staff)
- [ ] Create synthetic organizations, leads, opportunities
- [ ] Create synthetic contracts, invoices, payments
- [ ] Create synthetic tasks, projects, milestones
- [ ] Create synthetic systems, checks, incidents
- [ ] Do NOT copy production data to staging

---

## Staging vs Production Differences

| Feature | Production | Staging |
|---|---|---|
| Real customer data | Yes | No (synthetic only) |
| Real WhatsApp messages | Yes | Sandbox only |
| Real payments | Yes | Sandbox only |
| Real email delivery | Yes | Sandbox/Mailpit only |
| Uptime monitoring | Yes | Optional |
| Access restricted to staff | Yes | Yes (staff only) |
| Deploy on every merge to main | No | Yes (automatic) |
| Deploy on tag | Yes | No |

---

## Promotion Process (Staging → Production)

```
1. Feature branch merged to main
   ↓
2. CI runs all tests (Go, Elixir, Web, E2E, Security)
   ↓
3. Docker images built and scanned
   ↓
4. Deploy to staging automatically
   ↓
5. Smoke tests run against staging
   ↓
6. CEO acceptance testing on staging
   ↓
7. Both CEOs approve production deployment
   ↓
8. Tag release (e.g., v1.2.3)
   ↓
9. Deploy to production
   ↓
10. Post-deploy verification
```

### CEO Acceptance Criteria (Staging)

Before approving production deployment, both CEOs must verify on staging:

- [ ] Login works with MFA
- [ ] Dashboard loads and shows correct data
- [ ] Can create a task and assign it
- [ ] Can submit a daily report
- [ ] Can create an expense and submit for approval
- [ ] Can create a CRM lead and convert to opportunity
- [ ] Can draft a contract from an opportunity
- [ ] Approvals workflow works end-to-end

---

## Data Seeding Script

A seeding script populates staging with synthetic data:

```bash
# Run on staging database
go run ./cmd/atlas-cli seed --environment=staging
```

The seeder creates:
- 2 CEO accounts (internal and commercial)
- 1 finance officer account
- 3 staff accounts
- 5 departments
- 10 organizations
- 25 leads (mixed statuses)
- 5 opportunities
- 3 contracts
- 20 tasks (mixed statuses)
- 15 daily reports (past 2 weeks)
- 5 leave requests (mixed statuses)
- 10 expenses (mixed statuses)
- 5 systems (mixed tiers)
- 10 checks (mixed statuses)
- 3 incidents (mixed severity)

---

## Related Documents

- `docs/policies/secrets-management.md` — secrets management decision
- `docs/runbooks/rollback.md` — rollback procedure
- `docs/runbooks/incident-response.md` — incident response
- `docs/ATLAS.md §16.1` — environments
- `docs/ATLAS.md §16.2` — delivery pipeline
