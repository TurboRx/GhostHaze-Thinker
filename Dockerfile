FROM golang:1.27-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/ghosthaze-thinker ./cmd/ghosthaze-thinker

FROM alpine:3.24 AS production

LABEL org.opencontainers.image.source="https://github.com/TurboRx/GhostHaze-Thinker"
LABEL org.opencontainers.image.description="A Pokémon Showdown bot and client library in Go"
LABEL org.opencontainers.image.licenses="MIT"

# root certificates and tzdata are required for secure websocket connections
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S ghosthaze && adduser -S ghosthaze -G ghosthaze

WORKDIR /app

COPY --from=builder /build/ghosthaze-thinker /app/ghosthaze-thinker

# run unprivileged for security
USER ghosthaze

CMD ["/app/ghosthaze-thinker"]
