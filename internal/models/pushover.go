package models

import "time"

// PushoverConfig holds Pushover notification configuration.
type PushoverConfig struct {
	AppToken string
	UserKey  string
	Priority int
}

// PushoverMessage holds the data for a backup notification.
type PushoverMessage struct {
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

// PushoverResult holds the result of a Pushover notification.
type PushoverResult struct {
	MessageSent bool
	Error       error
}
