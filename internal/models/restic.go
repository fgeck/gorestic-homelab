package models

import "time"

// BackupResult holds the result of a backup operation.
type BackupResult struct {
	SnapshotID          string
	FilesNew            int
	FilesChanged        int
	FilesUnmodified     int
	DataAdded           int64
	TotalFilesProcessed int
	TotalBytesProcessed int64
	Duration            time.Duration
	Error               error
}

// ForgetResult holds the result of a forget operation.
type ForgetResult struct {
	SnapshotsRemoved int
	SnapshotsKept    int
	SpaceFreed       int64
	Duration         time.Duration
	Error            error
}

// CheckResult holds the result of a repository check.
type CheckResult struct {
	Passed   bool
	Duration time.Duration
	Error    error
}

// Snapshot represents a restic snapshot.
type Snapshot struct {
	ID       string
	Time     time.Time
	Hostname string
	Tags     []string
	Paths    []string
}
