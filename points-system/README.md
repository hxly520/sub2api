# Sub2API Points System

Independent points and check-in service for the Sub2API secondary deployment.
The business contract is recorded in `PRODUCT_REQUIREMENTS_CN.md`; that file is
the source of truth for future compatibility work.
The current sanitized production handoff is recorded in
`../docs/PRODUCTION_DEPLOYMENT_20260731_CN.md`.

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

## Production Baseline (2026-08-02)

- Sub2API runs `ghcr.io/hxly520/sub2api:0.1.169-f79803bb73d6`, OCI revision
  `f79803bb73d659e36627d6f716aab065ff4d56a6`, container prefix
  `dee0f8efd24d`; the container is healthy with restart count zero. It was
  switched manually by the operator and remains outside automated replacement.
- The points service runs
  `ghcr.io/hxly520/sub2api-points:0.1.169-1d8d50522429`, OCI revision
  `1d8d50522429b5d943766ad1d1b4a14b82e31d80`, container prefix
  `1e5ed38b81da`, and is healthy with restart count zero. Its GHCR digest is
  `sha256:cc798629371d94898fbd3b049f4f454166b9e79f2893cee1e0a643344bacb2c2`,
  the transferred archive SHA256 is
  `e33e80c5b28307120881ccf269ebd7b5cae46c447173cc136182643ef56d960b`,
  and the loaded image ID is
  `sha256:5d4edb7822499e7c2953f7aa1f4889d88fbd9ac630fce69742d0a4f694e192dd`.
  This points-only replacement did not recreate or restart Sub2API.
- Both services use the same PostgreSQL 17.8 `sub2api` database. The isolated
  `points` schema contains 21 tables and four points migrations. `points_app`
  has an eight-connection limit; the column-restricted, read-only
  `points_usage_reader` has a four-connection limit.
- The running production `points_app` role completed stage A in transaction
  `1960217` and now has the exact `id/email/username/deleted_at` read-only
  allowlist. Stage B remains pending until user 1 and administrator launch
  tickets confirm the login-email UI, theme, pagination, and refresh behavior;
  it then removes `username`. The role retains no table-wide user-table access,
  no other user columns, and no write permission throughout.
- Sub2API has 250 applied public migrations. Private migrations
  `192_media_balance_hold_reconciliation_index_notx.sql` and
  `193_points_balance_credit_ledger.sql` are applied; points migrations remain
  separate in `points.points_schema_migrations`.
- Policy version 4 has been effective since `2026-08-02`. It is enabled at
  `10.00 points/U`, refreshes at `00:05`, permits one check-in per natural day,
  and uses `consumer_only` with the `yesterday` basis. Its percentage reward
  range is `1,000-50,000 PPM` (`0.1%-5%` of the prior natural day's successful
  balance spend), and its per-grant, per-user daily, and platform daily safety
  caps are each `100 U`. Historical job
  `5174eef7-5f0a-4a17-b4f1-f50840940f64` remains the only successful baseline.
  Policy version 5 was appended through the administrator API on `2026-08-02`
  and becomes effective on `2026-08-03`. It keeps `10.00 points/U`, `00:05`,
  `consumer_only/yesterday`, and one daily check-in, but uses raw-spend tiers:
  `[1,10) U` at `1%-5%`, `[10,50) U` at `2%-5%`, `[50,100) U` at `3%-5%`,
  and `[100,+inf) U` at `4%-5%`. The minimum prior-day spend is `1 U`; all
  three monetary caps are `NULL` (unlimited). Until that date, version 4 is
  still authoritative. Production has 29 point accounts, 339 daily snapshots,
  333 point-ledger rows, one accepted check-in, and one settled balance grant.
  This production schema must not run another history plan or apply.
- Production is now in all-user deployment mode: Sub2API has
  `POINTS_SYSTEM_ENABLED=true` with an empty preview list, and the points
  service has `POINTS_USER_ACCESS_MODE=all` with an empty preview list. The
  current policy's `enabled` field is the user-facing switch: when enabled,
  every active user may see the menu and open `/points`; when disabled, the
  menu, launch ticket, session, page resources, and user APIs fail closed.
  Preview mode remains available only for an explicitly staged rollout.
- Check-in has a separate deployment gate. `POINTS_CHECKIN_ACCESS_MODE=preview`
  with `POINTS_CHECKIN_PREVIEW_IDS=1` keeps the points center available to all
  users while exposing the balance-affecting check-in feature only to user 1.
  Non-preview users receive `checkin_enabled=false` and direct check-in POSTs
  are rejected server-side.
- The uploaded Sub2API logo integration, deleted-user session invalidation,
  per-request preview enforcement, Sub2API-matched light/dark palette,
  login-email browser identity, compact cards, paginated records, primary
  points focus, semantic synchronization status, 8 px panel scale, and reduced
  motion behavior are retained in points revision `1d8d50522429`.
  Sub2API remains on `f79803bb73d6` and was not recreated by the points image
  replacement.

### Production check-in acceptance (user 1, 2026-08-02)

- The `00:05` scheduler successfully settled business date `2026-08-01` for
  user 1: successful prior-day spend was `86.890694 U` and the resulting prior-
  day points were `868.90`.
- The configured percentage tier produced a quantized theoretical reward range
  of `0.08-4.34 U`. Cryptographic sampling selected `35,537 PPM` (`3.5537%`),
  resulting in an actual `3.08 U` reward.
- Grant UUID `8e20f4f9-d3ab-4d16-95be-0b186c96da97` reached `settled`. Sub2API
  contains exactly one matching credit row. Replaying the same idempotency key
  returned the original settled `3.08 U` result without another credit; a new
  key for a second same-day check-in returned `409 daily check-in limit reached`.
- User 2 retained points-center access but received `checkin_enabled=false`, and
  a direct check-in POST returned `403 checkin_unavailable`. All users other
  than user 1 still have zero check-ins, check-in attempts, and daily check-in
  counters for this test.

### Sub2API credit compatibility boundary

The first production credit attempt reached the older Sub2API build but returned
`500` because its `points.balance_credit` audit insert supplied
`audit_logs.request_body=NULL` while the production column is `NOT NULL`. The
credit was retried with the original grant UUID after installing the narrowly
scoped compatibility trigger `points_credit_audit_request_body_compat` on
`public.audit_logs`; function `public.points_credit_audit_request_body_compat()`
substitutes an empty string only when `action='points.balance_credit'` and
`request_body IS NULL`. It does not relax the column constraint or affect other
audit actions. The retry settled once and did not duplicate the balance credit.

The permanent source fix is prepared and already loaded on the server as
`ghcr.io/hxly520/sub2api:0.1.169-1a4a690dd999` (GHCR digest
`sha256:d9646464040e846999f960e3050646fcfe7cac38695834ba85df21385ae5c3ef`,
archive SHA256
`302f996c047c09919e8af53455851f0e18d7fd53d9c06640f8b2e3de7398c477`,
image ID
`sha256:07303dd1787d08a3038ba347a3fdaf0f78296f5f7a01aaf67ccce31edcd4ab16`).
The Compose service still points to `0.1.169-f79803bb73d6`; only the operator may
perform the manual Sub2API switch. After `1a4a690dd999` is running and its credit
path is verified, remove the temporary compatibility objects:

```sql
DROP TRIGGER IF EXISTS points_credit_audit_request_body_compat ON public.audit_logs;
DROP FUNCTION IF EXISTS public.points_credit_audit_request_body_compat();
```

### Final spend-tier production release

The source revision `1d8d50522429b5d943766ad1d1b4a14b82e31d80` adds raw-spend
tiers, nullable monetary caps, migration `004_checkin_spend_tiers_and_optional_caps.sql`,
and the final private rule set: `1-<10 U` at `1%-5%`, `10-<50 U` at `2%-5%`,
`50-<100 U` at `3%-5%`, and `100 U+` at `4%-5%`, with a `1 U` minimum and one
check-in per natural day. The GHCR image is
`ghcr.io/hxly520/sub2api-points:0.1.169-1d8d50522429`, manifest digest
`sha256:cc798629371d94898fbd3b049f4f454166b9e79f2893cee1e0a643344bacb2c2`,
and the locally verified archive SHA256 is
`e33e80c5b28307120881ccf269ebd7b5cae46c447173cc136182643ef56d960b`.
The archive is deployed at
`/home/api/sub2api-points/releases/sub2api-points-0.1.169-1d8d50522429-linux-amd64.tar`.
Migration `004` was applied at `2026-08-02 09:19 CST`; the startup reconciliation
reported `changed_users=0`. Policy version 5 was then saved through the
administrator API for `2026-08-03`. The pre-change full-database backup is
`/home/api/backups/points-spend-tiers-20260802-091604` with dump SHA256
`dae794a05a1d43bd13e2c9baab55969b35d4fb7f08420899d9cddb8ad0634e24`.
The three monetary caps are intentionally `NULL` (unlimited); the daily count
limit, idempotency, serializable transaction, overflow guard, and user-1-only
check-in gate remain mandatory.

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
6. Check-in locks the daily user/platform counters, selects either a point or
   raw-spend tier, calculates a reward using `crypto/rand`, applies every
   configured monetary cap, records the immutable rule snapshot, and enqueues
   a balance grant in one serializable transaction. A null monetary cap means
   unlimited; the positive daily check-in count limit remains mandatory.
7. The outbox worker signs an idempotent Sub2API balance-credit request. A
   retryable or unknown result must be retried with the same transaction UUID
   until Sub2API confirms settlement. Only settled credits can enqueue a debit
   reversal; only a never-attempted pending credit can be cancelled locally.
   Attempt history is never deleted.

## Exact Units

- Money: micro-USD (`1 U = 1,000,000`, `0.01 U = 10,000`).
- Point conversion: hundredths of a point per U (`10.25 = 1025`).
- Point totals and point-tier bounds: hundredths of a point.
- Spend-tier bounds: micro-USD, cent aligned.
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
| GET | `/api/v1/daily-points?days=7|30|90` | Complete prior natural-day window, zero-filled and chronological |
| GET | `/api/v1/ledger` | Read-only point ledger |
| POST | `/api/v1/checkins` | Check in; requires `Idempotency-Key` |
| GET | `/api/v1/balance-grants` | Current user's balance reward delivery history |
| GET | `/api/v1/admin/users/points` | Paginated per-user total and previous-day consumption points |
| GET/POST | `/api/v1/admin/policies` | List or append policy versions |
| POST | `/api/v1/internal/user-access` | Signed Sub2API-only policy-aware user entry decision; returns only `allowed` |
| GET | `/api/v1/admin/balance-grants` | Inspect check-in reward deliveries only |
| GET | `/api/v1/admin/balance-grants/summary` | Full status counts for all check-in reward deliveries |
| POST | `/api/v1/admin/balance-grants/{id}/retry` | Retry a failed check-in reward delivery |
| POST | `/api/v1/admin/balance-grants/{id}/reverse` | Audit and reverse/cancel a check-in reward delivery |
| GET | `/healthz` | Database-backed health check |

Every user-facing identity is the Sub2API login email: the user and
administrator headers, the administrator user-points directory, and the user
column in check-in balance-grant delivery records. Numeric Sub2API user IDs
remain server-side keys for joins, financial records, audit attribution, and
idempotency only. Browser APIs must return `login_email` where an identity is
needed and must not return an otherwise unnecessary `user_id`.

The built-in user and administrator workspaces are separate Chinese pages at
`/app/` and `/admin/`, with separate scripts. Both require a session created by
`/launch`; the domain root returns 404. A user session cannot fetch the admin
page, admin script, or any `/api/v1/admin/*` endpoint. The user dashboard shows
total/yesterday points, today's and settled
unreversed check-in credits, a 7/30/90-day points trend, and personal records;
it contains no policy, manual grant, snapshot, retry, or reversal controls.
Its four summary cards use a compact equal-height layout without redundant
disabled-check-in copy. The check-in action uses an internal responsive grid,
not absolute positioning, so labels, values, and longer status text cannot
overlap. The personal ledger and check-in reward history have
independent previous/next controls and render ten rows per page. Their APIs use
user-bound signed keyset cursors (`id` for ledger rows and `(created_at,id)` for
check-in grants), so records inserted between page requests do not duplicate or
hide older rows. The visual tokens match
Sub2API's light `gray-50/white` surfaces and dark
`dark-950/dark-800/dark-700` surfaces, with the same teal primary scale. Table
rows, hover states, pagination, status chips, and chart colors must all remain
readable in both themes. The administrator page uses the same visual language
in a denser operations layout; sharing visual tokens does not merge pages,
scripts, roles, or API permissions.
The current visual contract uses `DESIGN_VARIANCE=6`, `MOTION_INTENSITY=4`, and
`VISUAL_DENSITY=5`: this remains an embedded product workspace rather than a
marketing page, while the user surface receives a clearer primary-score focal
point, a low-contrast precision grid, compact settlement context, and semantic
loading/ready/error synchronization state. All tool panels and metric cards keep
the established 8-pixel radius scale. Hover lift is limited to two pixels and
is removed under `prefers-reduced-motion`. It keeps the existing CSS tokens,
permitted Lucide sprite, and native canvas as one design system; a second
component runtime must not be added only for decoration. The canvas always uses
its measured CSS width. Date ticks are capped at six and
are reduced against an 88-pixel readability interval, so a 390-pixel viewport
renders three or four separated labels instead of scaling a fixed-width plot.
Ordinary table hover uses a light primary tint rather than a selected-row fill,
and the four-column check-in grant table has its own narrower desktop minimum.
Administrator pending work uses the amber semantic surface, while repeated
reward tiers are divided rows inside the policy editor instead of nested cards.
Its check-in grant history uses an administrator-bound signed keyset cursor and
independent previous/next controls; it is not truncated to the latest 100 rows.
User and administrator routes are role-exact in both directions. An
administrator session cannot invoke the user account, ledger, check-in, or
grant APIs, and the shared logout route is the only role-neutral write route.
Dashboard refresh controls retain a stable button reference across asynchronous
work so they always leave the busy state. Every browser API request has a
20-second client timeout that is cleared on completion and returns a localized,
retryable timeout message. Check-in keeps one idempotency key, login email,
business date, and pre-request count in session storage while a network outcome
is uncertain. An authoritative profile count increase clears that pending
intent; the key is also retained after a successful POST until that confirmation
arrives. Otherwise the action becomes an explicit result-confirmation retry that
replays the same key. It must never create a second reward or remain permanently
disabled after a recoverable request.
If the initial profile or any required dashboard request fails, the user page
replaces default zero placeholders with a persistent error state and an explicit
retry button. The iframe still sends its ready message so the parent can reveal
that actionable failure instead of leaving an endless skeleton. The
administrator workspace remains behind its loading state until its initial
policy, user, and grant summaries have completed, so placeholder values are
never presented as live business state.
Session reads used by pages, assets, and APIs do not update `last_seen_at`, so a
single page load does not create parallel write locks in the shared database.

The administrator workspace has no independent manual balance-grant control;
Sub2API remains the only administrator surface for direct balance changes. It
also has no manual snapshot-refresh control. Daily snapshots are internal,
idempotent accounting records produced automatically at the configured refresh
time, while the one-time full historical baseline is executed only by the
audited `points-history-backfill` operations workflow below. The administrator
user directory reads only `users.id`, `users.email`, and `users.deleted_at`
from Sub2API, then exposes login emails, points, and successful-spend totals plus
the prior natural day's settlement state. Zero-spend users remain visible with
zero totals. Page controls stay locked while a request is active and commit the
requested offset only after a successful response, preserving the prior page
after a failure. The delivery workspace uses the same login-email projection for
check-in reward records only; legacy manual-grant rows remain in the database
for audit but are not listed or mutable through the points HTTP API.

Sub2API opens the user workspace inside its authenticated right-hand content
area. It appends the exact allowlisted `ui_mode=embedded` value to `/launch`, and
the points service preserves that value on the redirect to `/app/`. This mode
changes presentation only: it does not select a role, relax ticket or session
checks, or expose administrator APIs. The embedded user view hides its
standalone top bar so the Sub2API sidebar, current light/dark theme, and uploaded
logo remain authoritative. Later parent theme changes use the
`sub2api:points-theme` message; the child applies only `light` or `dark` when
both `event.source` and the exact configured parent Origin match. Once received,
the parent theme remains authoritative across later profile refreshes, and a
theme change redraws the canvas trend immediately instead of retaining stale
light or dark chart colors.

The administrator route `/admin/settings/points` remains in Sub2API for bridge
status and launch controls, but the policy workspace is not appended as a
bottom iframe. After step-up authentication, **Open points settings** creates a
one-time administrator launch URL and opens it in a new browser tab with an
isolated opener and no-referrer policy; the original Sub2API settings page remains in place. A
blocked popup is reported as an explicit launch failure. The launched
administrator workspace keeps its compact points-only navigation and exact
administrator role checks.
Before navigation completes, the isolated tab receives the active Sub2API
light/dark color scheme, theme color, language, accessible busy status, and a
reduced-motion-aware progress line. This waiting document keeps the existing
`opener=null` and no-referrer guarantees and does not add a component runtime.

Direct standalone access keeps the original layouts.
Standalone brand slots on both pages receive the exact Sub2API logo URL from
the server: `POINTS_EMBED_PARENT_ORIGIN/api/v1/settings/logo`. Letter placeholders
such as the former user/admin marks are not part of the contract. The CSP adds
only the exact configured parent origin to `img-src`; if that image cannot load,
the shared script applies `/assets/logo.svg` once as the image-bundled,
authenticated fallback. Logo loading does not select a role or relax launch,
session, Origin, or administrator API checks.
For the user workspace, the parent waits for a role- and Origin-validated
`sub2api:points-ready` message from the iframe instead of treating an HTTP error
document as a loaded workspace. The iframe receives scripts, forms, and
same-origin access only; it does not receive popup, top-navigation, or clipboard
permissions. The administrator launch is a separate browser tab and does not
reuse this iframe-ready protocol.

Sub2API's Points tab in System Settings and the administrator route
`/admin/settings/points` are always visible to authenticated administrators, including while
`points_system.enabled=false`, so an administrator can inspect bridge status
and use step-up authentication to launch the policy workspace in a new tab. The
ordinary user menu, `/points` route, and user launch ticket share a signed policy-aware
authorization rule. Sub2API asks the points service at
`POST /api/v1/internal/user-access` before exposing the entry; the points
service then checks its own rollout mode and effective policy. The browser
receives only the current user's `points_system_access` boolean; neither
preview list is exposed. A failed status query is fail-closed, and the user
launch handler repeats the same check before issuing a ticket.
The points service independently rejects user launch tickets, user pages,
user assets, and user APIs when the effective points policy is missing,
disabled, incomplete, or is the initial activation policy whose confirmed
history baseline has not succeeded. This closes stale-session and direct-URL paths while
leaving the authenticated administrator workspace available for pre-release
debugging.
The administrator policy workspace is a single editor: saving never edits or
deletes a historical row and always appends a new immutable version for the
next natural day. The policy `enabled` field is the business switch for the
user points center. The HTTP handler rejects a client-supplied effective date
that is not exactly the next service-local natural day, so hiding the date
input is backed by a server-side invariant.
The Sub2API status endpoint returns only non-secret state such as
enabled/configured/active, public URL, key IDs, TTL, and clock skew; it never
returns launch or credit key material.

## Trust Contracts

### Policy-aware access query

Sub2API signs the JSON body `{"user_id":N}` with the existing launch-key
ring. The canonical request is:

```text
v1
KEY_ID
POST
/api/v1/internal/user-access
UNIX_TIMESTAMP
RANDOM_NONCE
SHA256_HEX_OF_BODY
```

The points service verifies the timestamp, key, nonce shape, method, path, and
body before reading the policy. This endpoint is not a browser API; Nginx may
proxy the path for the Sub2API service, but unsigned requests receive `401`.

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
Every operations-triggered reconciliation or history application succeeds only
if its audit event is serialized and written successfully. Audit failure makes
the entire operation fail. There is no browser endpoint or administrator UI
control for refreshing snapshots.

The personal ledger's displayed **issued at** (`awarded_at`) value is a business timestamp,
not an alias for the immutable row's insertion time. For consumption rows with
`kind=usage_points` and a non-null `business_date`, it is the start of the next
`Asia/Shanghai` natural day plus the `refresh_minute` from the policy effective
on that award day. It deliberately does not reuse the consumption-day or
ledger-bound policy at a schedule transition; the current default therefore
displays `00:05` on the day after `business_date`. A history-backfill row whose
award day predates the first effective policy uses its immutable ledger policy's
`refresh_minute` as the fallback. Non-consumption, legacy, or rows with neither
schedule nor ledger policy fall back to `created_at`. This display projection
must not rewrite `points_ledger.created_at` or any historical ledger row.

The one-time pre-launch history baseline is deliberately separate from this
rolling reconciliation. It processes one completed natural day per transaction,
persists a resumable cursor, and pins every covered date to the immutable initial
policy ratio. A later ordinary refresh of a covered date reuses that pinned ratio
instead of the disabled policy that predated launch. No browser endpoint can
start this operation.

## Configuration

Copy `.env.example` and replace every placeholder. Important details:

User rollout is enforced independently by both services. Production uses
Sub2API `enabled: true` and points service mode `all`, with both preview lists
empty. The lists are server-only: browsers receive only the current user's
`points_system_access` result. Once the bridge is configured, the policy
`enabled` toggle controls the signed runtime decision: a disabled or
not-yet-ready policy hides the user menu and rejects user tickets, while the
administrator workspace remains available. Do not emulate a global rollout by
expanding preview lists.

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
  credentials, and wildcard hosts. The same exact origin is the only additional
  CSP `img-src` and is used to construct the uploaded-logo URL; no independent
  arbitrary image origin is accepted. Do not add `X-Frame-Options` at Nginx
  because it would conflict with the exact cross-origin embedding policy.
- `POINTS_USER_ACCESS_MODE` is required and is exactly `preview` or `all`. Preview mode requires
  one to 10,000 positive comma-separated IDs in `POINTS_USER_PREVIEW_IDS`; all
  mode requires that list to be empty. This is an independent fail-closed gate,
  not a replacement for the Sub2API menu, route, and ticket checks.
- `POINTS_CHECKIN_ACCESS_MODE` defaults to `all` and is exactly `preview` or
  `all`. Preview mode requires one to 10,000 positive comma-separated IDs in
  `POINTS_CHECKIN_PREVIEW_IDS`; all mode requires that list to be empty. This
  gate affects only check-in availability and POST authorization, never points
  center visibility.

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
and role; its Sub2API user-table allowlist is exactly `id`, `email`, and
`deleted_at`. Embedded migrations then run inside that schema.

For an existing deployment created before the login-email UI, do not rerun the
bootstrap or the already executed username upgrade. First take and verify a
database backup, record the pre-change points account/snapshot/ledger and
Sub2API user counts, and load the existing role name as the `points_app_role`
psql variable from a root-only stdin file. Run stage A
`deploy/shared-database-users-email-upgrade.sql.example` as the PostgreSQL
bootstrap superuser. It atomically grants `SELECT (email)` while retaining the
legacy `SELECT (username)`, and asserts the exact transitional
`id/email/username/deleted_at` allowlist. Update only the points container and
verify login-email identities, exact role isolation, preview user 1, non-preview
denial, and unchanged accounting counts. Only then run stage B
`deploy/shared-database-users-email-finalize.sql.example`, which atomically
revokes `username` and asserts the final `id/email/deleted_at` allowlist.

To roll back after stage B, first run
`deploy/shared-database-users-email-rollback-prepare.sql.example`; it restores
`username` without removing `email`, so either image remains usable. Switch to
the old image and complete its username checks before running
`deploy/shared-database-users-email-rollback-finalize.sql.example` to remove
`email` and restore the exact legacy `id/username/deleted_at` state. Every stage
retains non-secret before/after audit output, rejects table-wide/extra-column/
write/PUBLIC access, and rolls back atomically on failure. The historical
username upgrade and its 2026-07-31 audit remain immutable evidence.

Both the regular CI workflow and the points-image workflow run
`deploy/ci/shared-database-users-email-acl-test.sh` against an isolated
PostgreSQL 16 service. The test exercises stage A, stage B, both rollback
stages, rejection of an expanded source ACL, exact column grants, and an
injected pre-commit failure that must leave the legacy ACL unchanged.

The Compose template publishes the service on loopback only and joins the
existing Sub2API Docker network for the read-only database and balance bridge.
Set `SUB2API_DOCKER_NETWORK` to the exact existing network name. Set
`POINTS_TRUSTED_PROXY_CIDR` to the exact bridge gateway `/32` observed by the
points container; forwarded client IP headers are ignored from every other
source. Only the Nginx `/launch` location disables access logging so one-time
tickets are not persisted in query-string logs. `/app/`, `/admin/`, assets,
APIs, denials, authorization failures, and rate limits retain access logs.
The user launch URL used by the Sub2API iframe must include `ui_mode=embedded`, and
its browser origin must exactly match `POINTS_EMBED_PARENT_ORIGIN` including any
non-default port.
Verify both authenticated workspaces with an uploaded Sub2API logo and with a
forced parent-logo load failure. The first case must display the uploaded raster
logo; the second must display `/assets/logo.svg` without a CSP violation loop.
Neither case may make the user admin HTML/script/API reachable, and the image
fallback must not be served outside an authenticated points session.

The user profile field `checkin_available` is a read-only eligibility result. It
checks that check-in is enabled, the daily count remains, yesterday's snapshot
is ready and review-free, minimum prior-day spend and the selected private tier
match, and every configured monetary cap has remaining headroom. Spend tiers
are forced to the prior natural day's raw successful balance spend; neither an
administrator request nor a direct store call can select cumulative spend. The
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

The current production `points` schema already completed this one-time workflow
as job `5174eef7-5f0a-4a17-b4f1-f50840940f64`. Do not run `activate`, `plan`,
`apply`, or `resume` against that schema during an image update. The commands
below are retained only for a new empty schema or disaster-recovery validation
whose existing job state has first been audited.

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
service health, embedded user UI, administrator new-tab launch, preview-user
success, and non-preview launch denial have all been verified may the Sub2API
bridge be prepared for manual preview. Check-in stays off until a later explicit
policy version enables it; all-user access stays off until separate acceptance.

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
