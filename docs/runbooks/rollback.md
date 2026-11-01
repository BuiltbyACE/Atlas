# Rollback Plan

**Source:** ATLAS.md §16.2 (Delivery pipeline)  
**Owner:** Both CEOs (approval for production rollback), Internal CEO (technical execution)  
**Status:** Active  
**Last reviewed:** 2026-11-01

---

## Purpose

This runbook defines how to roll back a production deployment when a release introduces critical issues. It covers database migration rollback, application rollback, and data integrity verification.

---

## When to Roll Back

Roll back when ANY of the following occur:

| Condition | Example |
|---|---|
| API availability drops below 99.5% (SLO) | `GET /api/v1/health` failing > 0.5% of requests |
| p95 read latency exceeds 300ms | Dashboard pages loading > 300ms |
| Outbox lag exceeds 10 seconds (p99) | Elixir consumer not processing events |
| Data integrity issue detected | Duplicate records, missing audit entries, corrupted JSON |
| Security vulnerability exploited | Unauthorized access, data breach |
| Critical user journey broken | Login, task creation, daily report submission all failing |

**Do NOT roll back for:** Cosmetic UI issues, minor bugs with workarounds, non-critical feature failures.

---

## Rollback Decision Matrix

| Situation | Action | Approval Required |
|---|---|---|
| New release causes SEV1/SEV2 | Immediate rollback | Incident commander (CEO or delegate) |
| New release causes SEV3 | Rollback within 4 hours | Technical lead |
| New release causes SEV4 | Fix in next release | No approval needed |
| Database migration fails | Rollback migration + application | Both CEOs |
| Security vulnerability exploited | Immediate rollback + incident response | Incident commander |

---

## Rollback Procedure

### Phase 1: Assess and Decide (0-15 minutes)

1. **Confirm the issue** — Verify the problem is caused by the new release, not infrastructure
2. **Check SLOs** — API availability, latency, outbox lag
3. **Check recent changes** — What was deployed in the last release?
4. **Make the decision** — Roll back or fix forward?
5. **Document the decision** — Create an incident record in Atlas

### Phase 2: Application Rollback (15-30 minutes)

#### Option A: Roll Back Container Images (Preferred)

```bash
# Identify the previous stable image tag
# Tags are immutable; previous release is tagged with previous version

# atlas-api
kubectl rollout undo deployment atlas-api  # Kubernetes
# or
docker-compose up -d --force-recreate atlas-api  # Docker Compose

# atlas-worker
kubectl rollout undo deployment atlas-worker
# or
docker-compose up -d --force-recreate atlas-worker

# atlas-orchestrator
kubectl rollout undo deployment atlas-orchestrator
# or
docker-compose up -d --force-recreate atlas-orchestrator

# atlas-web
kubectl rollout undo deployment atlas-web
# or
docker-compose up -d --force-recreate atlas-web
```

#### Option B: Roll Back to Specific Tag

```bash
# If you know the previous stable tag (e.g., v1.2.3)
kubectl set image deployment atlas-api atlas-api=atlas-api:v1.2.3
kubectl set image deployment atlas-web atlas-web=atlas-web:v1.2.3
```

#### Option C: Rebuild from Previous Commit (Last Resort)

```bash
# Check out the previous release tag
git checkout v1.2.3

# Rebuild and redeploy
docker build -t atlas-api:v1.2.3-fix -f infra/docker/Dockerfile.api apps/api
docker build -t atlas-web:v1.2.3-fix -f infra/docker/Dockerfile.web apps/web
```

### Phase 3: Database Rollback (If Migration Caused the Issue)

**WARNING:** Database rollbacks are destructive. Only perform if a migration caused data corruption or schema incompatibility.

```bash
# Identify the migration to roll back to
go run ./cmd/atlas-cli migrate status

# Roll back to the previous migration version
go run ./cmd/atlas-cli migrate down 1

# Verify the schema
go run ./cmd/atlas-cli migrate status
```

**Never roll back a migration that:**
- Modified data (not just schema)
- Is referenced by production data
- Has been in production for more than 24 hours

---

## Phase 3: Verify Rollback (30-45 minutes)

### Health Checks

```bash
# API health
curl -f https://api.atlas.example.com/api/v1/health

# API readiness
curl -f https://api.atlas.example.com/api/v1/health/ready

# Web application
curl -f https://atlas.example.com/

# Elixir orchestrator (check via API)
curl -f https://api.atlas.example.com/api/v1/health
```

### Critical User Journeys

Test the following end-to-end:

1. **Login** — CEO can sign in with MFA
2. **Create task** — Project lead can create and assign a task
3. **Submit daily report** — Staff member can file a daily report
4. **Approve expense** — CEO can approve an expense
5. **Create lead** — Sales rep can create a CRM lead

### Verify Data Integrity

```bash
# Check audit log integrity
psql -U atlas_api -d atlas -c "SELECT count(*) FROM audit.events;"

# Check outbox consumer is processing
psql -U atlas_orchestrator -d atlas -c "SELECT * FROM orchestrator.consumer_cursors;"

# Check no orphaned records
psql -U atlas_api -d atlas -c "SELECT count(*) FROM work.tasks WHERE status = 'error';"
```

---

## Phase 4: Communicate (Throughout)

### Internal Notification

```
Subject: [ROLLBACK] Atlas v{new_version} rolled back to v{previous_version}

Atlas production has been rolled back from v{new_version} to v{previous_version}
due to {reason}.

Incident: {incident-id}
Rollback initiated: {timestamp}
Rollback completed: {timestamp}

What users will notice:
- {description of what changed back}

What users should do:
- {any user action required}

We will post a follow-up with root cause analysis within 24 hours.
```

### Client Notification (if client-facing impact)

```
Subject: Important: Atlas service update

We experienced a service issue and have rolled back to a previous version.
Service is now restored. We apologize for any inconvenience.

We will provide a full incident report within 24 hours.

—SafariStack
```

---

## Rollback Decision Tree

```
Issue detected
    │
    ▼
Is it SEV1/SEV2?
    │
    ├─ Yes → Immediate rollback (both CEOs notified)
    │
    ├─ No → Is it SEV3?
    │       │
    │       ├─ Yes → Rollback within 4 hours (technical lead decides)
    │       │
    │       └─ No → Is it SEV4?
    │               │
    │               ├─ Yes → Fix in next release (no rollback)
    │               │
    │               └─ No → Monitor and investigate
    │
    ▼
Is data integrity affected?
    │
    ├─ Yes → Database rollback required (both CEOs approve)
    │
    └─ No → Application rollback only
    │
    ▼
Can we fix forward faster?
    │
    ├─ Yes → Fix forward (hotfix release)
    │
    └─ No → Rollback (proceed with rollback procedure)
    │
    ▼
Verify rollback succeeded
    │
    ├─ Yes → Close incident, schedule post-incident review
    │
    └─ No → Escalate to both CEOs, consider disaster recovery
```

---

## Post-Rollback Actions

1. **Close the incident** in Atlas
2. **Schedule post-incident review** — within 24 hours for SEV1/SEV2
3. **Create action items** — prevent recurrence
4. **Update deployment documentation** — if the rollback revealed gaps
5. **Notify stakeholders** — all users affected by the rollback

---

## Related Documents

- `docs/runbooks/incident-response.md` — incident severity and escalation
- `docs/policies/approval-matrix.md` — both-CEO approval for production changes
- `docs/ATLAS.md §16.2` — delivery pipeline and rollback requirements
- `docs/ATLAS.md §14` — observability and SLOs
