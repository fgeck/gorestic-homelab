package models

import "time"

// PostgresConfig holds PostgreSQL dump configuration.
type PostgresConfig struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	Format   string // "custom" (default), "plain", "tar"
}

// PostgresDumpResult holds the result of a pg_dump operation.
type PostgresDumpResult struct {
	OutputPath string
	SizeBytes  int64
	Duration   time.Duration
	Error      error
}
