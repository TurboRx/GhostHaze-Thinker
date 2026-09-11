FROM golang:1.27-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/turboot ./cmd/turboot

FROM alpine:3.21 AS production

LABEL org.opencontainers.image.source="https://github.com/TurboRx/Showdown-TurBOOT"
LABEL org.opencontainers.image.description="A Pokémon Showdown bot and client library in Go"
LABEL org.opencontainers.image.licenses="MIT"

# root certificates and tzdata are required for secure websocket connections
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S turboot && adduser -S turboot -G turboot

WORKDIR /app

COPY --from=builder /build/turboot /app/turboot

# run unprivileged for security
USER turboot

CMD ["/app/turboot"]
