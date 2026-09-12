# GhostHaze-Thinker

[![CI](https://github.com/TurboRx/GhostHaze-Thinker/actions/workflows/ci.yml/badge.svg)](https://github.com/TurboRx/GhostHaze-Thinker/actions/workflows/ci.yml)
[![Docker](https://github.com/TurboRx/GhostHaze-Thinker/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/TurboRx/GhostHaze-Thinker/actions/workflows/docker-publish.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A Pokémon Showdown bot and client library written in Go.

Connects to Pokémon Showdown over WebSockets, handles authentication, auto-joins configured rooms, and provides an event-driven framework for commands, room chat, and private messages.

## Features

- **WebSocket Client**: Persistent connection with automatic reconnection, keepalive pings, and outbound throttle rate limiting.
- **Authentication**: Challenge string (`challstr`) login assertion with support for registered accounts and guest/unregistered bots.
- **Battle Engine**: Full battle protocol support, challenge detection & auto-acceptance, and competitive decision heuristics (18-type effectiveness, lethal KO priority, priority finishers, smart status infliction, healing thresholds, entry hazards, Terastallization, and team preview lead selection).
- **Room State Tracking**: Active user lists, user ranks, away status, room titles, and room renames.
- **Security & Command Routing**: Intro backlog ignore, command injection prevention (`EscapeChat`), and prefix-based routing.
- **Server Discovery & Side Servers**: Automatic resolution of server host, port, TLS, WebSocket URL, and login endpoints using Pokémon Showdown's discovery API, with built-in CLI tool support.
- **Lightweight Docker**: ~6 MB Alpine image published to GitHub Container Registry (`ghcr.io/turborx/ghosthaze-thinker`).

## Getting Started

### Prerequisites

- [Go](https://go.dev/) 1.27+ (for local development) or [Docker](https://www.docker.com/)

### Setup & Run

1. Clone the repository:
   ```bash
   git clone https://github.com/TurboRx/GhostHaze-Thinker.git
   cd GhostHaze-Thinker
   ```

2. Create your `.env` configuration:
   ```bash
   cp .env.example .env
   ```
   Set your `PS_USERNAME` and `PS_PASSWORD` in `.env` (leave blank to connect as a guest).

3. Run the bot:
   ```bash
   # Run directly
   go run ./cmd/ghosthaze-thinker

   # Or run with Docker
   docker compose up -d
   ```

### Server Discovery Tool

Inspect connection details for Pokémon Showdown servers using the built-in discovery tool:

```bash
go run ./cmd/ghosthaze-thinker -get-server play.pokemonshowdown.com
```

Output:
```text
Server: sim3.psim.us
Port: 443
Server-ID: showdown
Secure connection (TLS): YES
WebSocket URL: wss://sim3.psim.us/showdown/websocket
```

The bot also supports side servers by configuring `PS_SERVER_ID`, `PS_SERVER_HOST`, or `PS_SERVER_URL` in `.env`.

## Commands

Commands use the configured prefix (default `.`).

| Command | Description |
|---|---|
| `.ping` | Responds with `pong!` in room chat or PM. |
| `.battle [format]` | Challenges the user to a battle (defaults to `gen9randombattle`). |
| `.challenge [format]` | Alias for `.battle`. |

### Adding Commands

Register commands in `cmd/ghosthaze-thinker/main.go`:

```go
bot.HandleCommand("hello", func(room, user, args string) {
    _ = bot.Reply(room, user, "Hello, " + user + "!")
})
```

## Using as a Library

Import `pkg/showdown` into any Go project:

```bash
go get github.com/TurboRx/GhostHaze-Thinker/pkg/showdown
```

```go
package main

import (
    "context"

    "github.com/TurboRx/GhostHaze-Thinker/pkg/showdown"
)

func main() {
    client := showdown.NewClient(showdown.Config{
        Username: "ghosthaze thinker",
        Password: "yourpassword",
        Rooms:    []string{"botdevelopment"},
    })

    client.HandleCommand("ping", func(room, user, args string) {
        _ = client.Reply(room, user, "pong!")
    })

    _ = client.Run(context.Background())
}
```

## License

[MIT](LICENSE)
