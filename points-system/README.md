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

## Production Baseline (2026-07-30)

- Sub2API runs `ghcr.io/hxly520/sub2api:0.1.168-339422728b2c`, OCI revision
  `339422728b2ceb87b4a81bb08229d370c4ca589d`; the container is healthy with
  restart count zero.
- The points service runs
  `ghcr.io/hxly520/sub2api-points:0.1.168-c0fe91506bca`, OCI revision
  `c0fe91506bca60dfcc96b6d868b48b30d2ca86f0`, and is also healthy with restart
  count zero. Its GHCR registry digest is
  `sha256:8a9b7f51ce454450fc797aeeb7bfea008351cdba354327ae6cf40d3ddbdb4148`.
  The production host imported the locally verified Docker archive because it
  has no private-package registry login; the archive SHA256 is
  `d8bed76bd257e4ecb3e72dddb5e26c11147274738a3e7e316e2015c85568ef7d`.
- Both services use the same PostgreSQL 17.8 `sub2api` database. The isolated
  `points` schema contains 19 tables and two points migrations. `points_app`
  has an eight-connection limit; the column-restricted, read-only
  `points_usage_reader` has a four-connection limit.
- Sub2API has 250 applied public migrations. Private migrations
  `192_media_balance_hold_reconciliation_index_notx.sql` and
  `193_points_balance_credit_ledger.sql` are applied; points migrations remain
  separate in `points.points_schema_migrations`.
- Sub2API public setting `points_system_enabled` and policy version 1 both
  remain disabled. The ordinary user menu is hidden, a user launch ticket is
  rejected with `403 points_disabled`, and the administrator-only launch,
  Chinese workspace, read-only policy check, CSRF logout, and post-logout 401
  flow have been verified without changing any financial table.

## Runtime Architecture

1. Sub2API creates a short-lived HMAC launch ticket for the current user.
2. The points service consumes the one-time nonce and creates an HttpOnly,
   SameSite=Strict session. Administrator sessions are capped at 30 minutes.
3. Points tables live in an isolated schema inside the existing Sub2API
   PostgreSQL database. The write pool is capped at eight connections and
   refuses to start if that schema is missing or resolves to `public`.
4. A separate four-connection, read-only PostgreSQL login aggregates `billing_type=0`
   rows whose `actual_cost` is positive over an `Asia/Shanghai` natural day;
   user aggregates below one micro-USD after rounding are omitted as zero.
5. The policy-defined daily refresh aggregates the preceding natural day into
   immutable snapshot revisions and updates the user's read-only point total.
6. Check-in locks the daily user/platform counters, calculates a reward using
   `crypto/rand`, reserves every monetary cap, records the immutable rule
   snapshot, and enqueues a balance grant in one serializable transaction.
7. The outbox worker signs an idempotent Sub2API balance-credit request. A
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
| GET | `/launch?ticket=...&ui_mode=embedded` | Exchange a one-time Sub2API launch ticket; the optional mode selects the embedded presentation only |
| GET | `/api/v1/me` | Account, check-in, snapshot, and CSRF state |
| GET | `/api/v1/daily-points?days=7|30|90` | User-scoped chronological daily points and successful spend |
| GET | `/api/v1/ledger` | Read-only point ledger |
| POST | `/api/v1/checkins` | Check in; requires `Idempotency-Key` |
| GET | `/api/v1/balance-grants` | Current user's balance reward delivery history |
| GET | `/api/v1/admin/users/points` | Paginated per-user total and previous-day consumption points |
| GET/POST | `/api/v1/admin/policies` | List or append policy versions |
| GET | `/api/v1/admin/balance-grants` | Inspect check-in reward deliveries only |
| POST | `/api/v1/admin/balance-grants/{id}/retry` | Retry a failed check-in reward delivery |
| POST | `/api/v1/admin/balance-grants/{id}/reverse` | Audit and reverse/cancel a check-in reward delivery |
| GET | `/healthz` | Database-backed health check |

The built-in user and administrator workspaces are separate Chinese pages at
`/app/` and `/admin/`, with separate scripts. Both require a session created by
`/launch`; the domain root returns 404. A user session cannot fetch the admin
page, admin script, or any `/api/v1/admin/*` endpoint. The user dashboard shows
total/yesterday points, successful yesterday spend, today's and settled
unreversed check-in credits, a 7/30/90-day points trend, and personal records;
it contains no policy, manual grant, snapshot, retry, or reversal controls.
User and administrator routes are role-exact in both directions. An
administrator session cannot invoke the user account, ledger, check-in, or
grant APIs, and the shared logout route is the only role-neutral write route.
Session reads used by pages, assets, and APIs do not update `last_seen_at`, so a
single page load does not create parallel write locks in the shared database.

The administrator workspace has no independent manual balance-grant control;
Sub2API remains the only administrator surface for direct balance changes. It
also has no manual snapshot-refresh control. Daily snapshots are internal,
idempotent accounting records produced automatically at the configured refresh
time, while the one-time full historical baseline is executed only by the
audited `points-history-backfill` operations workflow below. The administrator
user directory reads only `users.id` and `users.deleted_at` from Sub2API, then
exposes points and successful-spend totals plus the prior natural day's
settlement state. Zero-spend users remain visible with zero totals. The delivery
workspace contains check-in reward records only; legacy manual-grant rows remain
in the database for audit but are not listed or mutable through the points HTTP
API.

Sub2API opens both workspaces inside its authenticated right-hand content area.
It appends the exact allowlisted `ui_mode=embedded` value to `/launch`; the
points service preserves that value on the redirect to `/app/` or `/admin/`.
This mode changes presentation only: it does not select a role, relax ticket or
session checks, or expose administrator APIs. The embedded user view hides its
standalone top bar so the Sub2API sidebar and uploaded logo remain authoritative.
The embedded administrator view likewise removes its duplicate brand and logout
controls, while retaining a compact horizontal navigation bar for points-only
administrative functions. Direct standalone access keeps the original layouts.
The parent waits for a role- and Origin-validated `sub2api:points-ready`
message from the iframe instead of treating an HTTP error document as a loaded
workspace. The iframe receives scripts, forms, and same-origin access only; it
does not receive popup, top-navigation, or clipboard permissions.

Sub2API's Points tab in System Settings and the administrator route
`/admin/settings/points` are always visible to authenticated administrators, including while
`points_system.enabled=false`, so an administrator can inspect bridge status
and use step-up authentication to launch the policy workspace. The ordinary
user menu and `/points` route remain hidden unless the enabled switch is on.
The points service independently rejects user launch tickets, user pages,
user assets, and user APIs when the effective points policy is missing,
disabled, incomplete, or is the initial activation policy whose confirmed
history baseline has not succeeded. This closes stale-session and direct-URL paths while
leaving the authenticated administrator workspace available for pre-release
debugging.
The Sub2API status endpoint returns only non-secret state such as
enabled/configured/active, public URL, key IDs, TTL, and clock skew; it never
returns launch or credit key material.

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

Sub2API protects balance-cache database refill with a per-user Redis generation.
Invalidation and deductions atomically advance the generation, and an older
database read may refill the cache only if that generation is unchanged. If a
credit commits to PostgreSQL but balance-cache synchronization fails, Sub2API
returns retryable HTTP 503; the outbox retries the original UUID and must not
mark the grant settled early.

The internal credit route uses a Redis-backed, fail-closed application limit of
120 requests per minute. Public Nginx configuration must return exact 404 for
`POST` and `OPTIONS` on `/api/internal/points/credits`; only the points container
may call it over the Docker network. HMAC is not the sole network boundary.

### Usage collection

The points service does not accept usage facts over HTTP. Its dedicated
PostgreSQL role receives column-level `SELECT` access only to `usage_logs.id`,
`user_id`, `billing_type`, `actual_cost`, and `created_at`; see
`deploy/usage-reader.sql.example`. The application also forces read-only
transactions and limits this pool to four connections.

The scheduler defaults to `00:05` and rechecks the policy version at midnight,
so a refresh-time change takes effect on its configured natural day. Automatic
refreshes skip dates whose versioned policy is disabled, so starting the service
does not scan Sub2API usage data before points are active. A disabled date still
gets one idempotent zero-value readiness marker, allowing the first enabled day
to use an all-user zero-point tier without reporting that yesterday is pending.
It reconciles the latest seven days by default (`POINTS_USAGE_RECONCILE_DAYS`,
maximum 31). Use a one-day window for the first production start until the query
plan and database headroom have been checked. Late rows and source corrections
within that window create signed snapshot deltas;
the snapshot, account totals, point ledger, revision, and refresh audit are
committed atomically. A correction that would make an account total negative
is retained as `needs_review` instead of committing an invalid total.
An administrator-triggered historical refresh succeeds only if its audit event
is serialized and written successfully. Audit failure makes the entire request
fail; the API must not report an unaudited refresh as successful.

The one-time pre-launch history baseline is deliberately separate from this
rolling reconciliation. It processes one completed natural day per transaction,
persists a resumable cursor, and pins every covered date to the immutable initial
policy ratio. A later ordinary refresh of a covered date reuses that pinned ratio
instead of the disabled policy that predated launch. No browser endpoint can
start this operation.

## Configuration

Copy `.env.example` and replace every placeholder. Important details:

- `POINTS_DATABASE_URL` connects to the existing Sub2API database with the
  dedicated points application role. `POINTS_DATABASE_SCHEMA` defaults to
  `points`; `POINTS_DATABASE_MAX_CONNS` defaults to eight and cannot exceed 32.
- `POINTS_LAUNCH_HMAC_KEYS` contains versioned `key_id:base64_secret` entries.
- `POINTS_SUB2_CREDIT_KEY_ID` and `POINTS_SUB2_CREDIT_SECRET` select the active
  versioned, base64-decoded Sub2API balance bridge key.
- `POINTS_USAGE_DATABASE_URL` uses a separate Sub2API PostgreSQL login with
  only the grants in `deploy/usage-reader.sql.example`.
- `POINTS_PUBLIC_ORIGIN` is an origin without a path and must be HTTPS when
  secure cookies are enabled.
- `POINTS_EMBED_PARENT_ORIGIN` is the one exact Sub2API browser origin permitted
  by CSP `frame-ancestors`. It is required and rejects paths, queries, fragments,
  credentials, and wildcard hosts. Do not add `X-Frame-Options` at Nginx because
  it would conflict with the exact cross-origin embedding policy.

Generate 32-byte secrets with a CSPRNG. Production points, bridge, and psql
variable files are root-owned mode `0600`. Never place production secrets in
Git, logs, shell history, Compose output, or API responses.

## Deployment

`compose.example.yml` and `deploy/nginx.conf.example` are templates. Set
`POINTS_IMAGE` to the verified GHCR digest emitted by the image workflow; the
production template intentionally has no local `build` fallback and does not
start another PostgreSQL container. Before first start, take a verified backup
of the existing Sub2API database, run `deploy/shared-database-bootstrap.sql.example`
as the PostgreSQL bootstrap superuser, and provision the separate read-only login
from `deploy/usage-reader.sql.example`. Both templates require fresh role names,
run in a transaction, and fail instead of changing shared PUBLIC ACLs. Supply
their psql variables through a root-only stdin file rather than process arguments.
The bootstrap installs `btree_gist` and creates only the isolated points schema
and role; embedded migrations then run inside that schema.

The Compose template publishes the service on loopback only and joins the
existing Sub2API Docker network for the read-only database and balance bridge.
Set `SUB2API_DOCKER_NETWORK` to the exact existing network name. Set
`POINTS_TRUSTED_PROXY_CIDR` to the exact bridge gateway `/32` observed by the
points container; forwarded client IP headers are ignored from every other
source. Only the Nginx `/launch` location disables access logging so one-time
tickets are not persisted in query-string logs. `/app/`, `/admin/`, assets,
APIs, denials, authorization failures, and rate limits retain access logs.
The launch URL used by the Sub2API iframe must include `ui_mode=embedded`, and
its browser origin must exactly match `POINTS_EMBED_PARENT_ORIGIN` including any
non-default port.

The user profile field `checkin_available` is a read-only eligibility result. It
checks that check-in is enabled, the daily count remains, yesterday's snapshot
is ready and review-free, minimum prior-day spend and the selected points tier
match, and both user and platform monetary caps have remaining headroom. The
same rules are revalidated transactionally on submission, which remains
authoritative if eligibility changes after the profile response.

The process serializes migrations with a PostgreSQL advisory lock and verifies
`current_schema()` before running them, so a missing schema cannot fall back to
`public`. The first policy is disabled. Ordinary policy changes are created
through the admin API and become effective tomorrow or later. The dedicated
history tool has one fail-closed exception for initial launch: while no enabled
policy exists, it may append an enabled policy for the current natural day with
check-in off and refresh minute `00:05`. It never updates the original policy.

Serializable write transactions retry PostgreSQL `40001` serialization failures
and `40P01` deadlocks with context-aware jittered backoff, up to eight total
attempts. Transaction callbacks must reset captured mutable result state because
the callback can run more than once.

The Sub2API deployment must configure the matching public URL, launch secret,
credit secret, launch TTL, and clock-skew tolerance. The read-only usage login
must be reachable from the points container; no usage producer is deployed.

## Initial history baseline

Keep the Sub2API `points_system.enabled` bridge switch off for this entire
workflow. Take and checksum a fresh database backup first, then deploy only the
updated points image, let migration `003_usage_history_backfill.sql` complete,
and verify the service. This does not require replacing or restarting Sub2API.

The image contains `/usr/local/bin/points-history-backfill`. Run it as a
one-shot Compose process with the same root-owned environment file. Initial
activation appends a policy; the ratio below is `10.00 points/U` in internal
hundredth-point units, and check-in remains disabled:

The one-shot command caps its points write pool at two connections: one for the
runner lock and one for the per-day transaction. The configured application
pool must therefore allow at least two connections; the production default of
eight leaves headroom for the still-running points service.

User launch and existing user sessions remain fail-closed for this special
initial policy until its matching history job reaches `succeeded`; administrator
sessions remain available for verification.

```bash
docker compose --env-file "$POINTS_ENV_FILE" -f points-system/compose.example.yml \
  run --rm --no-deps --entrypoint /usr/local/bin/points-history-backfill \
  points-system activate --actor-user-id ADMIN_USER_ID \
  --points-per-usd-hundredths 1000
```

Record the returned `policy.version_no`. The dry run performs one read-only
range aggregate over successful balance consumption only. `--from auto` uses
the earliest qualifying `usage_logs` row; `--through` must be a completed day
before the activation policy's effective date:

```bash
docker compose --env-file "$POINTS_ENV_FILE" -f points-system/compose.example.yml \
  run --rm --no-deps --entrypoint /usr/local/bin/points-history-backfill \
  points-system plan --from auto --through YYYY-MM-DD \
  --policy-version POLICY_VERSION
```

Review the range, unique users, user-days, business days, source rows, spend,
points, and maximum usage-log ID. Apply only the exact fresh plan by supplying
its confirmation fingerprint:

```bash
docker compose --env-file "$POINTS_ENV_FILE" -f points-system/compose.example.yml \
  run --rm --no-deps --entrypoint /usr/local/bin/points-history-backfill \
  points-system apply --from auto --through YYYY-MM-DD \
  --policy-version POLICY_VERSION --actor-user-id ADMIN_USER_ID \
  --confirm-fingerprint PLAN_FINGERPRINT
```

`apply` writes one day at a time. `--max-days N` provides an intentional batch
stop without marking the job complete. A terminated or failed job retains its
next date and is continued with the same ID:

```bash
docker compose --env-file "$POINTS_ENV_FILE" -f points-system/compose.example.yml \
  run --rm --no-deps --entrypoint /usr/local/bin/points-history-backfill \
  points-system status --job-id JOB_ID

docker compose --env-file "$POINTS_ENV_FILE" -f points-system/compose.example.yml \
  run --rm --no-deps --entrypoint /usr/local/bin/points-history-backfill \
  points-system resume --job-id JOB_ID --actor-user-id ADMIN_USER_ID
```

The final day repeats the full read-only source summary and compares it with the
dry run. It also requires applied calendar days, unique users, user-days,
business days, source rows, maximum source ID, spend delta, and point delta to
match exactly. Drift or pre-existing accounting leaves the job `failed`; do not
enable the bridge. Only after a `succeeded` job, database/account/ledger totals,
service health, embedded UI, and launch denial while disabled have all been
verified may the Sub2API bridge be prepared for manual enablement. Check-in stays
off until a later explicit policy version enables it.

The baseline is one-time for the schema. Re-applying the exact successful plan
returns the original successful job; a different second plan is rejected. While
the initial baseline is pending, user access is globally closed and no later
enabled policy can bypass it. A site with no qualifying historical usage still
applies one audited empty completed day so the gate can finish without creating
spend or points.

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
checks migration idempotency, expired security-state cleanup, concurrent
daily/user/platform check-in caps, bounded rejection storage, settled
unreversed check-in totals, unknown credit reversal guards, and rollback of
snapshot-not-ready idempotency state. It also exercises initial same-day policy
activation, history-plan drift failure and resume, one-time ledger application,
and pinned-policy reconciliation when `POINTS_TEST_DATABASE_URL` is available.
