# syntax=docker/dockerfile:1.7
FROM golang:1.26.5-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git tzdata
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/points-server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S points && adduser -S -G points points
COPY --from=build /out/points-server /usr/local/bin/points-server
USER points:points
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/points-server"]
