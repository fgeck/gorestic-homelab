package restic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor is a mock implementation of CommandExecutor for testing.
type mockExecutor struct {
	executeFunc                 func(ctx context.Context, name string, args ...string) ([]byte, error)
	executeWithEnvFunc          func(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
	executeWithEnvStreamingFunc func(ctx context.Context, env []string, progressCb models.ResticProgressCallback, name string, args ...string) ([]byte, error)
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, name, args...)
	}
	return nil, nil
}

func (m *mockExecutor) ExecuteWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	if m.executeWithEnvFunc != nil {
		return m.executeWithEnvFunc(ctx, env, name, args...)
	}
	return nil, nil
}

func (m *mockExecutor) ExecuteWithEnvStreaming(ctx context.Context, env []string, progressCb models.ResticProgressCallback, name string, args ...string) ([]byte, error) {
	if m.executeWithEnvStreamingFunc != nil {
		return m.executeWithEnvStreamingFunc(ctx, env, progressCb, name, args...)
	}
	// Fall back to non-streaming version for compatibility
	return m.ExecuteWithEnv(ctx, env, name, args...)
}

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func testConfig() models.ResticConfig {
	return models.ResticConfig{
		Repository: "/backup",
		Password:   "secret",
	}
}

func TestInit_AlreadyInitialized(t *testing.T) {
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			if name == "restic" && len(args) > 0 && args[0] == "snapshots" {
				return []byte("[]"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	err := svc.Init(context.Background(), testConfig())

	assert.NoError(t, err)
}

func TestInit_NewRepository(t *testing.T) {
	callCount := 0
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			callCount++
			if callCount == 1 {
				// First call: snapshots fails (repo not initialized)
				return nil, errors.New("repository does not exist")
			}
			// Second call: init succeeds
			return []byte("created restic repository"), nil
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	err := svc.Init(context.Background(), testConfig())

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestInit_FailedToInitialize(t *testing.T) {
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			return nil, errors.New("init failed")
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	err := svc.Init(context.Background(), testConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize repository")
}

func TestSnapshots_Success(t *testing.T) {
	now := time.Now()
	snaps := []snapshotJSON{
		{
			ID:       "abc123",
			Time:     now,
			Hostname: "server1",
			Tags:     []string{"daily"},
			Paths:    []string{"/data"},
		},
		{
			ID:       "def456",
			Time:     now.Add(-24 * time.Hour),
			Hostname: "server1",
			Tags:     []string{"weekly"},
			Paths:    []string{"/data", "/home"},
		},
	}

	snapsJSON, _ := json.Marshal(snaps)

	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			return snapsJSON, nil
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	result, err := svc.Snapshots(context.Background(), testConfig())

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "abc123", result[0].ID)
	assert.Equal(t, "server1", result[0].Hostname)
	assert.Equal(t, []string{"daily"}, result[0].Tags)
	assert.Equal(t, "def456", result[1].ID)
}

func TestSnapshots_Empty(t *testing.T) {
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			return []byte("[]"), nil
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	result, err := svc.Snapshots(context.Background(), testConfig())

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSnapshots_Error(t *testing.T) {
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			return nil, errors.New("connection refused")
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	_, err := svc.Snapshots(context.Background(), testConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list snapshots")
}

func TestBackup_Success(t *testing.T) {
	summary := `{"message_type":"summary","files_new":10,"files_changed":5,"files_unmodified":100,"data_added":1048576,"total_files_processed":115,"total_bytes_processed":10485760,"snapshot_id":"abc123def456"}`

	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			// Verify args contain expected values
			assert.Contains(t, args, "backup")
			assert.Contains(t, args, "--json")
			return []byte(summary), nil
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	settings := models.BackupSettings{
		Paths: []string{"/data"},
		Tags:  []string{"daily"},
		Host:  "myserver",
	}

	result, err := svc.Backup(context.Background(), testConfig(), settings)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Error)
	assert.Equal(t, "abc123def456", result.SnapshotID)
	assert.Equal(t, 10, result.FilesNew)
	assert.Equal(t, 5, result.FilesChanged)
	assert.Equal(t, 100, result.FilesUnmodified)
	assert.Equal(t, int64(1048576), result.DataAdded)
	assert.Equal(t, 115, result.TotalFilesProcessed)
	assert.Equal(t, int64(10485760), result.TotalBytesProcessed)
}

func TestBackup_WithTags(t *testing.T) {
	var capturedArgs []string
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte(`{"message_type":"summary","snapshot_id":"test"}`), nil
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	settings := models.BackupSettings{
		Paths: []string{"/data"},
		Tags:  []string{"daily", "important"},
		Host:  "server",
	}

	_, err := svc.Backup(context.Background(), testConfig(), settings)

	require.NoError(t, err)
	assert.Contains(t, capturedArgs, "--tag")
	// Count tag occurrences (should be 2, one for each tag)
	tagCount := 0
	for _, arg := range capturedArgs {
		if arg == "--tag" {
			tagCount++
		}
	}
	assert.Equal(t, 2, tagCount)
}

func TestBackup_Error(t *testing.T) {
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			return []byte("error: cannot read file"), errors.New("exit status 1")
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	settings := models.BackupSettings{
		Paths: []string{"/nonexistent"},
	}

	result, err := svc.Backup(context.Background(), testConfig(), settings)

	// Backup returns result with error in Error field, not as function return
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "backup failed")
}

func TestForget_Success(t *testing.T) {
	output := `[{"keep":[{"id":"snap1"},{"id":"snap2"}],"remove":[{"id":"snap3"}]}]`

	var capturedArgs []string
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte(output), nil
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	policy := models.RetentionPolicy{
		KeepDaily:   7,
		KeepWeekly:  4,
		KeepMonthly: 6,
	}

	result, err := svc.Forget(context.Background(), testConfig(), policy)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Error)
	assert.Equal(t, 2, result.SnapshotsKept)
	assert.Equal(t, 1, result.SnapshotsRemoved)

	// Verify arguments
	assert.Contains(t, capturedArgs, "forget")
	assert.Contains(t, capturedArgs, "--prune")
	assert.Contains(t, capturedArgs, "--keep-daily")
	assert.Contains(t, capturedArgs, "7")
	assert.Contains(t, capturedArgs, "--keep-weekly")
	assert.Contains(t, capturedArgs, "4")
	assert.Contains(t, capturedArgs, "--keep-monthly")
	assert.Contains(t, capturedArgs, "6")
}

func TestForget_Error(t *testing.T) {
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			return nil, errors.New("forget failed")
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	policy := models.RetentionPolicy{KeepDaily: 7}

	result, err := svc.Forget(context.Background(), testConfig(), policy)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "forget failed")
}

func TestCheck_Disabled(t *testing.T) {
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			t.Fatal("should not be called when check is disabled")
			return nil, nil
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	settings := models.CheckSettings{Enabled: false}

	result, err := svc.Check(context.Background(), testConfig(), settings)

	require.NoError(t, err)
	assert.True(t, result.Passed)
}

func TestCheck_Success(t *testing.T) {
	var capturedArgs []string
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("no errors were found"), nil
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	settings := models.CheckSettings{
		Enabled: true,
		Subset:  "5%",
	}

	result, err := svc.Check(context.Background(), testConfig(), settings)

	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Nil(t, result.Error)

	// Verify arguments
	assert.Contains(t, capturedArgs, "check")
	assert.Contains(t, capturedArgs, "--read-data-subset")
	assert.Contains(t, capturedArgs, "5%")
}

func TestCheck_Error(t *testing.T) {
	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			return []byte("error: repository corrupted"), errors.New("exit status 1")
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	settings := models.CheckSettings{Enabled: true}

	result, err := svc.Check(context.Background(), testConfig(), settings)

	require.NoError(t, err)
	assert.False(t, result.Passed)
	assert.NotNil(t, result.Error)
}

func TestBuildEnv(t *testing.T) {
	svc := New(testLogger())

	tests := []struct {
		name     string
		cfg      models.ResticConfig
		expected []string
	}{
		{
			name: "basic config",
			cfg: models.ResticConfig{
				Repository: "/backup",
				Password:   "secret",
			},
			expected: []string{
				"RESTIC_REPOSITORY=/backup",
				"RESTIC_PASSWORD=secret",
			},
		},
		{
			name: "with REST auth",
			cfg: models.ResticConfig{
				Repository:   "rest:http://localhost:8000/backup",
				Password:     "secret",
				RestUser:     "admin",
				RestPassword: "restpass",
			},
			expected: []string{
				"RESTIC_REPOSITORY=rest:http://localhost:8000/backup",
				"RESTIC_PASSWORD=secret",
				"RESTIC_REST_USERNAME=admin",
				"RESTIC_REST_PASSWORD=restpass",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := svc.buildEnv(tt.cfg)
			for _, exp := range tt.expected {
				assert.Contains(t, env, exp)
			}
		})
	}
}

func TestBackup_StreamingProgress(t *testing.T) {
	// Simulated restic JSON output with status messages at different percentages
	// These represent: 10%, 10% (duplicate), 25%, 50%, 50% (duplicate)
	statusMsgs := []string{
		`{"message_type":"status","percent_done":0.10,"files_done":60,"bytes_done":20971520,"total_files":600,"total_bytes":209715200}`,
		`{"message_type":"status","percent_done":0.105,"files_done":63,"bytes_done":22020096,"total_files":600,"total_bytes":209715200}`,
		`{"message_type":"status","percent_done":0.25,"files_done":150,"bytes_done":52428800,"total_files":600,"total_bytes":209715200}`,
		`{"message_type":"status","percent_done":0.50,"files_done":300,"bytes_done":104857600,"total_files":600,"total_bytes":209715200}`,
		`{"message_type":"status","percent_done":0.501,"files_done":301,"bytes_done":105000000,"total_files":600,"total_bytes":209715200}`,
	}
	summaryMsg := `{"message_type":"summary","files_new":10,"files_changed":5,"files_unmodified":585,"data_added":1048576,"total_files_processed":600,"total_bytes_processed":209715200,"snapshot_id":"abc123"}`

	var callbackCount int

	executor := &mockExecutor{
		executeWithEnvStreamingFunc: func(ctx context.Context, env []string, progressCb models.ResticProgressCallback, name string, args ...string) ([]byte, error) {
			// Simulate streaming by calling progressCb for each status message
			for _, msg := range statusMsgs {
				var progress models.BackupProgress
				_ = json.Unmarshal([]byte(msg), &progress)
				if progressCb != nil {
					progressCb(progress)
					callbackCount++
				}
			}

			fullOutput := ""
			for _, msg := range statusMsgs {
				fullOutput += msg + "\n"
			}
			fullOutput += summaryMsg + "\n"
			return []byte(fullOutput), nil
		},
	}

	// Create logger with Debug level to trigger streaming
	logger := zerolog.New(io.Discard).Level(zerolog.DebugLevel)
	svc := NewWithExecutor(logger, executor)

	settings := models.BackupSettings{
		Paths: []string{"/data"},
		Host:  "testhost",
	}

	result, err := svc.Backup(context.Background(), testConfig(), settings)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Error)
	assert.Equal(t, "abc123", result.SnapshotID)

	// Verify all 5 status messages were passed to callback
	// (the filtering of duplicates happens inside the callback in Backup())
	assert.Equal(t, 5, callbackCount)
}

func TestBackup_StreamingProgressFiltering(t *testing.T) {
	// Test that only new whole percentages are logged when callbacks happen quickly
	// Simulates: 0%, 0.5%, 1%, 1.5%, 2%, 2.5%
	// Should only log: 0%, 1%, 2% (3 unique whole percentages)
	// Time-based logging (every 30s) won't trigger since callbacks are instant
	statusMsgs := []string{
		`{"message_type":"status","percent_done":0.001,"files_done":1,"bytes_done":1000}`,
		`{"message_type":"status","percent_done":0.005,"files_done":5,"bytes_done":5000}`,
		`{"message_type":"status","percent_done":0.01,"files_done":10,"bytes_done":10000}`,
		`{"message_type":"status","percent_done":0.015,"files_done":15,"bytes_done":15000}`,
		`{"message_type":"status","percent_done":0.02,"files_done":20,"bytes_done":20000}`,
		`{"message_type":"status","percent_done":0.025,"files_done":25,"bytes_done":25000}`,
	}
	summaryMsg := `{"message_type":"summary","snapshot_id":"abc123"}`

	var logBuffer bytes.Buffer
	logger := zerolog.New(&logBuffer).Level(zerolog.DebugLevel)

	executor := &mockExecutor{
		executeWithEnvStreamingFunc: func(ctx context.Context, env []string, progressCb models.ResticProgressCallback, name string, args ...string) ([]byte, error) {
			for _, msg := range statusMsgs {
				var progress models.BackupProgress
				_ = json.Unmarshal([]byte(msg), &progress)
				if progressCb != nil {
					progressCb(progress)
				}
			}
			return []byte(summaryMsg), nil
		},
	}

	svc := NewWithExecutor(logger, executor)

	settings := models.BackupSettings{
		Paths: []string{"/data"},
	}

	_, err := svc.Backup(context.Background(), testConfig(), settings)
	require.NoError(t, err)

	// Parse log output to count how many progress messages were logged
	loggedPercents := parseLoggedPercents(t, logBuffer.String())

	// Should have logged 0%, 1%, 2% = 3 entries (no time-based logs since instant)
	assert.Len(t, loggedPercents, 3, "should only log unique whole percentages")
	assert.Equal(t, []int{0, 1, 2}, loggedPercents)
}

func TestBackup_StreamingProgressStuckAtZero(t *testing.T) {
	// Test that progress is logged even when stuck at 0% for a long time
	// This simulates a large backup where percent stays at 0% but files/bytes increase
	// The time-based logging (every 30s) ensures we see progress
	statusMsgs := []string{
		`{"message_type":"status","percent_done":0.001,"files_done":10,"bytes_done":1000000}`,
		`{"message_type":"status","percent_done":0.002,"files_done":20,"bytes_done":2000000}`,
		`{"message_type":"status","percent_done":0.003,"files_done":30,"bytes_done":3000000}`,
		`{"message_type":"status","percent_done":0.004,"files_done":40,"bytes_done":4000000}`,
	}
	summaryMsg := `{"message_type":"summary","snapshot_id":"abc123"}`

	var logBuffer bytes.Buffer
	logger := zerolog.New(&logBuffer).Level(zerolog.DebugLevel)

	executor := &mockExecutor{
		executeWithEnvStreamingFunc: func(ctx context.Context, env []string, progressCb models.ResticProgressCallback, name string, args ...string) ([]byte, error) {
			for _, msg := range statusMsgs {
				var progress models.BackupProgress
				_ = json.Unmarshal([]byte(msg), &progress)
				if progressCb != nil {
					progressCb(progress)
				}
			}
			return []byte(summaryMsg), nil
		},
	}

	svc := NewWithExecutor(logger, executor)

	settings := models.BackupSettings{
		Paths: []string{"/data"},
	}

	_, err := svc.Backup(context.Background(), testConfig(), settings)
	require.NoError(t, err)

	// Parse log output
	loggedPercents := parseLoggedPercents(t, logBuffer.String())

	// All messages are at 0% (0.001-0.004 rounds to 0), so only 1 log entry
	// (time-based logging won't trigger in this fast test)
	assert.Len(t, loggedPercents, 1, "should log once at 0%")
	assert.Equal(t, []int{0}, loggedPercents)
}

// parseLoggedPercents extracts percent values from log output.
func parseLoggedPercents(t *testing.T, logOutput string) []int {
	t.Helper()
	var loggedPercents []int
	for _, line := range bytes.Split([]byte(logOutput), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var logEntry map[string]any
		if err := json.Unmarshal(line, &logEntry); err == nil {
			if msg, ok := logEntry["message"].(string); ok && msg == "backup progress" {
				if pct, ok := logEntry["percent"].(float64); ok {
					loggedPercents = append(loggedPercents, int(pct))
				}
			}
		}
	}
	return loggedPercents
}

func TestBackup_NonStreamingWithInfoLevel(t *testing.T) {
	summary := `{"message_type":"summary","files_new":10,"snapshot_id":"abc123"}`
	streamingCalled := false
	nonStreamingCalled := false

	executor := &mockExecutor{
		executeWithEnvFunc: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			nonStreamingCalled = true
			return []byte(summary), nil
		},
		executeWithEnvStreamingFunc: func(ctx context.Context, env []string, progressCb models.ResticProgressCallback, name string, args ...string) ([]byte, error) {
			streamingCalled = true
			return []byte(summary), nil
		},
	}

	// Create logger with Info level (not Debug) - should use non-streaming
	logger := zerolog.New(io.Discard).Level(zerolog.InfoLevel)
	svc := NewWithExecutor(logger, executor)

	settings := models.BackupSettings{
		Paths: []string{"/data"},
	}

	result, err := svc.Backup(context.Background(), testConfig(), settings)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "abc123", result.SnapshotID)

	// Verify non-streaming was used
	assert.True(t, nonStreamingCalled, "non-streaming executor should be called")
	assert.False(t, streamingCalled, "streaming executor should not be called")
}

func TestBackupProgress_JSONParsing(t *testing.T) {
	jsonStr := `{"message_type":"status","percent_done":0.75,"total_files":1000,"files_done":750,"total_bytes":1073741824,"bytes_done":805306368,"current_files":["/data/file1.txt","/data/file2.txt"]}`

	var progress models.BackupProgress
	err := json.Unmarshal([]byte(jsonStr), &progress)

	require.NoError(t, err)
	assert.Equal(t, "status", progress.MessageType)
	assert.Equal(t, 0.75, progress.PercentDone)
	assert.Equal(t, uint64(1000), progress.TotalFiles)
	assert.Equal(t, uint64(750), progress.FilesDone)
	assert.Equal(t, uint64(1073741824), progress.TotalBytes)
	assert.Equal(t, uint64(805306368), progress.BytesDone)
	assert.Len(t, progress.CurrentFiles, 2)
	assert.Equal(t, "/data/file1.txt", progress.CurrentFiles[0])
}
