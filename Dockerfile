# API server (Go + embedded Postgres)
FROM artifact.it.att.com/astra-secure-container-catalog/go:1.26.5 AS build
ENV GOPROXY=https://nm2553@att.com:***REMOVED-CREDENTIAL***@artifact.it.att.com/artifactory/api/go/att-go-public-group
ENV GONOSUMDB=off
ENV GOPRIVATE=it.att.com
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ingestion-server .

# in its own container, so there is no non-root requirement here.
FROM  artifact.it.att.com/docker-hub/debian:bookworm-slim
ENV GOPROXY=https://nm2553@att.com:***REMOVED-CREDENTIAL***@artifact.it.att.com/artifactory/api/go/att-go-public-group
ENV GONOSUMDB=off
ENV GOPRIVATE=it.att.com
# embedded-postgres downloads real PostgreSQL binaries over HTTPS on first
# start and runs them directly, so this needs glibc (not Alpine/musl) plus
# CA certs for the download.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --shell /bin/false app

WORKDIR /app
COPY --from=build /out/ingestion-server ./ingestion-server
COPY data/ ./data/
COPY web/ ./web/
RUN mkdir -p /app/pgvolume && chown -R app:app /app
USER app

# Mount a volume at /app/pgvolume (see docker-compose.yml) so the embedded
# Postgres data directory survives container recreation, not just restarts.
# PGDATA_PATH must be a subdirectory *inside* that mount, not the mount
# point itself — embedded-postgres os.RemoveAll()s this path on a fresh
# start, and you can't unlink a volume's mount point (device busy).
ENV PGDATA_PATH=/app/pgvolume/data

EXPOSE 8080
CMD ["./ingestion-server"]
