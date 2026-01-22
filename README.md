# gorestic-homelab

A Go-based restic backup orchestrator for homelab environments.

## Features

- **Wake-on-LAN**: Wake backup targets before starting
- **PostgreSQL Backups**: Automated pg_dump with configurable format
- **Restic Backup**: Full restic backup with retention policies
- **Lock Handling**: Detect stale locks with configurable auto-removal
- **SSH Shutdown**: Gracefully shutdown remote servers after backup
- **Telegram Notifications**: Get notified about backup status

## Installation

### Using Docker

```bash
docker pull ghcr.io/fgeck/gorestic-homelab:latest
```

### From Source

```bash
go install github.com/fgeck/gorestic-homelab/cmd/gorestic-homelab@latest
```

### Download Binary

Download pre-built binaries from the [releases page](https://github.com/fgeck/gorestic-homelab/releases).

## Usage

### Quick Start

1. Copy the example configuration:
   ```bash
   cp config.example.yaml config.yaml
   ```

2. Edit `config.yaml` with your settings

3. Validate your configuration:
   ```bash
   gorestic-homelab validate --config config.yaml
   ```

4. Run the backup:
   ```bash
   gorestic-homelab run --config config.yaml
   ```

### Docker Usage

```bash
docker run --rm \
  -v /path/to/config.yaml:/config.yaml:ro \
  -v /path/to/data:/data:ro \
  -v /path/to/ssh-key:/root/.ssh/id_rsa:ro \
  -e RESTIC_PASSWORD=your-password \
  ghcr.io/fgeck/gorestic-homelab:latest \
  run --config /config.yaml
```

## Configuration

See [config.example.yaml](config.example.yaml) for a complete example.

### Required Settings

```yaml
restic:
  repository: "rest:http://192.168.1.100:8000/backup/"
  password: "${RESTIC_PASSWORD}"
  fail_on_locked: true  # optional, default: true

backup:
  paths:
    - /data
```

#### Lock Handling

By default, `fail_on_locked: true` causes the backup to fail if the repository has stale locks from previous interrupted backups. This is the safe default to prevent concurrent access issues.

Set `fail_on_locked: false` to automatically remove stale locks and continue with the backup:

```yaml
restic:
  repository: "rest:http://192.168.1.100:8000/backup/"
  password: "${RESTIC_PASSWORD}"
  fail_on_locked: false  # auto-remove stale locks
```

### Environment Variable Expansion

All configuration values support environment variable expansion:

```yaml
restic:
  password: "${RESTIC_PASSWORD}"
postgres:
  password: "${POSTGRES_PASSWORD}"
```

### Optional Features

#### Wake-on-LAN

```yaml
wol:
  mac_address: "AA:BB:CC:DD:EE:FF"
  broadcast_ip: "192.168.1.255"
  poll_url: "http://192.168.1.100:8000"  # URL to poll until target is ready
  timeout: 5m
  poll_interval: 10s
  stabilize_wait: 10s
```

#### PostgreSQL Backup

```yaml
postgres:
  host: "192.168.1.100"
  port: 5432
  database: "myapp"
  username: "postgres"
  password: "${POSTGRES_PASSWORD}"
  format: "custom"  # custom, plain, or tar
```

#### SSH Shutdown

```yaml
ssh_shutdown:
  host: "192.168.1.100"
  port: 22
  username: "root"
  key_path: "${HOME}/.ssh/id_rsa"
  shutdown_delay: 1
```

#### Telegram Notifications

```yaml
telegram:
  bot_token: "${TELEGRAM_BOT_TOKEN}"
  chat_id: "${TELEGRAM_CHAT_ID}"
```

## CLI Reference

### Commands

- `run` - Execute the backup workflow
- `validate` - Validate configuration file

### Flags

- `-c, --config` - Path to configuration file (required)
- `-v, --verbose` - Enable verbose (debug) output
- `-q, --quiet` - Enable quiet mode (errors only)
- `--json` - Output logs in JSON format
- `--version` - Print version information

## Backup Workflow

When you run `gorestic-homelab run`, the following steps are executed:

1. **Wake-on-LAN** (if configured) - Wake the backup target and wait until ready
2. **Initialize Repository** - Initialize restic repository if it doesn't exist
3. **Lock Check** - Check for stale locks (fail or auto-remove based on `fail_on_locked`)
4. **PostgreSQL Dump** (if configured) - Create database dump to temporary file
5. **Backup** - Run restic backup (includes PostgreSQL dump if created)
6. **Retention Policy** - Apply forget/prune rules to manage snapshots
7. **Repository Check** (if enabled) - Verify repository integrity

After completion (success or failure):
- **SSH Shutdown** (if configured) - Shutdown remote server (only if WOL succeeded or wasn't used)
- **Telegram Notification** (if configured) - Send status message with backup statistics

## Development

### Prerequisites

- Go 1.23+
- Docker (for integration tests)
- [mockery](https://vektra.github.io/mockery/) (for generating mocks)
- [golangci-lint](https://golangci-lint.run/) (for linting)

### Makefile Commands

Run `make help` to see all available targets:

```
make help              # Show all available targets
```

#### Build & Run

```bash
make build             # Build the binary
make run               # Build and run the application
make clean             # Remove build artifacts
```

#### Code Quality

```bash
make fmt               # Format code with gofmt
make fmt-check         # Check code formatting (CI-friendly)
make vet               # Run go vet
make lint              # Run golangci-lint
make all               # Run fmt, lint, vet, test and build
```

#### Testing

```bash
make test              # Run unit tests (alias for test-unit)
make test-unit         # Run unit tests only
make test-unit-cover   # Run unit tests with coverage
make test-integration  # Run integration tests only
make test-e2e          # Run e2e tests only
make test-all          # Run all tests (unit, integration, e2e)
make cover             # Generate HTML coverage report
```

#### Integration Tests

```bash
make integration-up    # Start Docker services (postgres, restic-rest)
make integration-down  # Stop Docker services
make integration-local # Run integration tests with Docker services
make integration-ci    # Run integration tests in CI
```

#### Dependencies & Mocks

```bash
make deps              # Download dependencies
make deps-tidy         # Tidy dependencies (go mod tidy)
make mockery           # Generate mocks using mockery
```

#### Other

```bash
make docker-build      # Build Docker image
make install-hooks     # Install git pre-push hooks
```

## License

MIT License - see [LICENSE](LICENSE) for details.
