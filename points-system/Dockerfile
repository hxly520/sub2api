# syntax=docker/dockerfile:1.7
FROM golang:1.27.0-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git tzdata
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/points-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /out/points-history-backfill ./cmd/history-backfill

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S points && adduser -S -G points points
COPY --from=build /out/points-server /usr/local/bin/points-server
COPY --from=build /out/points-history-backfill /usr/local/bin/points-history-backfill
COPY THIRD_PARTY_NOTICES.md /usr/share/licenses/sub2api-points/THIRD_PARTY_NOTICES.md
USER points:points
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/points-server"]
