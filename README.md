# Atlas

### The Operating System for SafariStack Solutions

**One platform. One source of truth. Better decisions.**

Atlas is a company operations and management platform being developed to streamline how SafariStack Solutions manages its people, clients, projects, finances, internal systems, and business decisions.

As a company grows, its operations can become fragmented across spreadsheets, conversations, project management tools, financial records, and disconnected applications. Important information becomes harder to track, accountability becomes less clear, and leadership spends more time coordinating work than improving the business.

Atlas aims to bring these operations into one integrated platform, helping the company move from fragmented processes to structured, measurable, and accountable execution.

---

## The Vision

Atlas is being built to become the central operating system for SafariStack Solutions: a unified environment where management can understand what is happening across the company, teams can coordinate their responsibilities, and decisions can be made using reliable operational data.

The goal is not simply to build another management dashboard. It is to connect the company's workflows, records, responsibilities, and outcomes into a coherent system.

Atlas is designed around a simple principle:

> Every important business activity should have a clear owner, an observable status, an accountable process, and a measurable outcome.

## What Atlas Will Streamline

### 1. Customer Relationship Management

Centralize customer and prospect information, enquiries, interactions, follow-ups, sales opportunities, proposals, and contracts.

Atlas aims to help the team:
- Keep track of every lead and its next action.
- Reduce missed follow-ups and lost opportunities.
- Understand the sales pipeline and the progress of prospective deals.
- Connect sales activities to project delivery and client handover.
- Build a clearer picture of customer relationships over time.

### 2. Project and Work Management

Bring projects, tasks, milestones, dependencies, blockers, deadlines, and delivery evidence into one operational workspace.

Planned capabilities include:
- Clear task ownership and accountability.
- Project progress and milestone tracking.
- Visibility into dependencies, delays, and blockers.
- Structured daily reporting and management reviews.
- Delivery handover processes and evidence of completed work.
- Escalations when important commitments are at risk.

The objective is to make progress and delivery risks visible before they become larger problems.

### 3. Financial Management and Control

Connect operational activity to financial performance through structured financial records and approval processes.

Planned capabilities include:
- Budgets and expense management.
- Invoice and payment tracking.
- Receivables and outstanding balances.
- Expense approval workflows.
- Payment reconciliation and financial reporting.
- Project-level costs, margins, and profitability.
- Visibility into recurring expenses and financial commitments.

Atlas is intended to improve financial visibility and control. It is not a substitute for statutory accounting or regulatory compliance requirements without the appropriate implementation and verification.

### 4. People and Organizational Management

Provide a structured view of the company's departments, team members, responsibilities, permissions, and internal processes.

Planned capabilities include:
- Staff and departmental records.
- Role-based access and permissions.
- Invitations, account activation, and secure authentication.
- Leave and absence workflows.
- Daily operational reports.
- Approval responsibilities and accountability records.

The design emphasizes appropriate access, clear responsibilities, and responsible handling of employee information.

### 5. Systems and Technical Operations

Create a central registry for the systems and services the company operates or maintains.

Planned capabilities include:
- System and service inventories.
- Health checks and availability monitoring.
- Incident tracking and escalation.
- Service ownership and operational history.
- Post-incident reviews.
- Selected indicators from integrated systems.

This will help technical leadership understand the state of company-operated systems without relying entirely on manual status updates.

### 6. Executive Intelligence and Decision Support

Bring operational information together to support better management decisions.

Atlas is intended to provide:
- Executive dashboards and management summaries.
- Delivery, sales, financial, and operational scorecards.
- Overdue commitments and emerging risks.
- Decisions and approvals requiring attention.
- Performance indicators across departments and projects.
- A foundation for future AI-assisted reporting and business intelligence.

The focus is on actionable information, not dashboards for their own sake.

---

## How Atlas Fits Together

Atlas is being designed as a modular platform rather than a collection of disconnected applications.

Its core modules will share common identity, permissions, auditability, configuration, and integration foundations. This allows business activities to connect across the company.

For example, a sales opportunity can progress into a proposal, an approved contract, a project handover, tracked delivery, and ultimately financial reporting. The intention is to reduce duplicated data entry and give management a connected view of the entire process.

### Architectural approach

| Component | Responsibility |
|---|---|
| Next.js and TypeScript | Web interface, management dashboards, and user workflows |
| Go | Core business API, business rules, permissions, and data operations |
| Elixir and Phoenix | Event-driven coordination, workflows, notifications, and real-time updates |
| PostgreSQL | Authoritative storage for business records |
| OpenAPI and event schemas | Explicit contracts between components |
| Docker Compose | Reproducible local infrastructure and development services |

The architecture follows a modular-monolith approach for the core business backend, with a dedicated orchestration layer for asynchronous coordination and real-time functionality.

The objective is to maintain clear module boundaries, dependable data handling, and room to evolve without introducing unnecessary infrastructure complexity.

---

## Engineering Principles

Atlas is being developed around the following principles.

**Reliable data**

Business records should have clear ownership, validation, controlled transitions, and appropriate audit history.

**Security by design**

Authentication, authorization, least-privilege access, protected sessions, and secure integration boundaries are foundational requirements.

**Accountability without unnecessary surveillance**

The platform should make responsibilities, commitments, and delivery outcomes visible while respecting employee privacy and limiting access to sensitive information.

**Automation with control**

Repetitive coordination and notification work should be automated where useful, with explicit approval requirements for sensitive business operations.

**Modular architecture**

Each domain should have a clear responsibility and interact through defined contracts rather than tightly coupled implementations.

**Operational simplicity**

Infrastructure and dependencies should be introduced when they solve a demonstrated problem, not merely because they are available.

**Measurable business value**

Features should be evaluated against tangible outcomes such as fewer missed follow-ups, clearer project status, faster approvals, improved financial visibility, and more reliable reporting.

---

## Development Status

Atlas is an actively developed internal platform. Its architecture and foundational capabilities are being established before the main business modules are introduced.

The development roadmap is organized into stages:

1. **Platform foundation** — database structure, identity, access control, auditing, API standards, and development infrastructure.
2. **Work orchestration** — projects, tasks, milestones, reporting, blockers, and delivery workflows.
3. **CRM and sales** — customer relationships, leads, follow-ups, proposals, contracts, and handovers.
4. **Financial control** — budgets, expenses, invoices, payments, reconciliation, and profitability.
5. **Systems operations** — service inventory, health monitoring, incidents, and selected integrations.
6. **Intelligence and automation** — executive reporting, management scorecards, risk visibility, and decision support.

These are planned capabilities, not a claim that every module is already implemented or production-ready. Features will be introduced and validated incrementally.

### The sign-in experience

The foundation is in place, and the first user-facing slice is built: secure sign-in. Staff sign in with an email address or a phone number; privileged roles confirm with a one-time code from an authenticator app, and first-time users enrol with a QR code and are given one-time recovery codes. It is the page every other part of Atlas will sit behind.

| Desktop | Phone |
|---|---|
| ![Atlas sign-in on desktop](<images/Atlas — Sign in.png>) | ![Atlas sign-in on a phone](<images/Atlas — Sign in (phone).png>) |

## Long-Term Direction

Atlas is initially being developed to address the operational needs of SafariStack Solutions.

As the platform matures, it may provide a foundation for reusable business operating capabilities, deeper integrations, and more advanced automation. Any broader productization will depend on the maturity of the system, business requirements, security, and commercial feasibility.

The immediate priority is to build a dependable system that improves how the company itself operates.

---

## Project Information

**Project:** Atlas  
**Organization:** SafariStack Solutions  
**Category:** Internal business operations platform  
**Architecture:** Modular core backend with event-driven orchestration  
**Development status:** In active development

Atlas is a proprietary project of SafariStack Solutions. Its source code, internal business processes, and implementation details are not automatically licensed for reuse or redistribution. Third-party dependencies remain subject to their respective licenses.

---

*Atlas is being built to help turn business activity into coordinated execution, reliable information, and better decisions.*
