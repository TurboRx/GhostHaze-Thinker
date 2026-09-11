# TurBOOT

A Pokémon Showdown bot and client library in Go.

TurBOOT connects to Pokémon Showdown over WebSockets, authenticates with account credentials, automatically joins configured rooms, and provides an event-driven framework for handling chat messages, private messages (PMs), and custom commands.

It can be run as a standalone bot daemon or imported directly as a Go package (`pkg/showdown`) in your own applications.

---

## Features

- **Dual Purpose:** Ready-to-run bot daemon (`cmd/turboot`) and reusable client library (`pkg/showdown`).
- **Zero Heavy Dependencies:** Only uses standard Go and `gorilla/websocket`.
- **Automatic Reconnection:** Reconnects on connection loss with configurable backoff.
- **Full Authentication Support:** Handles challenge strings (`challstr`) and assertions via the Showdown login API.
- **Guest Fallback:** Seamlessly connects as a guest if credentials are not provided.
- **Event-Driven Hooks:** Callbacks for chat, private messages, room joins/leaves, popups, and raw protocol frames.
- **Command Router:** Simple registration for prefix commands (e.g. `.ping`).
- **Tiny Docker Footprint:** Statically compiled multi-stage build resulting in a ~6 MB container image running as non-root.

---

## Requirements

- **Go 1.27.1** or newer (to run or build natively)
- **Docker** and **Docker Compose** (optional, for containerized deployment)
- A Pokémon Showdown account (optional; connects as a guest without one)

---

## Quickstart

### 1. Clone & Configure

```bash
git clone https://github.com/TurboRx/turboot.git
cd turboot
cp .env.example .env
```

Edit `.env` with your bot's username and credentials:

```env
PS_USERNAME=YourBotName
PS_PASSWORD=YourBotPassword
PS_ROOMS=botdevelopment
PS_COMMAND_CHAR=.
```

*(Leave `PS_USERNAME` blank to connect as a guest.)*

### 2. Run Locally

```bash
# Run directly
go run ./cmd/turboot

# Or build a standalone binary
go build -o turboot ./cmd/turboot
./turboot
```

To gracefully stop the bot, press `Ctrl+C`.

---

## Running with Docker

### Docker Compose (Recommended)

```bash
# Start in the background
docker compose up -d --build

# Follow logs
docker compose logs -f

# Stop
docker compose down
```

### Standalone Docker

```bash
# Build the image (~6 MB)
docker build -t turboot .

# Run with environment variables
docker run -d --name turboot \
  -e PS_USERNAME=YourBotName \
  -e PS_PASSWORD=YourBotPassword \
  -e PS_ROOMS=botdevelopment \
  turboot

# Or mount your .env file
docker run -d --name turboot --env-file .env turboot
```

---

## Configuration Reference

All settings can be specified via environment variables or a local `.env` file:

| Variable | Default | Description |
|---|---|---|
| `PS_USERNAME` | _(empty)_ | Pokémon Showdown username (leaves as guest if blank) |
| `PS_PASSWORD` | _(empty)_ | Account password |
| `PS_ROOMS` | _(empty)_ | Comma-separated list of rooms to join upon login |
| `PS_SERVER_URL` | `wss://sim3.psim.us/showdown/websocket` | Showdown WebSocket endpoint |
| `PS_LOGIN_URL` | `https://play.pokemonshowdown.com/api/login` | HTTP assertion login endpoint |
| `PS_AVATAR` | _(empty)_ | Avatar sprite ID to set after logging in |
| `PS_RECONNECT_DELAY_MS` | `10000` | Milliseconds to wait before reconnecting |
| `PS_COMMAND_CHAR` | `.` | Command prefix symbol |

---

## Using as a Library

Because Go's module system allows importing subpackages directly, you can import `pkg/showdown` into any Go project without needing an external repository:

```bash
go get github.com/TurboRx/turboot/pkg/showdown
```

### Example Usage

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/TurboRx/turboot/pkg/showdown"
)

func main() {
	client := showdown.NewClient(showdown.Config{
		Username:    "MyBotName",
		Password:    "SecretPassword",
		Rooms:       []string{"lobby", "botdevelopment"},
		CommandChar: ".",
	})

	// Register chat listener
	client.OnChat(func(msg showdown.ChatMessage) {
		fmt.Printf("[%s] %s: %s\n", msg.Room, msg.User, msg.Text)
	})

	// Register custom command (.ping -> pong!)
	client.HandleCommand("ping", func(room, user, args string) {
		if room != "" {
			_ = client.SendToRoom(room, "pong!")
		} else {
			_ = client.SendPM(user, "pong!")
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Run(ctx); err != nil {
		fmt.Printf("Client stopped: %v\n", err)
	}
}
```

---

## Repository Structure

```
turboot/
├── cmd/
│   └── turboot/          # Bot application entrypoint (main.go)
├── internal/
│   └── config/           # Environment variable and .env parser
├── pkg/
│   └── showdown/         # Core reusable Pokémon Showdown client library
│       ├── client.go     # Connection loop, state, auth, and actions
│       ├── config.go     # Client configuration and defaults
│       ├── handler.go    # Event hooks and dispatcher
│       ├── message.go    # Protocol and frame parser
│       └── types.go      # Data models and structures
├── .env.example          # Sample environment variables
├── Dockerfile            # Multi-stage static build (~6 MB image)
├── docker-compose.yml    # Docker Compose definition
├── verify_docker.sh      # Automated validation script
└── go.mod
```

---

## Testing & Verification

Run the unit test suite:

```bash
go test -v ./...
```

Run the automated Docker and setup verification:

```bash
bash verify_docker.sh
```

---

## License

[MIT](LICENSE)
