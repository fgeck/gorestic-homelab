package models

import "time"

// TelegramConfig holds Telegram notification configuration.
type TelegramConfig struct {
	BotToken string
	ChatID   string
}

// TelegramMessage holds the data for a backup notification.
type TelegramMessage struct {
	Success    bool
	Host       string
	Repository string
	StartTime  time.Time
	Duration   time.Duration

	// Backup stats (if successful).
	SnapshotID      string
	FilesNew        int
	FilesChanged    int
	FilesUnmodified int
	DataAdded       int64
	TotalFiles      int
	TotalBytes      int64

	// Retention stats.
	SnapshotsRemoved int
	SnapshotsKept    int

	// Error info (if failed).
	ErrorMessage string
	FailedStep   string
}

// TelegramResult holds the result of a Telegram notification.
type TelegramResult struct {
	MessageSent bool
	Error       error
}
