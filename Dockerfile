# API server (Go, talks to the "postgres" service over the network — see
# docker-compose.yml. No embedded/subprocess database here.)
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ingestion-server .

FROM debian:bookworm-slim
RUN useradd --create-home --shell /bin/false app

WORKDIR /app
COPY --from=build /out/ingestion-server ./ingestion-server
COPY data/ ./data/
COPY web/ ./web/
RUN chown -R app:app /app
USER app

EXPOSE 8080
CMD ["./ingestion-server"]
