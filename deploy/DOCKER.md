# Sub2API Docker Image

Sub2API is an AI API Gateway Platform for distributing and managing AI product subscription API quotas.

## Quick Start

```bash
export SUB2API_IMAGE=ghcr.io/hxly520/sub2api:0.1.183-52t.1  # pin the approved immutable release
# Authenticate once with a read:packages PAT when the GHCR package is private.
echo "$GHCR_READ_TOKEN" | docker login ghcr.io -u "$GHCR_USER" --password-stdin

docker run -d \
  --name sub2api \
  -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@host:5432/sub2api" \
  -e REDIS_URL="redis://host:6379" \
  "$SUB2API_IMAGE"
```

## Docker Compose

```yaml
version: '3.8'

services:
  sub2api:
    image: ${SUB2API_IMAGE:?pin a private release tag or digest}
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://postgres:postgres@db:5432/sub2api?sslmode=disable
      - REDIS_URL=redis://redis:6379
    depends_on:
      - db
      - redis

  db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=sub2api
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Yes | - |
| `REDIS_URL` | Redis connection string | Yes | - |
| `PORT` | Server port | No | `8080` |
| `GIN_MODE` | Gin framework mode (`debug`/`release`) | No | `release` |

## Supported Architectures

- `linux/amd64`
- `linux/arm64`

## Tags

- `x.y.z-52t.n` - Approved private release
- `sha-<commit>` or `x.y.z-<commit>` - Immutable compatibility-build candidate
- Production deployments must pin an approved tag or digest; do not rely on `latest`.

## Links

- [Private GitHub Repository](https://github.com/hxly520/sub2api)
- [Deployment Documentation](https://github.com/hxly520/sub2api/tree/main/deploy)
