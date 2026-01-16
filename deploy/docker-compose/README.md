# Docker Compose Deployment

Deploy gorestic-homelab using Docker Compose.

## Quick Start

1. Copy and configure environment variables:
   ```bash
   cp .env.example .env
   # Edit .env with your values
   ```

2. Configure backup settings:
   ```bash
   # Edit config.yaml with your backup configuration
   ```

3. Run a one-time backup:
   ```bash
   docker compose run --rm gorestic-backup
   ```

## Scheduling Backups

### Using Host Cron

Add to your crontab (`crontab -e`):

```cron
# Run backup daily at 3 AM
0 3 * * * cd /path/to/deploy/docker-compose && docker compose run --rm gorestic-backup >> /var/log/gorestic.log 2>&1
```

### Using systemd Timer

Create `/etc/systemd/system/gorestic-backup.service`:

```ini
[Unit]
Description=gorestic-homelab backup
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
WorkingDirectory=/path/to/deploy/docker-compose
ExecStart=/usr/bin/docker compose run --rm gorestic-backup
```

Create `/etc/systemd/system/gorestic-backup.timer`:

```ini
[Unit]
Description=Run gorestic-homelab backup daily

[Timer]
OnCalendar=*-*-* 03:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gorestic-backup.timer
```

## Wake-on-LAN Support

If you need Wake-on-LAN functionality, uncomment `network_mode: host` in docker-compose.yaml. This gives the container access to the host network for sending WoL packets.

## Local REST Server

To run a local restic REST server as backup target, uncomment the `rest-server` service in docker-compose.yaml and start it:

```bash
docker compose up -d rest-server
```

Then update your config.yaml to use:
```yaml
restic:
  repository: "rest:http://localhost:8000/backup/"
```
