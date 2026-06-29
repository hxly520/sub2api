# sub2api cache compatibility image

This worktree builds a cache-compatibility fork of official `sub2api` `v0.1.139`.

The runtime image is intended to keep the same configuration, volumes, entrypoint,
healthcheck, and `/app/data` behavior as the official image. The only runtime
payload changed in the produced image archive is `/app/sub2api`.

## Image artifact

- Image tag inside archive: `gptcodex/sub2api:0.1.139`
- Base image: `weishaw/sub2api:0.1.139`
- Base linux/amd64 digest: `sha256:2bc58a1af11c0b3e22c7c1c1b99b65da46e480e3936d01a86db3a4f3caef5e7b`
- Produced image digest: `sha256:53e364e307dffb5a0161cfe50c2b7f8fde28260015d52f8a84280d580528dae0`
- Docker archive: `build-cachecompat/gptcodex-sub2api-0.1.139-cachecompat.docker.tar`
- Archive SHA256: `aa5369758613df2e59430747bc43be74650a20df53b0a162adf3fdd78b1a286a`
- Binary SHA256: `fb1b2844c180cbe401e62d548b2ada286a6da6e1284aaeb45f016d99894f94ef`

## What changed

The fork auto-derives and injects a stable `prompt_cache_key` for OpenAI
APIKey/ServiceAccount traffic when the client does not provide one.

Covered paths:

- `/v1/chat/completions` compatibility traffic that is forwarded upstream as
  OpenAI Responses.
- `/v1/responses` traffic for OpenAI APIKey/ServiceAccount accounts.

Behavior:

- Existing client-provided `prompt_cache_key` is preserved.
- `session_id` and `conversation_id` headers can be used as the stable seed.
- If no explicit session signal exists, a content-derived stable seed is used.
- The generated key is short-hashed and includes API key ID and model isolation.
- Image-generation intent is skipped so image endpoints are not affected.
- Official OAuth behavior remains unchanged.

## Import on server

Copy the archive to the server, then import it:

```bash
docker load -i gptcodex-sub2api-0.1.139-cachecompat.docker.tar
docker image inspect gptcodex/sub2api:0.1.139
```

If the archive is still in this worktree:

```bash
docker load -i /Users/hxly520/Documents/中转站/sub2api-cachecompat-v0.1.139/build-cachecompat/gptcodex-sub2api-0.1.139-cachecompat.docker.tar
```

## Switch docker compose

In the existing compose file, only change the application image:

```yaml
services:
  sub2api:
    image: gptcodex/sub2api:0.1.139
```

Keep the existing environment variables, volumes, ports, database, Redis, and
Nginx configuration unchanged.

Apply after confirming a maintenance window:

```bash
docker compose up -d sub2api
docker compose logs -f --tail=100 sub2api
```

## Roll back to official image

Change only the image back to the official tag:

```yaml
services:
  sub2api:
    image: weishaw/sub2api:0.1.139
```

Then restart the application container:

```bash
docker compose up -d sub2api
docker compose logs -f --tail=100 sub2api
```

The database and `/app/data` volume are shared with the official image, so no
configuration migration is required.

## Validation already run

```bash
go test ./internal/service -run 'TestForwardAsChatCompletions_APIKey(AutoDerivesPromptCacheKeyWhenMissing|PropagatesPromptCacheKeyInResponsesBody)|TestOpenAIGatewayService_APIKeyResponses(AutoDerivesPromptCacheKeyWhenMissing|PreservesExistingPromptCacheKey|DoesNotInjectPromptCacheKeyForImageIntent)' -count=1
git diff --check
crane validate --tarball build-cachecompat/gptcodex-sub2api-0.1.139-cachecompat.docker.tar
```

