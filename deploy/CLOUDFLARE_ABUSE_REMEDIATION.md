# 52Token Public-Page Security Remediation

This document records the durable remediation for Cloudflare abuse report
`d8c5351a87b6c954`. It intentionally contains no credentials, tokens, cookies,
or archived script bodies.

## Findings

- The former public help tree exposed shell and PowerShell installers that read
  local client credentials and rewrote local authentication configuration.
- Some retired downloads still referenced a previous service domain.
- The former CC Switch frontend bundle embedded and Base64-encoded a balance
  query script alongside API-key import logic.
- Custom home content was rendered with `v-html` without a dedicated allowlist.

No source-code path was found that deliberately sent credentials to an unknown
third-party origin. The public behavior was nevertheless too broad and matched
several static malware-detection signals, so it was removed instead of merely
renamed.

## Production Containment

- The original help scripts and skill archives were preserved for evidence at
  `/home/api/sub2api-deploy/security-archive/report-d8c5351a87b6c954-20260717-0918`.
- `/help/scripts/*` and `/help/skills/*` now return `410 Gone`.
- `/` and `/index.html` serve `deploy/public-landing/index.html` through exact
  Nginx locations. Other application routes continue to reach Sub2API.
- `/help/` serves `deploy/public-help/index.html` as static HTML.
- Both pages use `script-src 'none'`, `connect-src 'none'`, `frame-src 'none'`,
  `no-store`, and `X-Robots-Tag: noindex, nofollow, noarchive`.
- The production subtitle is `API 服务管理控制台`; custom home content remains
  empty and the CC Switch import button remains enabled.

Current production file checksums:

| File | SHA-256 |
| --- | --- |
| Public entry | `c5874fcd2ff5a4ea633610530d2151143b2182bb379531c3f618343ef4a0280a` |
| Public help | `926823de877cbb5548db011743ffa2ad9a3e39fb54598108e4b87e6c69d68395` |

## CC Switch Compatibility

One-click import and balance recognition are retained.

- The import still includes the selected API key, provider endpoint, client
  type, model mapping, and a 30-minute automatic balance refresh interval.
- The balance extractor still reads the platform `/v1/usage` response and
  supports `remaining`, `quota.remaining`, or `balance`.
- The fixed template is returned only by the JWT-protected
  `POST /api/v1/keys/ccswitch-usage-template` endpoint after the user clicks
  import. Responses are marked `no-store`.
- The public frontend bundle no longer contains the script body or a `btoa`
  encoding step.
- The fixed template contains no external origin, User-Agent inspection, input
  listener, dynamic import, `fetch`, XHR, WebSocket, `eval`, or `Function`.

Do not remove `usageEnabled`, `usageScript`, or `usageAutoInterval` from the CC
Switch deep link during future upgrades. Doing so would silently remove balance
recognition even if provider import still appeared to work.

## Durable Application Changes

- `HomeView.vue` uses a neutral service-console entry instead of commercial or
  provider-specific marketing copy.
- Custom home HTML is sanitized through a DOMPurify allowlist. Scripts, forms,
  inputs, media, inline styles, event attributes, and unsafe URL schemes are
  removed.
- HTTPS custom-home iframes use an empty sandbox and `no-referrer`.
- Static-page tests reject executable elements, credential collection surfaces,
  retired domains, external origins, and invalid responsive CSS math.
- Chinese and English defaults now use `API Service Management Console`.

## Upgrade Checklist

1. Keep the exact root include in both HTTP and HTTPS server blocks.
2. Keep the help include and both retired-download `410` locations.
3. Keep the static security-header include; do not weaken `script-src` or
   `connect-src` for these two pages.
4. Preserve the authenticated CC Switch template endpoint and its `no-store`
   headers when resolving merge conflicts.
5. Run frontend tests, typecheck, lint, production build, Go unit/integration
   tests, `go vet`, golangci-lint, and the security workflows.
6. Scan the built `KeysView` chunk: the `usageScript` parameter name is required,
   but the balance script body, `btoa`, retired domains, and dynamic execution
   primitives must be absent.
7. Verify desktop and 390 px mobile layouts have no horizontal document
   overflow.
8. Build images outside the production server. Upload the immutable Sub2API
   image without changing production Compose unless a separate rollout is
   explicitly approved.

## Rollback Boundary

The archived files are evidence, not deployable rollback artifacts. Do not make
the retired scripts or skill archives public again. A visual rollback may reuse
the application theme, but it must keep the static-page CSP, `410` locations,
home-content sanitization, and authenticated CC Switch balance-template flow.
