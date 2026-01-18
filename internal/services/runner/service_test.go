package runner

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations.
type mockResticService struct {
	initFunc      func(ctx context.Context, cfg models.ResticConfig) error
	snapshotsFunc func(ctx context.Context, cfg models.ResticConfig) ([]models.Snapshot, error)
	backupFunc    func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error)
	forgetFunc    func(ctx context.Context, cfg models.ResticConfig, policy models.RetentionPolicy) (*models.ForgetResult, error)
	checkFunc     func(ctx context.Context, cfg models.ResticConfig, settings models.CheckSettings) (*models.CheckResult, error)
}

func (m *mockResticService) Init(ctx context.Context, cfg models.ResticConfig) error {
	if m.initFunc != nil {
		return m.initFunc(ctx, cfg)
	}
	return nil
}

func (m *mockResticService) Snapshots(ctx context.Context, cfg models.ResticConfig) ([]models.Snapshot, error) {
	if m.snapshotsFunc != nil {
		return m.snapshotsFunc(ctx, cfg)
	}
	return []models.Snapshot{}, nil
}

func (m *mockResticService) Backup(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
	if m.backupFunc != nil {
		return m.backupFunc(ctx, cfg, settings)
	}
	return &models.BackupResult{SnapshotID: "test123"}, nil
}

func (m *mockResticService) Forget(ctx context.Context, cfg models.ResticConfig, policy models.RetentionPolicy) (*models.ForgetResult, error) {
	if m.forgetFunc != nil {
		return m.forgetFunc(ctx, cfg, policy)
	}
	return &models.ForgetResult{SnapshotsKept: 5, SnapshotsRemoved: 2}, nil
}

func (m *mockResticService) Check(ctx context.Context, cfg models.ResticConfig, settings models.CheckSettings) (*models.CheckResult, error) {
	if m.checkFunc != nil {
		return m.checkFunc(ctx, cfg, settings)
	}
	return &models.CheckResult{Passed: true}, nil
}

type mockWOLService struct {
	wakeFunc func(ctx context.Context, cfg models.WOLConfig) (*models.WOLResult, error)
}

func (m *mockWOLService) Wake(ctx context.Context, cfg models.WOLConfig) (*models.WOLResult, error) {
	if m.wakeFunc != nil {
		return m.wakeFunc(ctx, cfg)
	}
	return &models.WOLResult{PacketSent: true, TargetReady: true}, nil
}

type mockPostgresService struct {
	dumpFunc func(ctx context.Context, cfg models.PostgresConfig, outputPath string) (*models.PostgresDumpResult, error)
}

func (m *mockPostgresService) Dump(ctx context.Context, cfg models.PostgresConfig, outputPath string) (*models.PostgresDumpResult, error) {
	if m.dumpFunc != nil {
		return m.dumpFunc(ctx, cfg, outputPath)
	}
	return &models.PostgresDumpResult{OutputPath: outputPath, SizeBytes: 1024}, nil
}

type mockSSHService struct {
	shutdownFunc       func(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error)
	testConnectionFunc func(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error)
}

func (m *mockSSHService) Shutdown(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
	if m.shutdownFunc != nil {
		return m.shutdownFunc(ctx, cfg)
	}
	return &models.SSHResult{CommandRun: true}, nil
}

func (m *mockSSHService) TestConnection(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
	if m.testConnectionFunc != nil {
		return m.testConnectionFunc(ctx, cfg)
	}
	return &models.SSHResult{CommandRun: true, Output: "OK"}, nil
}

type mockTelegramService struct {
	sendFunc func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error)
}

func (m *mockTelegramService) SendNotification(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error) {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, cfg, msg)
	}
	return &models.TelegramResult{MessageSent: true}, nil
}

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
	resticSvc := &mockResticService{}
	wolSvc := &mockWOLService{}
	postgresSvc := &mockPostgresService{}
	sshSvc := &mockSSHService{}
	telegramSvc := &mockTelegramService{}

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
	wolCalled := false
	resticSvc := &mockResticService{}
	wolSvc := &mockWOLService{
		wakeFunc: func(ctx context.Context, cfg models.WOLConfig) (*models.WOLResult, error) {
			wolCalled = true
			return &models.WOLResult{PacketSent: true, TargetReady: true}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		wolSvc,
		&mockPostgresService{},
		&mockSSHService{},
		&mockTelegramService{},
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
	assert.True(t, wolCalled)
}

func TestRun_WOLFailure(t *testing.T) {
	wolSvc := &mockWOLService{
		wakeFunc: func(ctx context.Context, cfg models.WOLConfig) (*models.WOLResult, error) {
			return &models.WOLResult{Error: errors.New("timeout")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		&mockResticService{},
		wolSvc,
		&mockPostgresService{},
		&mockSSHService{},
		&mockTelegramService{},
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
	postgresCalled := false
	var capturedPaths []string

	resticSvc := &mockResticService{
		backupFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
			capturedPaths = settings.Paths
			return &models.BackupResult{SnapshotID: "test"}, nil
		},
	}

	postgresSvc := &mockPostgresService{
		dumpFunc: func(ctx context.Context, cfg models.PostgresConfig, outputPath string) (*models.PostgresDumpResult, error) {
			postgresCalled = true
			return &models.PostgresDumpResult{OutputPath: outputPath, SizeBytes: 1024}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		postgresSvc,
		&mockSSHService{},
		&mockTelegramService{},
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
	assert.True(t, postgresCalled)
	// Should have original path plus pg dump path
	assert.Len(t, capturedPaths, 2)
}

func TestRun_PostgresDumpFailure(t *testing.T) {
	postgresSvc := &mockPostgresService{
		dumpFunc: func(ctx context.Context, cfg models.PostgresConfig, outputPath string) (*models.PostgresDumpResult, error) {
			return &models.PostgresDumpResult{Error: errors.New("connection refused")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		&mockResticService{},
		&mockWOLService{},
		postgresSvc,
		&mockSSHService{},
		&mockTelegramService{},
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
	resticSvc := &mockResticService{
		backupFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
			return &models.BackupResult{Error: errors.New("disk full")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
		&mockTelegramService{},
		t.TempDir(),
	)

	err := runner.Run(context.Background(), minimalConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "backup failed")
}

func TestRun_ForgetFailure(t *testing.T) {
	resticSvc := &mockResticService{
		forgetFunc: func(ctx context.Context, cfg models.ResticConfig, policy models.RetentionPolicy) (*models.ForgetResult, error) {
			return &models.ForgetResult{Error: errors.New("prune failed")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
		&mockTelegramService{},
		t.TempDir(),
	)

	err := runner.Run(context.Background(), minimalConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forget failed")
}

func TestRun_WithCheck(t *testing.T) {
	checkCalled := false
	resticSvc := &mockResticService{
		checkFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.CheckSettings) (*models.CheckResult, error) {
			checkCalled = true
			return &models.CheckResult{Passed: true}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
		&mockTelegramService{},
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Check = models.CheckSettings{
		Enabled: true,
		Subset:  "5%",
	}

	err := runner.Run(context.Background(), cfg)

	assert.NoError(t, err)
	assert.True(t, checkCalled)
}

func TestRun_CheckFailure(t *testing.T) {
	resticSvc := &mockResticService{
		checkFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.CheckSettings) (*models.CheckResult, error) {
			return &models.CheckResult{Passed: false, Error: errors.New("corruption detected")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
		&mockTelegramService{},
		t.TempDir(),
	)

	cfg := minimalConfig()
	cfg.Check = models.CheckSettings{Enabled: true}

	err := runner.Run(context.Background(), cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "check failed")
}

func TestRun_WithSSHShutdown(t *testing.T) {
	sshCalled := false
	sshSvc := &mockSSHService{
		shutdownFunc: func(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
			sshCalled = true
			return &models.SSHResult{CommandRun: true}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		&mockResticService{},
		&mockWOLService{},
		&mockPostgresService{},
		sshSvc,
		&mockTelegramService{},
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
	assert.True(t, sshCalled)
}

func TestRun_SSHShutdownFailure(t *testing.T) {
	sshCalled := false
	sshSvc := &mockSSHService{
		shutdownFunc: func(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
			sshCalled = true
			return &models.SSHResult{CommandRun: false, Error: errors.New("connection refused")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		&mockResticService{},
		&mockWOLService{},
		&mockPostgresService{},
		sshSvc,
		&mockTelegramService{},
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
	assert.True(t, sshCalled)
}

func TestRun_SSHShutdownRunsOnBackupFailure(t *testing.T) {
	// When WOL succeeds but backup fails, SSH shutdown should still run
	sshCalled := false
	sshSvc := &mockSSHService{
		shutdownFunc: func(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
			sshCalled = true
			return &models.SSHResult{CommandRun: true}, nil
		},
	}

	resticSvc := &mockResticService{
		backupFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
			return &models.BackupResult{Error: errors.New("backup failed")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		sshSvc,
		&mockTelegramService{},
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
	assert.True(t, sshCalled, "SSH shutdown should run even when backup fails")
}

func TestRun_SSHShutdownSkippedWhenWOLFails(t *testing.T) {
	// When WOL fails, SSH shutdown should NOT run (machine wasn't woken up)
	sshCalled := false
	sshSvc := &mockSSHService{
		shutdownFunc: func(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
			sshCalled = true
			return &models.SSHResult{CommandRun: true}, nil
		},
	}

	wolSvc := &mockWOLService{
		wakeFunc: func(ctx context.Context, cfg models.WOLConfig) (*models.WOLResult, error) {
			return &models.WOLResult{Error: errors.New("WOL failed")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		&mockResticService{},
		wolSvc,
		&mockPostgresService{},
		sshSvc,
		&mockTelegramService{},
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
	assert.False(t, sshCalled, "SSH shutdown should NOT run when WOL fails")
}

func TestRun_WithTelegram_Success(t *testing.T) {
	var capturedMsg models.TelegramMessage
	telegramSvc := &mockTelegramService{
		sendFunc: func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error) {
			capturedMsg = msg
			return &models.TelegramResult{MessageSent: true}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		&mockResticService{},
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
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
	var capturedMsg models.TelegramMessage
	telegramSvc := &mockTelegramService{
		sendFunc: func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error) {
			capturedMsg = msg
			return &models.TelegramResult{MessageSent: true}, nil
		},
	}

	resticSvc := &mockResticService{
		backupFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
			return &models.BackupResult{Error: errors.New("backup failed")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
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
	resticSvc := &mockResticService{
		initFunc: func(ctx context.Context, cfg models.ResticConfig) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
		&mockTelegramService{},
		t.TempDir(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := runner.Run(ctx, minimalConfig())

	assert.Error(t, err)
}

func TestRun_TelegramIncludesBackupStatsOnSSHFailure(t *testing.T) {
	// When backup succeeds but SSH shutdown fails, Telegram should include backup stats
	var capturedMsg models.TelegramMessage
	telegramSvc := &mockTelegramService{
		sendFunc: func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error) {
			capturedMsg = msg
			return &models.TelegramResult{MessageSent: true}, nil
		},
	}

	resticSvc := &mockResticService{
		backupFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
			return &models.BackupResult{
				SnapshotID:          "abc123",
				FilesNew:            10,
				FilesChanged:        5,
				FilesUnmodified:     100,
				DataAdded:           1024 * 1024, // 1 MiB
				TotalFilesProcessed: 115,
				TotalBytesProcessed: 10 * 1024 * 1024,
			}, nil
		},
	}

	sshSvc := &mockSSHService{
		shutdownFunc: func(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
			return &models.SSHResult{CommandRun: false, Error: errors.New("connection refused")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
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
	var capturedMsg models.TelegramMessage
	telegramSvc := &mockTelegramService{
		sendFunc: func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error) {
			capturedMsg = msg
			return &models.TelegramResult{MessageSent: true}, nil
		},
	}

	resticSvc := &mockResticService{
		backupFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
			return &models.BackupResult{
				SnapshotID: "snap123",
				FilesNew:   20,
				DataAdded:  2048,
			}, nil
		},
		forgetFunc: func(ctx context.Context, cfg models.ResticConfig, retention models.RetentionPolicy) (*models.ForgetResult, error) {
			return &models.ForgetResult{
				SnapshotsKept:    5,
				SnapshotsRemoved: 2,
			}, nil
		},
		checkFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.CheckSettings) (*models.CheckResult, error) {
			return &models.CheckResult{Passed: false, Error: errors.New("repository corrupted")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
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
	var capturedMsg models.TelegramMessage
	telegramSvc := &mockTelegramService{
		sendFunc: func(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error) {
			capturedMsg = msg
			return &models.TelegramResult{MessageSent: true}, nil
		},
	}

	resticSvc := &mockResticService{
		backupFunc: func(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
			return &models.BackupResult{Error: errors.New("backup failed")}, nil
		},
	}

	runner := NewWithServices(
		testLogger(),
		resticSvc,
		&mockWOLService{},
		&mockPostgresService{},
		&mockSSHService{},
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
