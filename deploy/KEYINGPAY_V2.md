# KeyingPay V2 deployment notes

KeyingPay V2 uses the following public callback endpoint:

```text
GET|POST /api/v1/payment/webhook/keyingpay
```

The application route is public but verifies every callback with the configured
KeyingPay platform RSA public key. It returns plain text `success` only after the
notification is accepted or identified as an unknown legacy order.

## Nginx

If the existing Nginx configuration already proxies all `/api/` traffic to
sub2api without an allowlist, no extra location is required. If API routes are
allowlisted, add an exact-match location to the public API virtual host:

```nginx
location = /api/v1/payment/webhook/keyingpay {
    limit_except GET POST { deny all; }

    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    proxy_no_cache 1;
    proxy_cache_bypass 1;
    proxy_read_timeout 30s;
}
```

Keep the original request query string and form body. Do not rewrite this path,
redirect GET to POST, or add authentication in Nginx.

Validate before reloading:

```bash
nginx -t
systemctl reload nginx
```

## Cloudflare

For `52token.org`, do not add this as a third custom rule. The exact callback
exception is already part of the first consolidated Skip rule in
[`CLOUDFLARE_52TOKEN.md`](CLOUDFLARE_52TOKEN.md); the China block rule must remain
immediately after it. The expression below is the narrow provider requirement
for installations that do not use that consolidated policy.

Create a narrow rule for the API hostname and exact callback path:

```text
(http.host eq "api.52token.org"
 and http.request.uri.path eq "/api/v1/payment/webhook/keyingpay"
 and http.request.method in {"GET" "POST"})
```

For this exact match only:

- bypass cache;
- skip browser/JavaScript challenges and bot challenges that block server callbacks;
- keep TLS, DDoS protection, request logging, and normal origin proxying enabled;
- preserve query parameters and request bodies;
- do not redirect or transform the request method.

Use `Full (strict)` origin TLS. Do not create a broad skip rule for all `/api/*`.

## Provider configuration

- API base: `https://api.keyingpay.org`
- Notify URL: `https://api.52token.org/api/v1/payment/webhook/keyingpay`
- Return URL: `https://52token.org/payment/result`
- Keys: merchant private key and platform public key from the KeyingPay merchant console

The merchant private key and platform public key use the existing provider
configuration storage and are omitted from admin API responses. Restrict
database and backup access accordingly. Never place them in Nginx, Cloudflare
rules, compose files, or repository documentation.

## Smoke check

After a KeyingPay provider instance is configured, an unsigned callback should
reach sub2api and return an application response such as `400 verify failed`.
A Cloudflare block page, `403`, or cached HTML means the request did not reach
the payment webhook handler.
