# sub2api video edge worker

This Worker consumes the opaque AES-GCM URLs issued by sub2api and streams the
video directly from the provider/CDN. Provider URLs, task paths, and optional
upstream authorization headers remain encrypted inside the short-lived token.

## Configure

1. Copy `wrangler.toml.example` to the local, ignored `wrangler.toml` and set the
   custom route.
2. Generate one 32-byte random key. Configure the same 64-character hex value as
   the Worker secret and as `gateway.video_proxy.encryption_key` in sub2api.
3. Set `gateway.video_proxy.mode: edge` and
   `gateway.video_proxy.edge_base_url: https://image.52token.org` only after the
   Worker route is live.

The Worker route must be limited to `image.52token.org/v1/video-content/*` so the
existing workbench, static assets, and all other API paths continue to use the
current origin.

```bash
npx wrangler secret put VIDEO_PROXY_KEY_HEX
npm test
npx wrangler deploy
```

`ALLOWED_MEDIA_HOSTS` is optional. When the provider CDN host set is known, use
an exact/wildcard comma-separated allowlist. The Worker always rejects non-HTTPS,
credential-bearing, local, private-literal, and malformed targets.

The Worker supports `GET`, `HEAD`, byte ranges, conditional requests, and up to
four internal redirects. It returns only allowlisted media headers and generic
errors, so upstream response bodies and redirect locations never reach users.
