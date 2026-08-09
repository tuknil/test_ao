# API server (Go + embedded Postgres)
FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/wizserver .

FROM debian:bookworm-slim
# embedded-postgres downloads real PostgreSQL binaries over HTTPS on first
# start and runs them directly, so this needs glibc (not Alpine/musl) plus
# CA certs for the download.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --shell /bin/false app

WORKDIR /app
COPY --from=build /out/wizserver ./wizserver
COPY data/ ./data/
RUN chown -R app:app /app
USER app

EXPOSE 8080
CMD ["./wizserver"]
