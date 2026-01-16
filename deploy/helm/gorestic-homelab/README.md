# gorestic-homelab Helm Chart

A Helm chart for deploying gorestic-homelab as a Kubernetes CronJob.

## Installation

```bash
helm install gorestic-backup ./deploy/helm/gorestic-homelab \
  --namespace backup \
  --create-namespace \
  --set secrets.resticPassword=your-secure-password
```

## Configuration

See [values.yaml](values.yaml) for all available options.

### Required Parameters

| Parameter | Description |
|-----------|-------------|
| `secrets.resticPassword` | Restic repository password |
| `config.restic.repository` | Restic repository URL |

### Common Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `schedule` | `"0 3 * * *"` | Cron schedule |
| `timeZone` | `"Etc/UTC"` | Timezone for schedule |
| `image.repository` | `ghcr.io/fgeck/gorestic-homelab` | Image repository |
| `image.tag` | `""` (appVersion) | Image tag |
| `persistence.enabled` | `true` | Enable data PVC |
| `persistence.existingClaim` | `""` | Use existing PVC |
| `persistence.size` | `10Gi` | PVC size |

### Optional Features

#### PostgreSQL Backup

```yaml
config:
  postgres:
    enabled: true
    host: "postgres.default.svc"
    port: 5432
    database: "myapp"
    username: "postgres"
    format: "custom"

secrets:
  postgresPassword: "your-db-password"
```

#### Telegram Notifications

```yaml
config:
  telegram:
    enabled: true

secrets:
  telegramBotToken: "123456:ABC-DEF"
  telegramChatId: "-1001234567890"
```

#### SSH Shutdown

```yaml
config:
  sshShutdown:
    enabled: true
    host: "192.168.1.100"
    port: 22
    username: "root"
    keyPath: "/root/.ssh/id_rsa"
    shutdownDelay: 1

sshKey:
  enabled: true
  privateKey: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    ...
    -----END OPENSSH PRIVATE KEY-----
```

Or use an existing secret:

```bash
kubectl create secret generic my-ssh-key --from-file=id_rsa=/path/to/key -n backup
```

```yaml
sshKey:
  enabled: true
  existingSecret: "my-ssh-key"
```

#### Wake-on-LAN

```yaml
config:
  wol:
    enabled: true
    macAddress: "AA:BB:CC:DD:EE:FF"
    broadcastIp: "192.168.1.255"
    targetUrl: "http://192.168.1.100:8000"
    timeout: "5m"
    pollInterval: "10s"
    stabilizeWait: "10s"
```

### Using Existing Secrets

Instead of defining secrets in values, use an existing secret:

```bash
kubectl create secret generic gorestic-secrets \
  --from-literal=RESTIC_PASSWORD=your-password \
  --from-literal=POSTGRES_PASSWORD=your-db-password \
  -n backup
```

```yaml
secrets:
  existingSecret: "gorestic-secrets"
```

### Using Existing PVC

Back up data from an existing PVC:

```yaml
persistence:
  enabled: true
  existingClaim: "my-app-data"
```

## Examples

### Minimal Installation

```bash
helm install gorestic-backup ./deploy/helm/gorestic-homelab \
  --set secrets.resticPassword=supersecret \
  --set config.restic.repository=rest:http://rest-server:8000/backup/
```

### Full Featured Installation

```bash
helm install gorestic-backup ./deploy/helm/gorestic-homelab \
  --namespace backup \
  --create-namespace \
  -f my-values.yaml
```

Example `my-values.yaml`:

```yaml
schedule: "0 2 * * *"
timeZone: "Europe/Berlin"

secrets:
  resticPassword: "supersecret"
  postgresPassword: "dbpassword"
  telegramBotToken: "123456:ABC"
  telegramChatId: "-100123"

config:
  restic:
    repository: "rest:http://nas.local:8000/backup/"

  backup:
    paths:
      - /data
      - /config
    tags:
      - production
      - daily

  retention:
    keepDaily: 14
    keepWeekly: 8
    keepMonthly: 12

  postgres:
    enabled: true
    host: "postgres.default.svc"
    database: "myapp"

  telegram:
    enabled: true

persistence:
  existingClaim: "app-data"

resources:
  requests:
    memory: "256Mi"
    cpu: "200m"
  limits:
    memory: "1Gi"
    cpu: "1000m"
```

## Upgrading

```bash
helm upgrade gorestic-backup ./deploy/helm/gorestic-homelab -n backup -f my-values.yaml
```

## Uninstalling

```bash
helm uninstall gorestic-backup -n backup
```

Note: PVCs are not deleted automatically. Delete manually if needed:

```bash
kubectl delete pvc -l app.kubernetes.io/instance=gorestic-backup -n backup
```
