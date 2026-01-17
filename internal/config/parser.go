// Package config provides configuration file parsing.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/spf13/viper"
)

// Parser handles configuration file parsing.
type Parser struct {
	v *viper.Viper
}

// NewParser creates a new configuration parser.
func NewParser() *Parser {
	v := viper.New()
	v.SetConfigType("yaml")
	return &Parser{v: v}
}

// LoadFile loads configuration from a file path.
func (p *Parser) LoadFile(path string) (*models.BackupConfig, error) {
	p.v.SetConfigFile(path)

	if err := p.v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	return p.parse()
}

// LoadReader loads configuration from a reader (useful for testing).
func (p *Parser) LoadReader(content string) (*models.BackupConfig, error) {
	if err := p.v.ReadConfig(strings.NewReader(content)); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	return p.parse()
}

//nolint:gocognit,gocyclo // parsing config requires checking many fields
func (p *Parser) parse() (*models.BackupConfig, error) {
	cfg := &models.BackupConfig{}

	// Parse restic config (required).
	cfg.Restic = models.ResticConfig{
		Repository:   p.expandEnv(p.v.GetString("restic.repository")),
		Password:     p.expandEnv(p.v.GetString("restic.password")),
		RestUser:     p.expandEnv(p.v.GetString("restic.rest_user")),
		RestPassword: p.expandEnv(p.v.GetString("restic.rest_password")),
	}

	if cfg.Restic.Repository == "" {
		return nil, fmt.Errorf("restic.repository is required")
	}
	if cfg.Restic.Password == "" {
		return nil, fmt.Errorf("restic.password is required")
	}

	// Parse backup settings (required).
	cfg.Backup = models.BackupSettings{
		Paths: p.v.GetStringSlice("backup.paths"),
		Tags:  p.v.GetStringSlice("backup.tags"),
		Host:  p.v.GetString("backup.host"),
	}

	if len(cfg.Backup.Paths) == 0 {
		return nil, fmt.Errorf("backup.paths is required")
	}

	// Set default host if not specified.
	if cfg.Backup.Host == "" {
		hostname, err := os.Hostname()
		if err != nil {
			cfg.Backup.Host = "unknown"
		} else {
			cfg.Backup.Host = hostname
		}
	}

	// Parse retention policy.
	cfg.Retention = models.RetentionPolicy{
		KeepDaily:   p.v.GetInt("retention.keep_daily"),
		KeepWeekly:  p.v.GetInt("retention.keep_weekly"),
		KeepMonthly: p.v.GetInt("retention.keep_monthly"),
	}

	// Set defaults if no retention policy specified.
	if cfg.Retention.KeepDaily == 0 && cfg.Retention.KeepWeekly == 0 && cfg.Retention.KeepMonthly == 0 {
		cfg.Retention.KeepDaily = 7
		cfg.Retention.KeepWeekly = 4
		cfg.Retention.KeepMonthly = 6
	}

	// Parse check settings.
	cfg.Check = models.CheckSettings{
		Enabled: p.v.GetBool("check.enabled"),
		Subset:  p.v.GetString("check.subset"),
	}

	// Parse optional WOL config.
	if p.v.IsSet("wol") { //nolint:nestif // config parsing with defaults
		cfg.WOL = &models.WOLConfig{
			MACAddress:    p.v.GetString("wol.mac_address"),
			BroadcastIP:   p.v.GetString("wol.broadcast_ip"),
			PollURL:       p.v.GetString("wol.poll_url"),
			Timeout:       p.v.GetDuration("wol.timeout"),
			PollInterval:  p.v.GetDuration("wol.poll_interval"),
			StabilizeWait: p.v.GetDuration("wol.stabilize_wait"),
		}

		if cfg.WOL.MACAddress == "" {
			return nil, fmt.Errorf("wol.mac_address is required when wol is configured")
		}

		// Set defaults.
		if cfg.WOL.BroadcastIP == "" {
			cfg.WOL.BroadcastIP = "255.255.255.255"
		}
		if cfg.WOL.Timeout == 0 {
			cfg.WOL.Timeout = 5 * time.Minute
		}
		if cfg.WOL.PollInterval == 0 {
			cfg.WOL.PollInterval = 10 * time.Second
		}
		if cfg.WOL.StabilizeWait == 0 {
			cfg.WOL.StabilizeWait = 10 * time.Second
		}
	}

	// Parse optional PostgreSQL config.
	if p.v.IsSet("postgres") { //nolint:nestif // config parsing with defaults
		cfg.Postgres = &models.PostgresConfig{
			Host:     p.v.GetString("postgres.host"),
			Port:     p.v.GetInt("postgres.port"),
			Database: p.v.GetString("postgres.database"),
			Username: p.v.GetString("postgres.username"),
			Password: p.expandEnv(p.v.GetString("postgres.password")),
			Format:   p.v.GetString("postgres.format"),
		}

		if cfg.Postgres.Host == "" {
			cfg.Postgres.Host = "localhost"
		}
		if cfg.Postgres.Port == 0 {
			cfg.Postgres.Port = 5432
		}
		if cfg.Postgres.Database == "" {
			return nil, fmt.Errorf("postgres.database is required when postgres is configured")
		}
		if cfg.Postgres.Username == "" {
			cfg.Postgres.Username = "postgres"
		}
		if cfg.Postgres.Format == "" {
			cfg.Postgres.Format = "custom"
		}

		// Validate format.
		validFormats := map[string]bool{"custom": true, "plain": true, "tar": true}
		if !validFormats[cfg.Postgres.Format] {
			return nil, fmt.Errorf("postgres.format must be one of: custom, plain, tar")
		}
	}

	// Parse optional SSH shutdown config.
	if p.v.IsSet("ssh_shutdown") { //nolint:nestif // config parsing with defaults
		cfg.SSHShutdown = &models.SSHShutdownConfig{
			Host:          p.v.GetString("ssh_shutdown.host"),
			Port:          p.v.GetInt("ssh_shutdown.port"),
			Username:      p.v.GetString("ssh_shutdown.username"),
			KeyPath:       p.expandEnv(p.v.GetString("ssh_shutdown.key_path")),
			ShutdownDelay: p.v.GetInt("ssh_shutdown.shutdown_delay"),
			OS:            p.v.GetString("ssh_shutdown.os"),
		}

		if cfg.SSHShutdown.Host == "" {
			return nil, fmt.Errorf("ssh_shutdown.host is required when ssh_shutdown is configured")
		}
		if cfg.SSHShutdown.Port == 0 {
			cfg.SSHShutdown.Port = 22
		}
		if cfg.SSHShutdown.Username == "" {
			cfg.SSHShutdown.Username = "root"
		}
		if cfg.SSHShutdown.KeyPath == "" {
			return nil, fmt.Errorf("ssh_shutdown.key_path is required when ssh_shutdown is configured")
		}
		if cfg.SSHShutdown.ShutdownDelay == 0 {
			cfg.SSHShutdown.ShutdownDelay = 1
		}
		// Validate and default OS
		if cfg.SSHShutdown.OS == "" {
			cfg.SSHShutdown.OS = "linux"
		}
		validOS := map[string]bool{"linux": true, "windows": true}
		if !validOS[cfg.SSHShutdown.OS] {
			return nil, fmt.Errorf("ssh_shutdown.os must be one of: linux, windows")
		}
	}

	// Parse optional Telegram config.
	if p.v.IsSet("telegram") {
		cfg.Telegram = &models.TelegramConfig{
			BotToken: p.expandEnv(p.v.GetString("telegram.bot_token")),
			ChatID:   p.expandEnv(p.v.GetString("telegram.chat_id")),
		}

		if cfg.Telegram.BotToken == "" {
			return nil, fmt.Errorf("telegram.bot_token is required when telegram is configured")
		}
		if cfg.Telegram.ChatID == "" {
			return nil, fmt.Errorf("telegram.chat_id is required when telegram is configured")
		}
	}

	return cfg, nil
}

// expandEnv expands environment variables in the format ${VAR} or $VAR.
func (p *Parser) expandEnv(s string) string {
	return os.ExpandEnv(s)
}

// Validate performs validation on the loaded configuration.
func Validate(cfg *models.BackupConfig) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	if cfg.Restic.Repository == "" {
		return fmt.Errorf("restic.repository is required")
	}

	if cfg.Restic.Password == "" {
		return fmt.Errorf("restic.password is required")
	}

	if len(cfg.Backup.Paths) == 0 {
		return fmt.Errorf("backup.paths is required")
	}

	return nil
}
