# syntax=docker/dockerfile:1

# ── Stage 1: build ─────────────────────────────────────────────
FROM golang:1.27-alpine AS build

WORKDIR /src

# Cache modules first so dependency download is not repeated on every change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/atlas-api ./cmd/atlas-api

# ── Stage 2: run ───────────────────────────────────────────────
# Distroless: no shell, no package manager, runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/atlas-api /usr/local/bin/atlas-api

EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/atlas-api"]