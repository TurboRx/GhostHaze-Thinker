# ── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.27-alpine AS builder

WORKDIR /build

# Copy dependency manifests first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/

# Compile static binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/turboot ./cmd/turboot

# ── Production stage ────────────────────────────────────────────────────────
FROM alpine:3.21 AS production

LABEL org.opencontainers.image.source="https://github.com/TurboRx/Showdown-TurBOOT"
LABEL org.opencontainers.image.description="A Pokémon Showdown bot and client library in Go"
LABEL org.opencontainers.image.licenses="MIT"

# Install SSL root certificates and tzdata, create unprivileged user
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S turboot && adduser -S turboot -G turboot

WORKDIR /app

# Copy only the compiled static binary
COPY --from=builder /build/turboot /app/turboot

USER turboot

CMD ["/app/turboot"]
