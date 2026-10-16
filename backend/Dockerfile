# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
# Download modules in their own layer so code changes do not re-fetch them.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The PostgreSQL driver is linked via the `pgx` build tag (cmd/api/driver_pgx.go).
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -tags pgx -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app && mkdir -p /data/uploads && chown -R app /data
USER app
ENV HTTP_ADDR=:8080 STORAGE_DIR=/data/uploads
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --retries=5 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
COPY --from=build /out/api /usr/local/bin/api
ENTRYPOINT ["/usr/local/bin/api"]
