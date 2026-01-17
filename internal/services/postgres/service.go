// Package postgres provides PostgreSQL dump operations.
package postgres

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
)

// PostgreSQL dump format constants.
const (
	FormatPlain = "plain"
	FormatTar   = "tar"
)

// Service defines the interface for PostgreSQL dump operations.
type Service interface {
	Dump(ctx context.Context, cfg models.PostgresConfig, outputPath string) (*models.PostgresDumpResult, error)
}

// CommandExecutor allows mocking exec.Command in tests.
type CommandExecutor interface {
	ExecuteWithEnv(ctx context.Context, env []string, outputPath string, name string, args ...string) error
}

// DefaultExecutor is the default command executor using os/exec.
type DefaultExecutor struct{}

// ExecuteWithEnv runs pg_dump and writes output to the specified file.
func (e *DefaultExecutor) ExecuteWithEnv(ctx context.Context, env []string, outputPath string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)

	output, err := os.Create(outputPath) //nolint:gosec // outputPath is controlled by caller
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = output.Close() }()

	cmd.Stdout = output

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	return nil
}

// Impl implements the PostgreSQL Service interface.
type Impl struct {
	executor CommandExecutor
	logger   zerolog.Logger
}

// New creates a new PostgreSQL service.
func New(logger zerolog.Logger) *Impl {
	return &Impl{
		executor: &DefaultExecutor{},
		logger:   logger,
	}
}

// NewWithExecutor creates a new PostgreSQL service with a custom executor (for testing).
func NewWithExecutor(logger zerolog.Logger, executor CommandExecutor) *Impl {
	return &Impl{
		executor: executor,
		logger:   logger,
	}
}

// Dump performs a pg_dump operation.
func (s *Impl) Dump(ctx context.Context, cfg models.PostgresConfig, outputPath string) (*models.PostgresDumpResult, error) {
	s.logger.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("database", cfg.Database).
		Str("format", cfg.Format).
		Str("output", outputPath).
		Msg("starting PostgreSQL dump")

	start := time.Now()
	result := &models.PostgresDumpResult{
		OutputPath: outputPath,
	}

	// Ensure output directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		result.Error = fmt.Errorf("failed to create output directory: %w", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Build pg_dump arguments
	args := []string{
		"-h", cfg.Host,
		"-p", fmt.Sprintf("%d", cfg.Port),
		"-U", cfg.Username,
		"-d", cfg.Database,
	}

	// Add format flag
	switch cfg.Format {
	case "custom":
		args = append(args, "-Fc")
	case FormatPlain:
		args = append(args, "-Fp")
	case FormatTar:
		args = append(args, "-Ft")
	default:
		args = append(args, "-Fc") // Default to custom
	}

	// Set environment for password
	env := []string{}
	if cfg.Password != "" {
		env = append(env, fmt.Sprintf("PGPASSWORD=%s", cfg.Password))
	}

	// Execute pg_dump
	if execErr := s.executor.ExecuteWithEnv(ctx, env, outputPath, "pg_dump", args...); execErr != nil {
		// Clean up partial file
		_ = os.Remove(outputPath)
		result.Error = execErr
		result.Duration = time.Since(start)
		return result, nil //nolint:nilerr // error is stored in result struct by design
	}

	// Get file size
	if info, err := os.Stat(outputPath); err == nil {
		result.SizeBytes = info.Size()
	}

	result.Duration = time.Since(start)

	s.logger.Info().
		Str("output", outputPath).
		Int64("size_bytes", result.SizeBytes).
		Dur("duration", result.Duration).
		Msg("PostgreSQL dump completed")

	return result, nil
}

// GetOutputFilename returns a suggested output filename based on config.
func GetOutputFilename(cfg models.PostgresConfig) string {
	timestamp := time.Now().Format("20060102-150405")
	ext := "dump"

	switch cfg.Format {
	case FormatPlain:
		ext = "sql"
	case FormatTar:
		ext = FormatTar
	}

	return fmt.Sprintf("%s-%s.%s", cfg.Database, timestamp, ext)
}
