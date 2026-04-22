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
	"github.com/fgeck/gorestic-homelab/internal/services/pushover"
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
	pushoverSvc pushover.Service
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
		pushoverSvc: pushover.New(logger),
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
	pushoverSvc pushover.Service,
	tempDir string,
) *Impl {
	return &Impl{
		resticSvc:   resticSvc,
		wolSvc:      wolSvc,
		postgresSvc: postgresSvc,
		sshSvc:      sshSvc,
		telegramSvc: telegramSvc,
		pushoverSvc: pushoverSvc,
		logger:      logger,
		tempDir:     tempDir,
	}
}

// Run executes the complete backup workflow.
//
//nolint:gocognit,gocyclo // backup workflow has multiple steps by design
func (s *Impl) Run(ctx context.Context, cfg models.BackupConfig) (returnErr error) {
	startTime := time.Now()
	var failedStep string
	wolAttempted := cfg.WOL != nil
	wolSucceeded := false

	// Track backup results for notification even if later steps fail
	var backupStats *models.BackupResult
	var forgetStats *models.ForgetResult

	s.logger.Info().
		Str("repository", cfg.Restic.Repository).
		Str("host", cfg.Backup.Host).
		Msg("starting backup run")

	// Send notification on exit if configured (registered first, runs last due to LIFO)
	defer func() {
		if cfg.Telegram != nil {
			s.sendNotificationWithStats(ctx, cfg, startTime, failedStep, returnErr, backupStats, forgetStats)
		}
		if cfg.Pushover != nil {
			s.sendPushoverNotification(ctx, cfg, startTime, failedStep, returnErr, backupStats, forgetStats)
		}
	}()

	// SSH shutdown runs on exit if configured and either:
	// - WOL was not configured (standalone SSH shutdown), or
	// - WOL was configured and succeeded (machine was woken up)
	// This ensures the target machine is shut down even if backup fails
	// (registered second, runs before Telegram notification)
	defer func() {
		shouldShutdown := cfg.SSHShutdown != nil && (!wolAttempted || wolSucceeded)
		if shouldShutdown {
			if err := s.runSSHShutdown(ctx, cfg.SSHShutdown); err != nil {
				s.logger.Error().Err(err).Msg("SSH shutdown failed")
				// Don't override returnErr if backup already failed
				if returnErr == nil {
					failedStep = "ssh_shutdown"
					returnErr = err
				}
			}
		}
	}()

	// Step 1: Wake-on-LAN (if configured)
	if cfg.WOL != nil {
		failedStep = "wol"
		if err := s.runWOL(ctx, cfg.WOL); err != nil {
			returnErr = err
			return err
		}
		wolSucceeded = true
	}

	// Step 2: Initialize repository (if needed)
	failedStep = "init"
	if err := s.resticSvc.Init(ctx, cfg.Restic); err != nil {
		returnErr = err
		return fmt.Errorf("init failed: %w", err)
	}

	// Step 3: Unlock repository (remove stale locks)
	failedStep = "unlock"
	if err := s.resticSvc.Unlock(ctx, cfg.Restic); err != nil {
		returnErr = err
		return fmt.Errorf("unlock failed: %w", err)
	}

	// Step 4: PostgreSQL dump (if configured)
	var pgDumpPath string
	if cfg.Postgres != nil {
		failedStep = "postgres"
		var err error
		pgDumpPath, err = s.runPostgresDump(ctx, cfg.Postgres)
		if err != nil {
			returnErr = err
			return err
		}
		defer func() { _ = os.Remove(pgDumpPath) }() // Clean up after backup
	}

	// Step 5: Backup
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
		returnErr = err
		return fmt.Errorf("backup failed: %w", err)
	}
	if backupResult.Error != nil {
		returnErr = backupResult.Error
		return fmt.Errorf("backup failed: %w", backupResult.Error)
	}

	// Store backup stats for notification (even if later steps fail)
	backupStats = backupResult

	// Step 6: Apply retention policy
	failedStep = "forget"
	forgetResult, err := s.resticSvc.Forget(ctx, cfg.Restic, cfg.Retention)
	if err != nil {
		returnErr = err
		return fmt.Errorf("forget failed: %w", err)
	}
	if forgetResult.Error != nil {
		returnErr = forgetResult.Error
		return fmt.Errorf("forget failed: %w", forgetResult.Error)
	}

	// Store forget stats for notification
	forgetStats = forgetResult

	// Step 7: Repository check (if enabled)
	if cfg.Check.Enabled {
		failedStep = "check"
		checkResult, err := s.resticSvc.Check(ctx, cfg.Restic, cfg.Check)
		if err != nil {
			returnErr = err
			return fmt.Errorf("check failed: %w", err)
		}
		if checkResult.Error != nil {
			returnErr = checkResult.Error
			return fmt.Errorf("check failed: %w", checkResult.Error)
		}
		if !checkResult.Passed {
			returnErr = fmt.Errorf("repository check failed")
			return returnErr
		}
	}

	// Success - clear failedStep
	failedStep = ""
	s.logger.Info().
		Str("duration", time.Since(startTime).Round(time.Millisecond).String()).
		Msg("backup run completed successfully")

	return nil
}

func (s *Impl) runWOL(ctx context.Context, cfg *models.WOLConfig) error {
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

	return nil
}

func (s *Impl) runPostgresDump(ctx context.Context, cfg *models.PostgresConfig) (string, error) {
	outputPath := filepath.Join(s.tempDir, postgres.GetOutputFilename(*cfg))

	result, err := s.postgresSvc.Dump(ctx, *cfg, outputPath)
	if err != nil {
		return "", fmt.Errorf("PostgreSQL dump failed: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("PostgreSQL dump failed: %w", result.Error)
	}

	return result.OutputPath, nil
}

func (s *Impl) runSSHShutdown(ctx context.Context, cfg *models.SSHShutdownConfig) error {
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
	}

	return nil
}

// notificationStats holds the common data used to build notification messages.
type notificationStats struct {
	success         bool
	host            string
	repository      string
	startTime       time.Time
	duration        time.Duration
	failedStep      string
	errorMessage    string
	snapshotID      string
	filesNew        int
	filesChanged    int
	filesUnmodified int
	dataAdded       int64
	totalFiles      int
	totalBytes      int64
	snapshotsKept   int
	snapshotsRemoved int
}

func buildStats(
	startTime time.Time,
	cfg models.BackupConfig,
	failedStep string,
	runErr error,
	backupStats *models.BackupResult,
	forgetStats *models.ForgetResult,
) notificationStats {
	s := notificationStats{
		success:    runErr == nil,
		host:       cfg.Backup.Host,
		repository: cfg.Restic.Repository,
		startTime:  startTime,
		duration:   time.Since(startTime),
	}
	if runErr != nil {
		s.failedStep = failedStep
		s.errorMessage = runErr.Error()
	}
	if backupStats != nil {
		s.snapshotID = backupStats.SnapshotID
		s.filesNew = backupStats.FilesNew
		s.filesChanged = backupStats.FilesChanged
		s.filesUnmodified = backupStats.FilesUnmodified
		s.dataAdded = backupStats.DataAdded
		s.totalFiles = backupStats.TotalFilesProcessed
		s.totalBytes = backupStats.TotalBytesProcessed
	}
	if forgetStats != nil {
		s.snapshotsKept = forgetStats.SnapshotsKept
		s.snapshotsRemoved = forgetStats.SnapshotsRemoved
	}
	return s
}

func (s *Impl) sendNotificationWithStats(
	ctx context.Context,
	cfg models.BackupConfig,
	startTime time.Time,
	failedStep string,
	runErr error,
	backupStats *models.BackupResult,
	forgetStats *models.ForgetResult,
) {
	ns := buildStats(startTime, cfg, failedStep, runErr, backupStats, forgetStats)

	// Collect backup stats for notification
	msg := models.TelegramMessage{
		Success:         ns.success,
		Host:            ns.host,
		Repository:      ns.repository,
		StartTime:       ns.startTime,
		Duration:        ns.duration,
		FailedStep:      ns.failedStep,
		ErrorMessage:    ns.errorMessage,
		SnapshotID:      ns.snapshotID,
		FilesNew:        ns.filesNew,
		FilesChanged:    ns.filesChanged,
		FilesUnmodified: ns.filesUnmodified,
		DataAdded:       ns.dataAdded,
		TotalFiles:      ns.totalFiles,
		TotalBytes:      ns.totalBytes,
		SnapshotsKept:   ns.snapshotsKept,
		SnapshotsRemoved: ns.snapshotsRemoved,
	}

	result, err := s.telegramSvc.SendNotification(ctx, *cfg.Telegram, msg)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to send Telegram notification")
		return
	}
	if result.Error != nil {
		s.logger.Error().Err(result.Error).Msg("failed to send Telegram notification")
	}
}

func (s *Impl) sendPushoverNotification(
	ctx context.Context,
	cfg models.BackupConfig,
	startTime time.Time,
	failedStep string,
	runErr error,
	backupStats *models.BackupResult,
	forgetStats *models.ForgetResult,
) {
	ns := buildStats(startTime, cfg, failedStep, runErr, backupStats, forgetStats)

	msg := models.PushoverMessage{
		Success:         ns.success,
		Host:            ns.host,
		Repository:      ns.repository,
		StartTime:       ns.startTime,
		Duration:        ns.duration,
		FailedStep:      ns.failedStep,
		ErrorMessage:    ns.errorMessage,
		SnapshotID:      ns.snapshotID,
		FilesNew:        ns.filesNew,
		FilesChanged:    ns.filesChanged,
		FilesUnmodified: ns.filesUnmodified,
		DataAdded:       ns.dataAdded,
		TotalFiles:      ns.totalFiles,
		TotalBytes:      ns.totalBytes,
		SnapshotsKept:   ns.snapshotsKept,
		SnapshotsRemoved: ns.snapshotsRemoved,
	}

	result, err := s.pushoverSvc.SendNotification(ctx, *cfg.Pushover, msg)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to send Pushover notification")
		return
	}
	if result.Error != nil {
		s.logger.Error().Err(result.Error).Msg("failed to send Pushover notification")
	}
}
