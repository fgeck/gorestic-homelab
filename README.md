# gorestic-homelab

A Go-based restic backup orchestrator for homelab environments.

## Features

- **Wake-on-LAN**: Wake backup targets before starting
- **PostgreSQL Backups**: Automated pg_dump with configurable format
- **Restic Backup**: Full restic backup with retention policies
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

### Scheduling with Cron

```cron
# Run backup daily at 3 AM
0 3 * * * /usr/local/bin/gorestic-homelab run --config /etc/gorestic/config.yaml
```

### Scheduling with systemd Timer

```ini
# /etc/systemd/system/gorestic-backup.service
[Unit]
Description=gorestic-homelab backup

[Service]
Type=oneshot
ExecStart=/usr/local/bin/gorestic-homelab run --config /etc/gorestic/config.yaml
```

```ini
# /etc/systemd/system/gorestic-backup.timer
[Unit]
Description=Run gorestic-homelab backup daily

[Timer]
OnCalendar=*-*-* 03:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

## Configuration

See [config.example.yaml](config.example.yaml) for a complete example.

### Required Settings

```yaml
restic:
  repository: "rest:http://192.168.1.100:8000/backup/"
  password: "${RESTIC_PASSWORD}"

backup:
  paths:
    - /data
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
  target_url: "http://192.168.1.100:8000"
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

When you run `gorestic-homelab run`, the following steps are executed in order:

1. **Wake-on-LAN** (if configured) - Wake the backup target
2. **Initialize Repository** - Initialize restic repo if needed
3. **PostgreSQL Dump** (if configured) - Create database dump
4. **Backup** - Run restic backup
5. **Retention Policy** - Apply forget/prune rules
6. **Repository Check** (if enabled) - Verify repository integrity
7. **SSH Shutdown** (if configured) - Shutdown remote server
8. **Notification** (if configured) - Send Telegram message

## Development

### Prerequisites

- Go 1.23+
- Docker (for integration tests)

### Building

```bash
go build -o gorestic-homelab ./cmd/gorestic-homelab
```

### Testing

```bash
# Unit tests
go test -v ./...

# Integration tests (requires Docker services)
docker run -d -p 8000:8000 restic/rest-server
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=test postgres:16

TEST_RESTIC_REPO="rest:http://localhost:8000/test" \
TEST_RESTIC_PASSWORD="test" \
TEST_POSTGRES_HOST="localhost" \
TEST_POSTGRES_PASSWORD="test" \
TEST_POSTGRES_DB="postgres" \
go test -tags=integration -v ./integration/...
```

### Linting

```bash
golangci-lint run
```

## License

MIT License - see [LICENSE](LICENSE) for details.
