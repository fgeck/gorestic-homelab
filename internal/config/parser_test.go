package config

import (
	"os"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_LoadReader_MinimalConfig(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	assert.Equal(t, "/backup", cfg.Restic.Repository)
	assert.Equal(t, "secret", cfg.Restic.Password)
	assert.Equal(t, []string{"/data"}, cfg.Backup.Paths)
	// Check defaults
	assert.Equal(t, 7, cfg.Retention.KeepDaily)
	assert.Equal(t, 4, cfg.Retention.KeepWeekly)
	assert.Equal(t, 6, cfg.Retention.KeepMonthly)
	assert.True(t, cfg.Restic.FailOnLocked) // Default is true
}

func TestParser_LoadReader_FullConfig(t *testing.T) {
	yaml := `
restic:
  repository: "rest:http://192.168.1.100:8000/backup/"
  password: "secret123"
  rest_user: "backup"
  rest_password: "restpass"

backup:
  paths:
    - /data
    - /home
  tags:
    - daily
    - important
  host: "myserver"

retention:
  keep_daily: 14
  keep_weekly: 8
  keep_monthly: 12

check:
  enabled: true
  subset: "5%"

wol:
  mac_address: "AA:BB:CC:DD:EE:FF"
  broadcast_ip: "192.168.1.255"
  poll_url: "http://192.168.1.100:8000"
  timeout: 10m
  poll_interval: 5s
  stabilize_wait: 15s

postgres:
  host: "192.168.1.100"
  port: 5433
  database: "myapp"
  username: "dbuser"
  password: "dbpass"
  format: "plain"

ssh_shutdown:
  host: "192.168.1.100"
  port: 2222
  username: "admin"
  key_path: "/home/user/.ssh/id_rsa"
  shutdown_delay: 5

telegram:
  bot_token: "123456:ABC"
  chat_id: "-100123456789"
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)

	// Restic config
	assert.Equal(t, "rest:http://192.168.1.100:8000/backup/", cfg.Restic.Repository)
	assert.Equal(t, "secret123", cfg.Restic.Password)
	assert.Equal(t, "backup", cfg.Restic.RestUser)
	assert.Equal(t, "restpass", cfg.Restic.RestPassword)

	// Backup settings
	assert.Equal(t, []string{"/data", "/home"}, cfg.Backup.Paths)
	assert.Equal(t, []string{"daily", "important"}, cfg.Backup.Tags)
	assert.Equal(t, "myserver", cfg.Backup.Host)

	// Retention
	assert.Equal(t, 14, cfg.Retention.KeepDaily)
	assert.Equal(t, 8, cfg.Retention.KeepWeekly)
	assert.Equal(t, 12, cfg.Retention.KeepMonthly)

	// Check
	assert.True(t, cfg.Check.Enabled)
	assert.Equal(t, "5%", cfg.Check.Subset)

	// WOL
	require.NotNil(t, cfg.WOL)
	assert.Equal(t, "AA:BB:CC:DD:EE:FF", cfg.WOL.MACAddress)
	assert.Equal(t, "192.168.1.255", cfg.WOL.BroadcastIP)
	assert.Equal(t, "http://192.168.1.100:8000", cfg.WOL.PollURL)
	assert.Equal(t, 10*time.Minute, cfg.WOL.Timeout)
	assert.Equal(t, 5*time.Second, cfg.WOL.PollInterval)
	assert.Equal(t, 15*time.Second, cfg.WOL.StabilizeWait)

	// Postgres
	require.NotNil(t, cfg.Postgres)
	assert.Equal(t, "192.168.1.100", cfg.Postgres.Host)
	assert.Equal(t, 5433, cfg.Postgres.Port)
	assert.Equal(t, "myapp", cfg.Postgres.Database)
	assert.Equal(t, "dbuser", cfg.Postgres.Username)
	assert.Equal(t, "dbpass", cfg.Postgres.Password)
	assert.Equal(t, "plain", cfg.Postgres.Format)

	// SSH Shutdown
	require.NotNil(t, cfg.SSHShutdown)
	assert.Equal(t, "192.168.1.100", cfg.SSHShutdown.Host)
	assert.Equal(t, 2222, cfg.SSHShutdown.Port)
	assert.Equal(t, "admin", cfg.SSHShutdown.Username)
	assert.Equal(t, "/home/user/.ssh/id_rsa", cfg.SSHShutdown.KeyPath)
	assert.Equal(t, 5, cfg.SSHShutdown.ShutdownDelay)

	// Telegram
	require.NotNil(t, cfg.Telegram)
	assert.Equal(t, "123456:ABC", cfg.Telegram.BotToken)
	assert.Equal(t, "-100123456789", cfg.Telegram.ChatID)
}

func TestParser_LoadReader_EnvVarExpansion(t *testing.T) {
	// Set test environment variables
	t.Setenv("TEST_RESTIC_PASSWORD", "env_secret")
	t.Setenv("TEST_REST_PASSWORD", "env_rest_pass")

	yaml := `
restic:
  repository: "/backup"
  password: "${TEST_RESTIC_PASSWORD}"
  rest_password: "$TEST_REST_PASSWORD"
backup:
  paths:
    - /data
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	assert.Equal(t, "env_secret", cfg.Restic.Password)
	assert.Equal(t, "env_rest_pass", cfg.Restic.RestPassword)
}

func TestParser_LoadReader_MissingRepository(t *testing.T) {
	yaml := `
restic:
  password: "secret"
backup:
  paths:
    - /data
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "restic.repository is required")
}

func TestParser_LoadReader_MissingPassword(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
backup:
  paths:
    - /data
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "restic.password is required")
}

func TestParser_LoadReader_MissingPaths(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  tags:
    - test
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "backup.paths is required")
}

func TestParser_LoadReader_WOL_MissingMACAddress(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
wol:
  target_url: "http://localhost:8000"
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wol.mac_address is required")
}

func TestParser_LoadReader_WOL_Defaults(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
wol:
  mac_address: "AA:BB:CC:DD:EE:FF"
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	require.NotNil(t, cfg.WOL)
	assert.Equal(t, "255.255.255.255", cfg.WOL.BroadcastIP)
	assert.Equal(t, 5*time.Minute, cfg.WOL.Timeout)
	assert.Equal(t, 10*time.Second, cfg.WOL.PollInterval)
	assert.Equal(t, 10*time.Second, cfg.WOL.StabilizeWait)
}

func TestParser_LoadReader_WOL_WithPollURL(t *testing.T) {
	yaml := `
restic:
  repository: "rest:http://192.168.1.100:8000/backup/"
  password: "secret"
backup:
  paths:
    - /data
wol:
  mac_address: "AA:BB:CC:DD:EE:FF"
  poll_url: "http://192.168.1.100:8000"
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	require.NotNil(t, cfg.WOL)
	assert.Equal(t, "http://192.168.1.100:8000", cfg.WOL.PollURL)
}

func TestParser_LoadReader_Postgres_MissingDatabase(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
postgres:
  host: "localhost"
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "postgres.database is required")
}

func TestParser_LoadReader_Postgres_InvalidFormat(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
postgres:
  database: "test"
  format: "invalid"
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "postgres.format must be one of")
}

func TestParser_LoadReader_Postgres_Defaults(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
postgres:
  database: "mydb"
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	require.NotNil(t, cfg.Postgres)
	assert.Equal(t, "localhost", cfg.Postgres.Host)
	assert.Equal(t, 5432, cfg.Postgres.Port)
	assert.Equal(t, "postgres", cfg.Postgres.Username)
	assert.Equal(t, "custom", cfg.Postgres.Format)
}

func TestParser_LoadReader_SSHShutdown_MissingHost(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
ssh_shutdown:
  key_path: "/home/user/.ssh/id_rsa"
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssh_shutdown.host is required")
}

func TestParser_LoadReader_SSHShutdown_MissingKeyPath(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
ssh_shutdown:
  host: "192.168.1.100"
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssh_shutdown.key_path is required")
}

func TestParser_LoadReader_SSHShutdown_Defaults(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
ssh_shutdown:
  host: "192.168.1.100"
  key_path: "/home/user/.ssh/id_rsa"
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	require.NotNil(t, cfg.SSHShutdown)
	assert.Equal(t, 22, cfg.SSHShutdown.Port)
	assert.Equal(t, "root", cfg.SSHShutdown.Username)
	assert.Equal(t, 1, cfg.SSHShutdown.ShutdownDelay)
}

func TestParser_LoadReader_Telegram_MissingBotToken(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
telegram:
  chat_id: "-100123456789"
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "telegram.bot_token is required")
}

func TestParser_LoadReader_Telegram_MissingChatID(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
telegram:
  bot_token: "123456:ABC"
`
	parser := NewParser()
	_, err := parser.LoadReader(yaml)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "telegram.chat_id is required")
}

func TestParser_LoadReader_DefaultHost(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	// Should use system hostname as default
	expectedHost, _ := os.Hostname()
	if expectedHost == "" {
		expectedHost = "unknown"
	}
	assert.Equal(t, expectedHost, cfg.Backup.Host)
}

func TestParser_LoadReader_FailOnLocked_DefaultTrue(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
backup:
  paths:
    - /data
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	assert.True(t, cfg.Restic.FailOnLocked)
}

func TestParser_LoadReader_FailOnLocked_ExplicitTrue(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
  fail_on_locked: true
backup:
  paths:
    - /data
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	assert.True(t, cfg.Restic.FailOnLocked)
}

func TestParser_LoadReader_FailOnLocked_ExplicitFalse(t *testing.T) {
	yaml := `
restic:
  repository: "/backup"
  password: "secret"
  fail_on_locked: false
backup:
  paths:
    - /data
`
	parser := NewParser()
	cfg, err := parser.LoadReader(yaml)

	require.NoError(t, err)
	assert.False(t, cfg.Restic.FailOnLocked)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *models.BackupConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "configuration is nil",
		},
		{
			name: "missing repository",
			cfg: &models.BackupConfig{
				Restic: models.ResticConfig{Password: "secret"},
				Backup: models.BackupSettings{Paths: []string{"/data"}},
			},
			wantErr: true,
			errMsg:  "restic.repository is required",
		},
		{
			name: "missing password",
			cfg: &models.BackupConfig{
				Restic: models.ResticConfig{Repository: "/backup"},
				Backup: models.BackupSettings{Paths: []string{"/data"}},
			},
			wantErr: true,
			errMsg:  "restic.password is required",
		},
		{
			name: "missing paths",
			cfg: &models.BackupConfig{
				Restic: models.ResticConfig{Repository: "/backup", Password: "secret"},
				Backup: models.BackupSettings{},
			},
			wantErr: true,
			errMsg:  "backup.paths is required",
		},
		{
			name: "valid config",
			cfg: &models.BackupConfig{
				Restic: models.ResticConfig{Repository: "/backup", Password: "secret"},
				Backup: models.BackupSettings{Paths: []string{"/data"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
