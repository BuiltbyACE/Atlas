# Database Restore Runbook

**Source:** ATLAS.md §10.4 (Data lifecycle), §14 (Observability and reliability), §16.2 (Delivery pipeline)  
**Owner:** Internal CEO (technical lead), Both CEOs (approval for production changes)  
**Status:** Active  
**Last reviewed:** 2026-11-01  
**Last restore drill:** [Enter date] — record as task in Atlas

---

## Purpose

This runbook defines how to restore the Atlas PostgreSQL database from backups. It covers point-in-time recovery (PITR), full restore, and restore drills. The goal is to minimize data loss and downtime while maintaining audit integrity.

---

## Backup Strategy (Current)

| Backup Type | Frequency | Retention | Location |
|---|---|---|---|
| Full backup | Nightly | 30 days | Off-site, encrypted |
| WAL archiving | Continuous | 7 days | Point-in-time recovery |
| Restore drill | Monthly | N/A (drill record only) | Recorded as Atlas task with evidence |

---

## Prerequisites

Before starting any restore:

- [ ] **Both CEOs have approved** the restore (production only)
- [ ] **Incident declared** if this is incident-driven (see incident-response.md)
- [ ] **Audit entry written** — restore is an audited operation
- [ ] **Backup verified** — backup files exist and are readable
- [ ] **Staging tested** — restore procedure tested on staging first (production restores only)
- [ ] **Rollback plan prepared** — if restore fails, what is the fallback?

---

## Scenario A: Full Restore from Nightly Backup

Use when: Database corruption, catastrophic failure, or major data integrity issue.

### Step 1: Stop the Applications

```bash
# Stop all Atlas services to prevent writes during restore
kubectl scale deployment atlas-api --replicas=0   # or docker-compose stop
kubectl scale deployment atlas-worker --replicas=0
kubectl scale deployment atlas-orchestrator --replicas=0
kubectl scale deployment atlas-web --replicas=0
```

Or with Docker Compose:
```bash
cd infra/compose
docker-compose stop atlas-api atlas-worker atlas-orchestrator atlas-web
```

### Step 2: Restore the Database

```bash
# Identify the backup file to restore from
ls -la /backups/postgres/full/

# Stop PostgreSQL temporarily (if managed service, use provider console)
# Restore from backup
pg_restore -U atlas_migrator -d atlas --clean --if-exists \
  /backups/postgres/full/atlas-full-YYYYMMDD.dump

# Or if using cloud provider managed restore:
# Follow provider-specific point-in-time restore procedure
```

### Step 3: Re-apply WAL (Point-in-Time Recovery)

If you need to recover to a specific point in time (e.g., before a bad migration):

```bash
# Use pg_basebackup + WAL replay
# Or use your managed database provider's PITR feature
# Target: latest consistent state before the incident
```

### Step 4: Verify the Restore

```bash
# Connect and verify key tables
psql -U atlas_migrator -d atlas -c "SELECT count(*) FROM core.users;"
psql -U atlas_migrator -d atlas -c "SELECT count(*) FROM audit.events;"
psql -U atlas_migrator -d atlas -c "SELECT max(seq) FROM outbox.events;"

# Verify schema integrity
go run ./cmd/atlas-cli migrate status
```

### Step 5: Restart Applications

```bash
kubectl scale deployment atlas-api --replicas=3
kubectl scale deployment atlas-worker --replicas=2
kubectl scale deployment atlas-orchestrator --replicas=2
kubectl scale deployment atlas-web --replicas=3
```

Or with Docker Compose:
```bash
docker-compose start atlas-api atlas-worker atlas-orchestrator atlas-web
```

### Step 6: Post-Restore Verification

- [ ] Health endpoint responds: `GET /api/v1/health`
- [ ] Readiness endpoint responds: `GET /api/v1/health/ready`
- [ ] Login works for at least one CEO account
- [ ] Outbox consumer is processing events (check consumer cursor)
- [ ] Audit log shows the restore operation

---

## Scenario B: Point-in-Time Recovery (PITR)

Use when: Data corruption discovered after the fact, bad migration deployed, or need to recover to a specific timestamp.

### Prerequisites

- Continuous WAL archiving must be enabled (it is, per §10.4)
- You know the target timestamp (e.g., "before the bad migration at 14:30 UTC")

### Procedure

1. **Restore the latest full backup** (steps 1-2 from Scenario A)
2. **Configure recovery target time** in `postgresql.auto.conf`:
   ```
   recovery_target_time = '2026-11-01 14:29:00+00'
   recovery_target_action = 'promote'
   ```
3. **Start PostgreSQL** — it will replay WAL up to the target time
4. **Verify** — check that the bad change is not present
5. **Promote** — make the recovered database the primary
6. **Restart applications** — same as Scenario A step 5

---

## Scenario C: Selective Table Restore

Use when: Only one table is corrupted (e.g., `audit.events` or `core.approvals`).

### Procedure

1. **Restore full backup to a staging database** (not production)
2. **Export the clean table:**
   ```bash
   pg_dump -U atlas_migrator -d atlas_staging -t audit.events > audit_events.sql
   ```
3. **Import into production:**
   ```bash
   psql -U atlas_migrator -d atlas -c "TRUNCATE audit.events CASCADE;"
   psql -U atlas_migrator -d atlas -f audit_events.sql
   ```
4. **Verify** — check counts and latest entries match expectations
5. **Audit** — write audit entry documenting the selective restore

---

## Restore Drill (Monthly)

Per ATLAS.md §10.4: **"restore drill monthly and recorded in Atlas as a task with evidence."**

### Monthly Drill Procedure

1. **Schedule** — Create an Atlas task: "Monthly restore drill" assigned to technical lead
2. **Execute** — Perform a full restore to a staging environment
3. **Verify** — All health checks pass, sample data intact, audit trail intact
4. **Record** — Create an Atlas task completion with evidence:
   - Date of drill
   - Person who performed it
   - Restore time (how long it took)
   - Verification results
   - Any issues found
5. **Retain** — Keep drill records for audit

### Drill Success Criteria

- [ ] Restore completes within [CONFIG] target time (e.g., 4 hours for full restore)
- [ ] All critical tables have expected row counts
- [ ] Audit events table is intact and append-only
- [ ] Health endpoints respond after application restart
- [ ] Outbox consumer resumes processing without manual intervention

---

## Emergency Contacts

| Role | Name | Contact | Escalation |
|---|---|---|---|
| Incident Commander | Both CEOs | — | Immediate |
| Technical Lead | Internal CEO | — | L2 |
| Database Administrator | [Name] | — | L1 |
| Cloud Provider Support | [Provider] | — | Infrastructure issues |
| Legal Counsel | [Name] | — | Data breach notification |

---

## Related Documents

- `docs/policies/data-retention.md` — retention schedule, backup requirements
- `docs/policies/approval-matrix.md` — both-CEO approval for production changes
- `docs/ATLAS.md §10.4` — data lifecycle and backup requirements
- `docs/ATLAS.md §14` — observability, SLOs (API availability 99.5%)
- `docs/ATLAS.md §16.2` — delivery pipeline, rollback plans per release
