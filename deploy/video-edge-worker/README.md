# sub2api media edge worker

This Worker consumes the opaque AES-GCM URLs issued by sub2api and streams
generated images or videos directly from the provider/CDN. Provider URLs, task paths, and optional
upstream authorization headers remain encrypted inside the short-lived token.

## Configure

1. Copy `wrangler.toml.example` to the local, ignored `wrangler.toml` and set the
   custom route.
2. Generate one 32-byte random key. Configure the same 64-character hex value as
   the Worker secret and as `gateway.video_proxy.encryption_key` in sub2api.
3. Set `gateway.video_proxy.mode: edge` and
   `gateway.video_proxy.edge_base_url: https://video.52token.org` only after the
   Worker route is live.

The Worker routes must be limited to `video.52token.org/v1/video-content/*` and
`video.52token.org/v1/image-content/*`. All other paths remain on the restricted
Nginx media virtual host.

```bash
npx wrangler secret put VIDEO_PROXY_KEY_HEX
npm test
npx wrangler deploy
```

`ALLOWED_MEDIA_HOSTS` is optional. When the provider CDN host set is known, use
an exact/wildcard comma-separated allowlist. The Worker rejects credentials,
local/private literals, malformed targets, and non-HTTPS video targets.

The Worker supports `GET`, `HEAD`, byte ranges, conditional requests, and up to
four internal redirects. Image tokens may target a public HTTP URL for legacy
providers; video targets remain HTTPS-only. It returns only allowlisted media
headers and generic errors, so upstream response bodies and redirect locations
never reach users.
