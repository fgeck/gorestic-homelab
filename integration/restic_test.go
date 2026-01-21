//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/fgeck/gorestic-homelab/internal/services/restic"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getResticConfig(t *testing.T) models.ResticConfig {
	t.Helper()

	repo := os.Getenv("TEST_RESTIC_REPO")
	if repo == "" {
		t.Skip("TEST_RESTIC_REPO not set")
	}

	password := os.Getenv("TEST_RESTIC_PASSWORD")
	if password == "" {
		t.Skip("TEST_RESTIC_PASSWORD not set")
	}

	return models.ResticConfig{
		Repository:   repo,
		Password:     password,
		RestUser:     os.Getenv("TEST_RESTIC_REST_USER"),
		RestPassword: os.Getenv("TEST_RESTIC_REST_PASSWORD"),
	}
}

func testLogger() zerolog.Logger {
	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}

func TestResticInit_Integration(t *testing.T) {
	cfg := getResticConfig(t)

	svc := restic.New(testLogger())
	err := svc.Init(context.Background(), cfg)

	require.NoError(t, err)
}

func TestResticBackupAndSnapshots_Integration(t *testing.T) {
	cfg := getResticConfig(t)

	// Create temporary test data
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"
	err := os.WriteFile(testFile, []byte("test data for backup"), 0o600)
	require.NoError(t, err)

	svc := restic.New(testLogger())

	// Initialize repository first
	err = svc.Init(context.Background(), cfg)
	require.NoError(t, err)

	// Perform backup
	backupSettings := models.BackupSettings{
		Paths: []string{tmpDir},
		Tags:  []string{"integration-test"},
		Host:  "test-host",
	}

	result, err := svc.Backup(context.Background(), cfg, backupSettings)

	require.NoError(t, err)
	assert.NotEmpty(t, result.SnapshotID)
	assert.Nil(t, result.Error)

	// List snapshots
	snapshots, err := svc.Snapshots(context.Background(), cfg)

	require.NoError(t, err)
	assert.NotEmpty(t, snapshots)

	// Find our snapshot
	found := false
	for _, snap := range snapshots {
		if snap.ID == result.SnapshotID {
			found = true
			assert.Equal(t, "test-host", snap.Hostname)
			assert.Contains(t, snap.Tags, "integration-test")
			break
		}
	}
	assert.True(t, found, "backup snapshot not found in snapshots list")
}

func TestResticForget_Integration(t *testing.T) {
	cfg := getResticConfig(t)

	// Create test data and backup
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.txt"
	err := os.WriteFile(testFile, []byte("test data"), 0o600)
	require.NoError(t, err)

	svc := restic.New(testLogger())

	err = svc.Init(context.Background(), cfg)
	require.NoError(t, err)

	// Create multiple snapshots
	for i := 0; i < 3; i++ {
		settings := models.BackupSettings{
			Paths: []string{tmpDir},
			Tags:  []string{"forget-test"},
			Host:  "test-host",
		}
		_, err := svc.Backup(context.Background(), cfg, settings)
		require.NoError(t, err)
	}

	// Apply retention policy
	policy := models.RetentionPolicy{
		KeepDaily: 1,
	}

	result, err := svc.Forget(context.Background(), cfg, policy)

	require.NoError(t, err)
	assert.Nil(t, result.Error)
	assert.GreaterOrEqual(t, result.SnapshotsKept, 1)
}

func TestResticCheck_Integration(t *testing.T) {
	cfg := getResticConfig(t)

	svc := restic.New(testLogger())

	err := svc.Init(context.Background(), cfg)
	require.NoError(t, err)

	settings := models.CheckSettings{
		Enabled: true,
		Subset:  "",
	}

	result, err := svc.Check(context.Background(), cfg, settings)

	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Nil(t, result.Error)
}

func TestResticBackupProgress_Integration(t *testing.T) {
	cfg := getResticConfig(t)

	// Create temporary test data with multiple files to see progress
	tmpDir := t.TempDir()
	for i := 0; i < 100; i++ {
		testFile := tmpDir + "/test" + string(rune('0'+i%10)) + string(rune('0'+i/10)) + ".txt"
		err := os.WriteFile(testFile, []byte("test data for progress backup - padding to make files larger for progress visibility"), 0o600)
		require.NoError(t, err)
	}

	// Use debug-level logger to enable progress streaming
	// Progress is only logged when a new whole percentage (0-100) is reached
	debugLogger := zerolog.New(os.Stdout).With().Timestamp().Logger().Level(zerolog.DebugLevel)
	svc := restic.New(debugLogger)

	// Initialize repository first
	err := svc.Init(context.Background(), cfg)
	require.NoError(t, err)

	// Perform backup - should show progress messages at each new percentage
	backupSettings := models.BackupSettings{
		Paths: []string{tmpDir},
		Tags:  []string{"progress-test"},
		Host:  "test-host",
	}

	t.Log("Starting backup with progress logging enabled (debug level)...")
	t.Log("Progress is logged only when reaching new whole percentages (0%, 1%, 2%, ..., 100%)")
	result, err := svc.Backup(context.Background(), cfg, backupSettings)

	require.NoError(t, err)
	assert.NotEmpty(t, result.SnapshotID)
	assert.Nil(t, result.Error)
	t.Logf("Backup completed: snapshot=%s, files_new=%d, data_added=%d",
		result.SnapshotID, result.FilesNew, result.DataAdded)
}
