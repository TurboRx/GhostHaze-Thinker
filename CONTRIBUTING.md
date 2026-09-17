# Contributing to GhostHaze-Thinker

Thank you for your interest in contributing to GhostHaze-Thinker! This document outlines our development process, guidelines, and quality standards to help you get started.

---

## Code of Conduct

All contributors and maintainers are expected to adhere to our [Code of Conduct](CODE_OF_CONDUCT.md). Please treat everyone with respect and kindness.

---

## How to Contribute

### Reporting Issues

- Search the issue tracker before opening a new issue to avoid duplicates.
- Use our issue templates for bug reports and feature suggestions.
- Provide clear steps to reproduce, expected versus actual behavior, and relevant logs (redacting sensitive tokens or credentials).

### Submitting Pull Requests

1. Fork the repository and create your branch from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```
2. Write clean, readable, well-tested code.
3. Ensure all tests pass with the race detector enabled.
4. Run static analysis and linting before pushing.
5. Submit your pull request with a descriptive title and summary using our pull request template.

---

## Development Setup

### Prerequisites

- **Go 1.22+** (Go 1.24+ recommended)
- **Git**
- Optional: **Docker** & **docker-compose** for containerized workflows

### Building and Running Locally

```bash
# clone repository
git clone https://github.com/TurboRx/GhostHaze-Thinker.git
cd GhostHaze-Thinker

# download dependencies
go mod download
go mod verify

# compile binary
go build -o ghosthaze-thinker ./cmd/ghosthaze-thinker

# run the bot
./ghosthaze-thinker
```

### Running Tests

Run the full test suite with race detection enabled:

```bash
go test -v -race ./...
```

### Linting and Code Analysis

Verify code health using Go's built-in static analysis and `golangci-lint`:

```bash
# standard go static analysis
go vet ./...

# golangci-lint
golangci-lint run ./...
```

---

## Code Style and Guidelines

To keep the repository clean, consistent, and maintainable:

1. **Comment Style**:
   - Keep only useful developer comments explaining non-obvious logic, protocol behaviors, or concurrency details.
   - All code comments must be strictly lowercase.
   - Avoid noisy or redundant comments that merely restate what the code does.

2. **Domain Terminology**:
   - Use **"Chatroom"** / **"Chatrooms"** when referring to Pokémon Showdown rooms.
   - Use **"user"** / **"users"** when referring to participants, players, and accounts.

3. **Concurrency and Safety**:
   - Protect shared mutable state with appropriate synchronization (`sync.RWMutex` or `sync.Mutex`).
   - Avoid nested lock acquisitions that could risk deadlocks.
   - Ensure slices and maps are properly initialized to avoid nil pointer panics and null serialization in JSON endpoints.

4. **Web UI Consistency**:
   - Use custom modals and the built-in notification system (`showAlert`) rather than native browser pop-ups (`alert`, `confirm`, `prompt`).
   - Maintain dark-mode-first responsive design principles.

5. **Commit Messages**:
   - Write clear, imperative, and descriptive commit messages.
   - Avoid conventional commit prefixes (e.g. `feat:`, `fix:`, `chore:`).

---

## Security

If you discover a security vulnerability, please do not open a public issue. Follow our disclosure policy in [SECURITY.md](SECURITY.md).
