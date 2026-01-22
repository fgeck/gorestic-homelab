// Package models contains the data structures used throughout gorestic-homelab.
package models

// BackupConfig holds the complete configuration for a backup run.
type BackupConfig struct {
	Restic      ResticConfig
	Backup      BackupSettings
	Retention   RetentionPolicy
	Check       CheckSettings
	WOL         *WOLConfig         // nil if not configured
	Postgres    *PostgresConfig    // nil if not configured
	SSHShutdown *SSHShutdownConfig // nil if not configured
	Telegram    *TelegramConfig    // nil if not configured
}

// ResticConfig holds restic repository configuration.
type ResticConfig struct {
	Repository   string
	Password     string
	RestUser     string // optional, for REST server auth
	RestPassword string // optional, for REST server auth
	FailOnLocked bool   // if true (default), fail when locks exist; if false, remove locks and continue
}

// BackupSettings holds backup-specific settings.
type BackupSettings struct {
	Paths []string
	Tags  []string
	Host  string
}

// RetentionPolicy defines how many snapshots to keep.
type RetentionPolicy struct {
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
}

// CheckSettings defines repository check behavior.
type CheckSettings struct {
	Enabled bool
	Subset  string // e.g., "1%"
}
