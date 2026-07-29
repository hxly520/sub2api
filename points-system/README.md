# Sub2API Points System

Independent points and check-in service for the Sub2API secondary deployment.
The business contract is recorded in `PRODUCT_REQUIREMENTS_CN.md`; that file is
the source of truth for future compatibility work.

## Scope

- Points are read-only statistics derived from successful balance consumption.
- Point values and point conversion rates use hundredths as their integer unit.
- Check-in rewards are Sub2API balance credits, not spendable points.
- There is no point redemption or subscription-settlement feature.
- Reward policies are append-only and become effective no earlier than the next
  natural day in `Asia/Shanghai`.
- A policy controls its refresh minute, check-in mode, point basis, monetary
  safety caps, point tiers, and fixed or percentage reward ranges.

Only server-recorded successful usage is accepted. A browser or push endpoint
is never a usage fact source. Production reads Sub2API `usage_logs` through the
dedicated, read-only `POINTS_USAGE_DATABASE_URL` connection.

## Runtime Architecture

1. Sub2API creates a short-lived HMAC launch ticket for the current user.
2. The points service consumes the one-time nonce and creates an HttpOnly,
   SameSite=Strict session. Administrator sessions are capped at 30 minutes.
3. A four-connection, read-only PostgreSQL pool aggregates `billing_type=0`
   rows whose `actual_cost` is positive over an `Asia/Shanghai` natural day;
   user aggregates below one micro-USD after rounding are omitted as zero.
4. The policy-defined daily refresh aggregates the preceding natural day into
   immutable snapshot revisions and updates the user's read-only point total.
5. Check-in locks the daily user/platform counters, calculates a reward using
   `crypto/rand`, reserves every monetary cap, records the immutable rule
   snapshot, and enqueues a balance grant in one serializable transaction.
6. The outbox worker signs an idempotent Sub2API balance-credit request. A
   retryable or unknown result must be retried with the same transaction UUID
   until Sub2API confirms settlement. Only settled credits can enqueue a debit
   reversal; only a never-attempted pending credit can be cancelled locally.
   Attempt history is never deleted.

## Exact Units

- Money: micro-USD (`1 U = 1,000,000`, `0.01 U = 10,000`).
- Point conversion: hundredths of a point per U (`10.25 = 1025`).
- Point totals and tier bounds: hundredths of a point.
- Percentages: parts per million (`5% = 50,000 PPM`).

Percentage rewards use integer arithmetic and are rounded down to `0.01 U`.
Fixed ranges are sampled only on `0.01 U` steps. Floating point is not used for
accounting or policy calculation.

## HTTP API

Browser endpoints require a valid session. Every state-changing browser request
also requires an exact `Origin` match and `X-CSRF-Token` from `GET /api/v1/me`.
Rejected check-ins converge to one immutable audit row per user, business date,
and rejection reason, so rotating client idempotency keys cannot grow the
financial tables without bound. Client keys are claimed only when a reward is
ready to be committed; request-volume evidence remains in the reverse-proxy
access log.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/launch?ticket=...` | Exchange a one-time Sub2API launch ticket |
| GET | `/api/v1/me` | Account, check-in, snapshot, and CSRF state |
| GET | `/api/v1/ledger` | Read-only point ledger |
| POST | `/api/v1/checkins` | Check in; requires `Idempotency-Key` |
| GET | `/api/v1/balance-grants` | Current user's balance reward delivery history |
| GET/POST | `/api/v1/admin/policies` | List or append policy versions |
| POST | `/api/v1/admin/grants` | Enqueue an audited manual balance grant |
| GET | `/api/v1/admin/balance-grants` | Inspect all balance grants |
| POST | `/api/v1/admin/balance-grants/{id}/retry` | Retry a failed delivery |
| POST | `/api/v1/admin/balance-grants/{id}/reverse` | Reverse/cancel a delivery |
| POST | `/api/v1/admin/snapshots/refresh` | Idempotently refresh a past day |
| GET | `/healthz` | Database-backed health check |

The built-in user and admin workspaces are served at `/app/` and `/admin/`.
Both require a session created by `/launch`; the domain root returns 404. The
user overview includes total/yesterday points, today's reward, and the total of
settled check-in credits that have not been reversed.

## Trust Contracts

### Launch ticket

Sub2API emits
`key_id.base64url(payload).base64url(HMAC-SHA256(key_id.payload))`. The payload
contains `iss=sub2api`, `aud=points-system`, numeric `sub`, `role`, validated
`theme`/`lang`, `nonce`, `iat`, and `exp`. Configure the same decoded Base64
secret on both services.
The points service accepts a keyring to permit rotation.

### Balance credit

The outbox worker calls `/api/internal/points/credits`. Its canonical signature
is exactly:

```text
v1
KEY_ID
POST
/api/internal/points/credits
UNIX_TIMESTAMP
TRANSACTION_UUID
SHA256_HEX_OF_BODY
```

`X-Points-Signature` is lowercase hex HMAC-SHA256. Credit retries reuse the
grant UUID; reversal retries reuse a deterministic, distinct UUID. A credit
whose response was lost cannot be marked reversed locally because its remote
outcome is unknown.

### Usage collection

The points service does not accept usage facts over HTTP. Its dedicated
PostgreSQL role receives column-level `SELECT` access only to `usage_logs.id`,
`user_id`, `billing_type`, `actual_cost`, and `created_at`; see
`deploy/usage-reader.sql.example`. The application also forces read-only
transactions and limits this pool to four connections.

The scheduler defaults to `00:05` and rechecks the policy version at midnight,
so a refresh-time change takes effect on its configured natural day. It
reconciles the latest seven days by default (`POINTS_USAGE_RECONCILE_DAYS`,
maximum 31). Late rows and source corrections create signed snapshot deltas;
the snapshot, account totals, point ledger, revision, and refresh audit are
committed atomically. A correction that would make an account total negative
is retained as `needs_review` instead of committing an invalid total.

## Configuration

Copy `.env.example` and replace every placeholder. Important details:

- `POINTS_DB_PASSWORD` must match the password embedded in
  `POINTS_DATABASE_URL` for the Compose-managed points database.
- `POINTS_LAUNCH_HMAC_KEYS` contains versioned `key_id:base64_secret` entries.
- `POINTS_SUB2_CREDIT_KEY_ID` and `POINTS_SUB2_CREDIT_SECRET` select the active
  versioned, base64-decoded Sub2API balance bridge key.
- `POINTS_USAGE_DATABASE_URL` uses a separate Sub2API PostgreSQL login with
  only the grants in `deploy/usage-reader.sql.example`.
- `POINTS_PUBLIC_ORIGIN` is an origin without a path and must be HTTPS when
  secure cookies are enabled.

Generate 32-byte secrets with a CSPRNG. Never place production secrets in Git.

## Deployment

`compose.example.yml` and `deploy/nginx.conf.example` are templates. Set
`POINTS_IMAGE` to the verified GHCR digest emitted by the image workflow; the
production template intentionally has no local `build` fallback. PostgreSQL
must permit `CREATE EXTENSION btree_gist` during the first migration. If the
application role cannot create extensions, install `btree_gist` once using a
database administrator before starting the service.

The Compose template publishes the service on loopback only and joins the
existing Sub2API Docker network for the read-only database and balance bridge.
Set `SUB2API_DOCKER_NETWORK` to the exact existing network name. Set
`POINTS_TRUSTED_PROXY_CIDR` to the exact bridge gateway `/32` observed by the
points container; forwarded client IP headers are ignored from every other
source. The Nginx launch location disables access logging so one-time tickets
are not persisted in query-string logs.

The process serializes migrations with a PostgreSQL advisory lock. The first
policy is disabled. Create a complete policy through the admin API; its effective
date must be tomorrow or later.

Serializable write transactions retry PostgreSQL `40001` serialization failures
and `40P01` deadlocks with context-aware jittered backoff, up to eight total
attempts. Transaction callbacks must reset captured mutable result state because
the callback can run more than once.

The Sub2API deployment must configure the matching public URL, launch secret,
credit secret, launch TTL, and clock-skew tolerance. The read-only usage login
must be reachable from the points container; no usage producer is deployed.

## Development

```bash
go mod tidy
gofmt -w .
go test ./...
go vet ./...
```

Database integration tests require PostgreSQL with `btree_gist`:

```bash
POINTS_TEST_DATABASE_URL='postgres://points_test:points_test@127.0.0.1:5432/points_test?sslmode=disable' \
  go test -tags=integration ./internal/store -count=1
```

The tagged suite applies the embedded migrations in isolated schemas and
checks migration idempotency, concurrent daily/user/platform check-in caps,
settled unreversed check-in totals, and rollback of snapshot-not-ready
idempotency state. Snapshot revision concurrency, one-time launch-ticket
storage, outbox lease recovery, balance retry/reversal idempotency, and the
append-only triggers remain candidates for broader integration coverage.
