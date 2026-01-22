package runner

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	postgresmocks "github.com/fgeck/gorestic-homelab/internal/services/postgres/mocks"
	resticmocks "github.com/fgeck/gorestic-homelab/internal/services/restic/mocks"
	sshmocks "github.com/fgeck/gorestic-homelab/internal/services/ssh/mocks"
	telegrammocks "github.com/fgeck/gorestic-homelab/internal/services/telegram/mocks"
	wolmocks "github.com/fgeck/gorestic-homelab/internal/services/wol/mocks"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func minimalConfig() models.BackupConfig {
	return models.BackupConfig{
		Restic: models.ResticConfig{
			Repository: "/backup",
			Password:   "secret",
		},
		Backup: models.BackupSettings{
			Paths: []string{"/data"},
			Host:  "testhost",
		},
		Retention: models.RetentionPolicy{
			KeepDaily:   7,
			KeepWeekly:  4,
			KeepMonthly: 6,
		},
	}
}

func TestRun_Success_MinimalConfig(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// Set up expectations for minimal config
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{SnapshotID: "test123"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	err := runner.Run(context.Background(), minimalConfig())

	assert.NoError(t, err)
}

func TestRun_WithWOL(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// WOL should be called
	wolSvc.EXPECT().Wake(mock.Anything, mock.Anything).Return(&models.WOLResult{PacketSent: true, TargetReady: true}, nil)

	// Standard restic operations
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{SnapshotID: "test"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.WOL = &models.WOLConfig{
		MACAddress:   "AA:BB:CC:DD:EE:FF",
		BroadcastIP:  "255.255.255.255",
		Timeout:      5 * time.Minute,
		PollInterval: 10 * time.Second,
	}

	err := runner.Run(context.Background(), cfg)

	assert.NoError(t, err)
}

func TestRun_WOLFailure(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// WOL fails
	wolSvc.EXPECT().Wake(mock.Anything, mock.Anything).Return(&models.WOLResult{Error: errors.New("timeout")}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.WOL = &models.WOLConfig{
		MACAddress: "AA:BB:CC:DD:EE:FF",
	}

	err := runner.Run(context.Background(), cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WOL failed")
}

func TestRun_WithPostgres(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	var capturedPaths []string

	// Postgres dump should be called
	postgresSvc.EXPECT().Dump(mock.Anything, mock.Anything, mock.Anything).Return(&models.PostgresDumpResult{OutputPath: "/tmp/dump.sql", SizeBytes: 1024}, nil)

	// Standard restic operations
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) {
		capturedPaths = settings.Paths
	}).Return(&models.BackupResult{SnapshotID: "test"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Postgres = &models.PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		Username: "postgres",
		Format:   "custom",
	}

	err := runner.Run(context.Background(), cfg)

	assert.NoError(t, err)
	// Should have original path plus pg dump path
	assert.Len(t, capturedPaths, 2)
}

func TestRun_PostgresDumpFailure(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// Init and unlock succeed, but postgres dump fails
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	postgresSvc.EXPECT().Dump(mock.Anything, mock.Anything, mock.Anything).Return(&models.PostgresDumpResult{Error: errors.New("connection refused")}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Postgres = &models.PostgresConfig{
		Database: "testdb",
	}

	err := runner.Run(context.Background(), cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PostgreSQL dump failed")
}

func TestRun_BackupFailure(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// Init and unlock succeed, backup fails
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{Error: errors.New("disk full")}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	err := runner.Run(context.Background(), minimalConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "backup failed")
}

func TestRun_ForgetFailure(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// Backup succeeds, forget fails
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{SnapshotID: "test"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{Error: errors.New("prune failed")}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	err := runner.Run(context.Background(), minimalConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forget failed")
}

func TestRun_WithCheck(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// All operations succeed including check
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{SnapshotID: "test"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)
	resticSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(&models.CheckResult{Passed: true}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Check = models.CheckSettings{
		Enabled: true,
		Subset:  "5%",
	}

	err := runner.Run(context.Background(), cfg)

	assert.NoError(t, err)
}

func TestRun_CheckFailure(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// Backup and forget succeed, check fails
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{SnapshotID: "test"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)
	resticSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(&models.CheckResult{Passed: false, Error: errors.New("corruption detected")}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Check = models.CheckSettings{Enabled: true}

	err := runner.Run(context.Background(), cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check failed")
}

func TestRun_WithSSHShutdown(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// All operations succeed including SSH shutdown
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{SnapshotID: "test"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)
	sshSvc.EXPECT().Shutdown(mock.Anything, mock.Anything).Return(&models.SSHResult{CommandRun: true}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.SSHShutdown = &models.SSHShutdownConfig{
		Host:          "192.168.1.100",
		Port:          22,
		Username:      "root",
		PrivateKey:    []byte("test-key"),
		ShutdownDelay: 1,
	}

	err := runner.Run(context.Background(), cfg)

	assert.NoError(t, err)
}

func TestRun_SSHShutdownFailure(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// Backup succeeds, SSH shutdown fails
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{SnapshotID: "test"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)
	sshSvc.EXPECT().Shutdown(mock.Anything, mock.Anything).Return(&models.SSHResult{CommandRun: false, Error: errors.New("connection refused")}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.SSHShutdown = &models.SSHShutdownConfig{
		Host:       "192.168.1.100",
		PrivateKey: []byte("test-key"),
	}

	err := runner.Run(context.Background(), cfg)

	// SSH shutdown runs in defer, failure is returned even if backup succeeded
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SSH shutdown failed")
}

func TestRun_SSHShutdownRunsOnBackupFailure(t *testing.T) {
	// When WOL succeeds but backup fails, SSH shutdown should still run
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// WOL succeeds
	wolSvc.EXPECT().Wake(mock.Anything, mock.Anything).Return(&models.WOLResult{PacketSent: true, TargetReady: true}, nil)

	// Backup fails
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{Error: errors.New("backup failed")}, nil)

	// SSH shutdown should still be called (deferred)
	sshSvc.EXPECT().Shutdown(mock.Anything, mock.Anything).Return(&models.SSHResult{CommandRun: true}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.WOL = &models.WOLConfig{
		MACAddress:  "00:11:22:33:44:55",
		BroadcastIP: "255.255.255.255",
	}
	cfg.SSHShutdown = &models.SSHShutdownConfig{
		Host:       "192.168.1.100",
		PrivateKey: []byte("test-key"),
	}

	err := runner.Run(context.Background(), cfg)

	// Backup failed, but SSH shutdown should still have been called
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "backup failed")
}

func TestRun_SSHShutdownSkippedWhenWOLFails(t *testing.T) {
	// When WOL fails, SSH shutdown should NOT run (machine wasn't woken up)
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// WOL fails
	wolSvc.EXPECT().Wake(mock.Anything, mock.Anything).Return(&models.WOLResult{Error: errors.New("WOL failed")}, nil)

	// SSH shutdown should NOT be called because WOL failed

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.WOL = &models.WOLConfig{
		MACAddress:  "00:11:22:33:44:55",
		BroadcastIP: "255.255.255.255",
	}
	cfg.SSHShutdown = &models.SSHShutdownConfig{
		Host:       "192.168.1.100",
		PrivateKey: []byte("test-key"),
	}

	err := runner.Run(context.Background(), cfg)

	// WOL failed, SSH shutdown should NOT be called
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WOL failed")
}

func TestRun_WithTelegram_Success(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	var capturedMsg models.TelegramMessage

	// Standard operations succeed
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{SnapshotID: "test"}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)

	// Telegram notification should be sent
	telegramSvc.EXPECT().SendNotification(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) {
		capturedMsg = msg
	}).Return(&models.TelegramResult{MessageSent: true}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Telegram = &models.TelegramConfig{
		BotToken: "123456:ABC",
		ChatID:   "-100123",
	}

	err := runner.Run(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, capturedMsg.Success)
	assert.Equal(t, "testhost", capturedMsg.Host)
	assert.Equal(t, "/backup", capturedMsg.Repository)
}

func TestRun_WithTelegram_Failure(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	var capturedMsg models.TelegramMessage

	// Backup fails
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{Error: errors.New("backup failed")}, nil)

	// Telegram notification should still be sent (with failure info)
	telegramSvc.EXPECT().SendNotification(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) {
		capturedMsg = msg
	}).Return(&models.TelegramResult{MessageSent: true}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Telegram = &models.TelegramConfig{
		BotToken: "123456:ABC",
		ChatID:   "-100123",
	}

	err := runner.Run(context.Background(), cfg)

	require.Error(t, err)
	assert.False(t, capturedMsg.Success)
	assert.Equal(t, "backup", capturedMsg.FailedStep)
	assert.Contains(t, capturedMsg.ErrorMessage, "backup failed")
}

func TestRun_ContextCancelled(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// Init returns context error
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(context.Canceled)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := runner.Run(ctx, minimalConfig())

	assert.Error(t, err)
}

func TestRun_UnlockFailure(t *testing.T) {
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	// Init succeeds, unlock fails (e.g., repository is locked and fail_on_locked is true)
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(errors.New("repository has 2 stale lock(s)"))

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	err := runner.Run(context.Background(), minimalConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unlock failed")
}

func TestRun_TelegramIncludesBackupStatsOnSSHFailure(t *testing.T) {
	// When backup succeeds but SSH shutdown fails, Telegram should include backup stats
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	var capturedMsg models.TelegramMessage

	// Backup succeeds with stats
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{
		SnapshotID:          "abc123",
		FilesNew:            10,
		FilesChanged:        5,
		FilesUnmodified:     100,
		DataAdded:           1024 * 1024, // 1 MiB
		TotalFilesProcessed: 115,
		TotalBytesProcessed: 10 * 1024 * 1024,
	}, nil)
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil)

	// SSH shutdown fails
	sshSvc.EXPECT().Shutdown(mock.Anything, mock.Anything).Return(&models.SSHResult{CommandRun: false, Error: errors.New("connection refused")}, nil)

	// Telegram should include backup stats even though SSH failed
	telegramSvc.EXPECT().SendNotification(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) {
		capturedMsg = msg
	}).Return(&models.TelegramResult{MessageSent: true}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Telegram = &models.TelegramConfig{
		BotToken: "123456:ABC",
		ChatID:   "-100123",
	}
	cfg.SSHShutdown = &models.SSHShutdownConfig{
		Host:       "192.168.1.100",
		PrivateKey: []byte("test-key"),
	}

	err := runner.Run(context.Background(), cfg)

	// SSH shutdown failed, but backup succeeded
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SSH shutdown failed")

	// Telegram message should include backup stats even though run failed
	assert.False(t, capturedMsg.Success)
	assert.Equal(t, "ssh_shutdown", capturedMsg.FailedStep)
	assert.Equal(t, "abc123", capturedMsg.SnapshotID)
	assert.Equal(t, 10, capturedMsg.FilesNew)
	assert.Equal(t, 5, capturedMsg.FilesChanged)
	assert.Equal(t, int64(1024*1024), capturedMsg.DataAdded)
}

func TestRun_TelegramIncludesForgetStatsOnCheckFailure(t *testing.T) {
	// When backup and forget succeed but check fails, Telegram should include both stats
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	var capturedMsg models.TelegramMessage

	// Backup succeeds
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{
		SnapshotID: "snap123",
		FilesNew:   20,
		DataAdded:  2048,
	}, nil)

	// Forget succeeds
	resticSvc.EXPECT().Forget(mock.Anything, mock.Anything, mock.Anything).Return(&models.ForgetResult{
		SnapshotsKept:    5,
		SnapshotsRemoved: 2,
	}, nil)

	// Check fails
	resticSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(&models.CheckResult{Passed: false, Error: errors.New("repository corrupted")}, nil)

	// Telegram should include backup and forget stats
	telegramSvc.EXPECT().SendNotification(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) {
		capturedMsg = msg
	}).Return(&models.TelegramResult{MessageSent: true}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Telegram = &models.TelegramConfig{
		BotToken: "123456:ABC",
		ChatID:   "-100123",
	}
	cfg.Check = models.CheckSettings{Enabled: true}

	err := runner.Run(context.Background(), cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check failed")

	// Telegram should include backup and forget stats
	assert.False(t, capturedMsg.Success)
	assert.Equal(t, "check", capturedMsg.FailedStep)
	assert.Equal(t, "snap123", capturedMsg.SnapshotID)
	assert.Equal(t, 20, capturedMsg.FilesNew)
	assert.Equal(t, 5, capturedMsg.SnapshotsKept)
	assert.Equal(t, 2, capturedMsg.SnapshotsRemoved)
}

func TestRun_TelegramNoBackupStatsOnBackupFailure(t *testing.T) {
	// When backup fails, Telegram should NOT include backup stats
	resticSvc := resticmocks.NewMockService(t)
	wolSvc := wolmocks.NewMockService(t)
	postgresSvc := postgresmocks.NewMockService(t)
	sshSvc := sshmocks.NewMockService(t)
	telegramSvc := telegrammocks.NewMockService(t)

	var capturedMsg models.TelegramMessage

	// Backup fails
	resticSvc.EXPECT().Init(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Unlock(mock.Anything, mock.Anything).Return(nil)
	resticSvc.EXPECT().Backup(mock.Anything, mock.Anything, mock.Anything).Return(&models.BackupResult{Error: errors.New("backup failed")}, nil)

	// Telegram should NOT include backup stats since backup failed
	telegramSvc.EXPECT().SendNotification(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) {
		capturedMsg = msg
	}).Return(&models.TelegramResult{MessageSent: true}, nil)

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		postgresSvc,
		sshSvc,
		telegramSvc,
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Telegram = &models.TelegramConfig{
		BotToken: "123456:ABC",
		ChatID:   "-100123",
	}

	err := runner.Run(context.Background(), cfg)

	assert.Error(t, err)

	// Telegram should NOT include backup stats since backup failed
	assert.False(t, capturedMsg.Success)
	assert.Equal(t, "backup", capturedMsg.FailedStep)
	assert.Empty(t, capturedMsg.SnapshotID)
	assert.Zero(t, capturedMsg.FilesNew)
	assert.Zero(t, capturedMsg.DataAdded)
}
