// Package runner orchestrates the backup workflow.
package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/fgeck/gorestic-homelab/internal/services/postgres"
	"github.com/fgeck/gorestic-homelab/internal/services/restic"
	"github.com/fgeck/gorestic-homelab/internal/services/ssh"
	"github.com/fgeck/gorestic-homelab/internal/services/telegram"
	"github.com/fgeck/gorestic-homelab/internal/services/wol"
	"github.com/rs/zerolog"
)

// Service defines the interface for the backup runner.
type Service interface {
	Run(ctx context.Context, cfg models.BackupConfig) error
}

// Impl implements the runner Service interface.
type Impl struct {
	resticSvc   restic.Service
	wolSvc      wol.Service
	postgresSvc postgres.Service
	sshSvc      ssh.Service
	telegramSvc telegram.Service
	logger      zerolog.Logger
	tempDir     string
}

// New creates a new runner service.
func New(logger zerolog.Logger) *Impl {
	return &Impl{
		resticSvc:   restic.New(logger),
		wolSvc:      wol.New(logger),
		postgresSvc: postgres.New(logger),
		sshSvc:      ssh.New(logger),
		telegramSvc: telegram.New(logger),
		logger:      logger,
		tempDir:     os.TempDir(),
	}
}

// NewWithServices creates a new runner service with custom services (for testing).
func NewWithServices(
	logger zerolog.Logger,
	resticSvc restic.Service,
	wolSvc wol.Service,
	postgresSvc postgres.Service,
	sshSvc ssh.Service,
	telegramSvc telegram.Service,
	tempDir string,
) *Impl {
	return &Impl{
		resticSvc:   resticSvc,
		wolSvc:      wolSvc,
		postgresSvc: postgresSvc,
		sshSvc:      sshSvc,
		telegramSvc: telegramSvc,
		logger:      logger,
		tempDir:     tempDir,
	}
}

// Run executes the complete backup workflow.
//
//nolint:gocognit,gocyclo // backup workflow has multiple steps by design
func (s *Impl) Run(ctx context.Context, cfg models.BackupConfig) error {
	startTime := time.Now()
	var failedStep string
	var runErr error

	s.logger.Info().
		Str("repository", cfg.Restic.Repository).
		Str("host", cfg.Backup.Host).
		Msg("starting backup run")

	defer func() {
		// Send notification if configured
		if cfg.Telegram != nil {
			s.sendNotification(ctx, cfg, startTime, failedStep, runErr)
		}
	}()

	// Step 1: Wake-on-LAN (if configured)
	if cfg.WOL != nil {
		failedStep = "wol"
		if err := s.runWOL(ctx, cfg.WOL); err != nil {
			runErr = err
			return err
		}
	}

	// Step 2: Initialize repository (if needed)
	failedStep = "init"
	if err := s.resticSvc.Init(ctx, cfg.Restic); err != nil {
		runErr = err
		return fmt.Errorf("init failed: %w", err)
	}

	// Step 3: PostgreSQL dump (if configured)
	var pgDumpPath string
	if cfg.Postgres != nil {
		failedStep = "postgres"
		var err error
		pgDumpPath, err = s.runPostgresDump(ctx, cfg.Postgres)
		if err != nil {
			runErr = err
			return err
		}
		defer func() { _ = os.Remove(pgDumpPath) }() // Clean up after backup
	}

	// Step 4: Backup
	failedStep = "backup"
	backupPaths := cfg.Backup.Paths
	if pgDumpPath != "" {
		backupPaths = append(backupPaths, pgDumpPath)
	}

	backupResult, err := s.resticSvc.Backup(ctx, cfg.Restic, models.BackupSettings{
		Paths: backupPaths,
		Tags:  cfg.Backup.Tags,
		Host:  cfg.Backup.Host,
	})
	if err != nil {
		runErr = err
		return fmt.Errorf("backup failed: %w", err)
	}
	if backupResult.Error != nil {
		runErr = backupResult.Error
		return fmt.Errorf("backup failed: %w", backupResult.Error)
	}

	s.logger.Info().
		Str("snapshot_id", backupResult.SnapshotID).
		Int("files_new", backupResult.FilesNew).
		Int("files_changed", backupResult.FilesChanged).
		Int64("data_added", backupResult.DataAdded).
		Msg("backup completed")

	// Step 5: Apply retention policy
	failedStep = "forget"
	forgetResult, err := s.resticSvc.Forget(ctx, cfg.Restic, cfg.Retention)
	if err != nil {
		runErr = err
		return fmt.Errorf("forget failed: %w", err)
	}
	if forgetResult.Error != nil {
		runErr = forgetResult.Error
		return fmt.Errorf("forget failed: %w", forgetResult.Error)
	}

	s.logger.Info().
		Int("kept", forgetResult.SnapshotsKept).
		Int("removed", forgetResult.SnapshotsRemoved).
		Msg("retention policy applied")

	// Step 6: Repository check (if enabled)
	if cfg.Check.Enabled {
		failedStep = "check"
		checkResult, err := s.resticSvc.Check(ctx, cfg.Restic, cfg.Check)
		if err != nil {
			runErr = err
			return fmt.Errorf("check failed: %w", err)
		}
		if checkResult.Error != nil {
			runErr = checkResult.Error
			return fmt.Errorf("check failed: %w", checkResult.Error)
		}
		if !checkResult.Passed {
			runErr = fmt.Errorf("repository check failed")
			return runErr
		}

		s.logger.Info().
			Bool("passed", checkResult.Passed).
			Dur("duration", checkResult.Duration).
			Msg("repository check completed")
	}

	// Step 7: SSH shutdown (if configured)
	if cfg.SSHShutdown != nil {
		failedStep = "ssh_shutdown"
		if err := s.runSSHShutdown(ctx, cfg.SSHShutdown); err != nil {
			runErr = err
			return err
		}
	}

	// Success - clear failedStep
	failedStep = ""
	s.logger.Info().
		Dur("duration", time.Since(startTime)).
		Msg("backup run completed successfully")

	return nil
}

func (s *Impl) runWOL(ctx context.Context, cfg *models.WOLConfig) error {
	s.logger.Info().
		Str("mac", cfg.MACAddress).
		Str("target", cfg.PollURL).
		Msg("sending Wake-on-LAN packet")

	result, err := s.wolSvc.Wake(ctx, *cfg)
	if err != nil {
		return fmt.Errorf("WOL failed: %w", err)
	}
	if result.Error != nil {
		return fmt.Errorf("WOL failed: %w", result.Error)
	}

	if !result.TargetReady && cfg.PollURL != "" {
		return fmt.Errorf("target did not become ready after WOL")
	}

	s.logger.Info().
		Bool("packet_sent", result.PacketSent).
		Bool("target_ready", result.TargetReady).
		Dur("wait_duration", result.WaitDuration).
		Msg("WOL completed")

	return nil
}

func (s *Impl) runPostgresDump(ctx context.Context, cfg *models.PostgresConfig) (string, error) {
	outputPath := filepath.Join(s.tempDir, postgres.GetOutputFilename(*cfg))

	s.logger.Info().
		Str("database", cfg.Database).
		Str("output", outputPath).
		Msg("starting PostgreSQL dump")

	result, err := s.postgresSvc.Dump(ctx, *cfg, outputPath)
	if err != nil {
		return "", fmt.Errorf("PostgreSQL dump failed: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("PostgreSQL dump failed: %w", result.Error)
	}

	s.logger.Info().
		Str("output", result.OutputPath).
		Int64("size", result.SizeBytes).
		Dur("duration", result.Duration).
		Msg("PostgreSQL dump completed")

	return result.OutputPath, nil
}

func (s *Impl) runSSHShutdown(ctx context.Context, cfg *models.SSHShutdownConfig) error {
	s.logger.Info().
		Str("host", cfg.Host).
		Int("delay", cfg.ShutdownDelay).
		Msg("initiating remote shutdown")

	// Load private key if needed
	if cfg.PrivateKey == nil && cfg.KeyPath != "" {
		key, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return fmt.Errorf("failed to read SSH key: %w", err)
		}
		cfg.PrivateKey = key
	}

	result, err := s.sshSvc.Shutdown(ctx, *cfg)
	if err != nil {
		return fmt.Errorf("SSH shutdown failed: %w", err)
	}
	if result.Error != nil {
		// SSH shutdown might return error due to connection closing
		// Only treat as error if command wasn't run
		if !result.CommandRun {
			return fmt.Errorf("SSH shutdown failed: %w", result.Error)
		}
		s.logger.Warn().
			Err(result.Error).
			Str("output", result.Output).
			Msg("shutdown command returned error (may be expected)")
	}

	s.logger.Info().
		Bool("command_run", result.CommandRun).
		Str("output", result.Output).
		Msg("SSH shutdown command sent")

	return nil
}

func (s *Impl) sendNotification(
	ctx context.Context,
	cfg models.BackupConfig,
	startTime time.Time,
	failedStep string,
	runErr error,
) {
	// Collect backup stats for notification
	msg := models.TelegramMessage{
		Success:    runErr == nil,
		Host:       cfg.Backup.Host,
		Repository: cfg.Restic.Repository,
		StartTime:  startTime,
		Duration:   time.Since(startTime),
	}

	if runErr != nil {
		msg.FailedStep = failedStep
		msg.ErrorMessage = runErr.Error()
	}

	// If backup was successful, try to get snapshot info
	if runErr == nil {
		snapshots, err := s.resticSvc.Snapshots(ctx, cfg.Restic)
		if err == nil && len(snapshots) > 0 {
			// Get the most recent snapshot
			latest := snapshots[len(snapshots)-1]
			msg.SnapshotID = latest.ID
		}
	}

	result, err := s.telegramSvc.SendNotification(ctx, *cfg.Telegram, msg)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to send Telegram notification")
		return
	}
	if result.Error != nil {
		s.logger.Error().Err(result.Error).Msg("failed to send Telegram notification")
		return
	}

	s.logger.Info().Msg("Telegram notification sent")
}
