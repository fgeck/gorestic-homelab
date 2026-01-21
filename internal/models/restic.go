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

// BackupProgress for restic status messages during backup.
type BackupProgress struct {
	MessageType  string   `json:"message_type"`
	PercentDone  float64  `json:"percent_done"`
	TotalFiles   uint64   `json:"total_files"`
	FilesDone    uint64   `json:"files_done"`
	TotalBytes   uint64   `json:"total_bytes"`
	BytesDone    uint64   `json:"bytes_done"`
	CurrentFiles []string `json:"current_files"`
}

// ResticProgressCallback for backup progress updates.
type ResticProgressCallback func(progress BackupProgress)
