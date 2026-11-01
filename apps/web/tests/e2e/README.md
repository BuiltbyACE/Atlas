# E2E Tests (Playwright)

These tests verify the critical user journeys end-to-end through the browser.

## Prerequisites

Before running E2E tests, the following must be running:

1. **PostgreSQL database** on `localhost:55433` (Atlas database `atlas`)
2. **Go API server** (`atlas-api`) on `localhost:8080`
3. **Next.js dev server** (started automatically by Playwright's `webServer` config)
4. **Test user** in the database with credentials matching `TEST_USER` in `helpers/auth.ts`

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_TEST_EMAIL` | `test@example.com` | Test user email |
| `E2E_TEST_PASSWORD` | `TestPassword123!` | Test user password |
| `E2E_TEST_MFA_CODE` | `123456` | TOTP code (if MFA is required) |

## Running Tests

```bash
# Install Playwright browsers (one-time)
npx playwright install chromium

# Run all E2E tests
npm run test:e2e

# Run with UI mode
npm run test:e2e:ui

# Run a single test file
npx playwright test tests/e2e/auth.spec.ts

# Run in headed mode (see the browser)
npx playwright test tests/e2e/auth.spec.ts --headed
```

## Test Suites

| File | Coverage |
|------|----------|
| `auth.spec.ts` | Login, MFA, sign out, invalid credentials, auth guards |
| `work.spec.ts` | Create task, view task details |
| `crm-contracts.spec.ts` | Create lead, create contract |
| `finance.spec.ts` | View finance dashboard, create expense |
| `daily-report.spec.ts` | Submit daily report |

## Test Data

Tests use the credentials in `TEST_USER` from `helpers/auth.ts`. Override via environment variables:

```bash
E2E_TEST_EMAIL=ceo@example.com E2E_TEST_PASSWORD=CeoPassword123! npx playwright test
```

## Notes

- Tests use the actual UI (not API mocking) - they click buttons, fill forms, and navigate pages
- All tests start by signing in via `/login`
- Screenshots and videos are captured on failure
- Traces are captured on first retry
