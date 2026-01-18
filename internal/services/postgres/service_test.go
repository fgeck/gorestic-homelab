package postgres

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExecutor struct {
	executeFunc func(ctx context.Context, env []string, outputPath string, name string, args ...string) error
}

func (m *mockExecutor) ExecuteWithEnv(ctx context.Context, env []string, outputPath string, name string, args ...string) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, env, outputPath, name, args...)
	}
	// Default behavior: create an empty output file
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func testConfig() models.PostgresConfig {
	return models.PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		Username: "postgres",
		Password: "secret",
		Format:   "custom",
	}
}

func TestDump_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.dump")

	var capturedArgs []string
	var capturedEnv []string

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, env []string, op string, name string, args ...string) error {
			capturedArgs = args
			capturedEnv = env

			// Create the output file with some content
			return os.WriteFile(op, []byte("test dump content"), 0o600)
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	result, err := svc.Dump(context.Background(), testConfig(), outputPath)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Error)
	assert.Equal(t, outputPath, result.OutputPath)
	assert.Greater(t, result.SizeBytes, int64(0))
	assert.Greater(t, result.Duration, int64(0))

	// Verify arguments
	assert.Contains(t, capturedArgs, "-h")
	assert.Contains(t, capturedArgs, "localhost")
	assert.Contains(t, capturedArgs, "-p")
	assert.Contains(t, capturedArgs, "5432")
	assert.Contains(t, capturedArgs, "-U")
	assert.Contains(t, capturedArgs, "postgres")
	assert.Contains(t, capturedArgs, "-d")
	assert.Contains(t, capturedArgs, "testdb")
	assert.Contains(t, capturedArgs, "-Fc") // custom format

	// Verify password in env
	assert.Contains(t, capturedEnv, "PGPASSWORD=secret")
}

func TestDump_PlainFormat(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.sql")

	var capturedArgs []string

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, env []string, op string, name string, args ...string) error {
			capturedArgs = args
			return os.WriteFile(op, []byte(""), 0o600)
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	cfg := testConfig()
	cfg.Format = "plain"

	result, err := svc.Dump(context.Background(), cfg, outputPath)

	require.NoError(t, err)
	assert.Nil(t, result.Error)
	assert.Contains(t, capturedArgs, "-Fp")
}

func TestDump_TarFormat(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.tar")

	var capturedArgs []string

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, env []string, op string, name string, args ...string) error {
			capturedArgs = args
			return os.WriteFile(op, []byte(""), 0o600)
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	cfg := testConfig()
	cfg.Format = "tar"

	result, err := svc.Dump(context.Background(), cfg, outputPath)

	require.NoError(t, err)
	assert.Nil(t, result.Error)
	assert.Contains(t, capturedArgs, "-Ft")
}

func TestDump_ExecutorError(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.dump")

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, env []string, op string, name string, args ...string) error {
			return errors.New("connection refused")
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	result, err := svc.Dump(context.Background(), testConfig(), outputPath)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "connection refused")

	// Verify partial file was cleaned up
	_, statErr := os.Stat(outputPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestDump_NoPassword(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.dump")

	var capturedEnv []string

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, env []string, op string, name string, args ...string) error {
			capturedEnv = env
			return os.WriteFile(op, []byte(""), 0o600)
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	cfg := testConfig()
	cfg.Password = ""

	result, err := svc.Dump(context.Background(), cfg, outputPath)

	require.NoError(t, err)
	assert.Nil(t, result.Error)

	// Verify no PGPASSWORD in env
	for _, e := range capturedEnv {
		assert.NotContains(t, e, "PGPASSWORD")
	}
}

func TestDump_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "subdir", "nested", "test.dump")

	executor := &mockExecutor{
		executeFunc: func(ctx context.Context, env []string, op string, name string, args ...string) error {
			return os.WriteFile(op, []byte("test"), 0o600)
		},
	}

	svc := NewWithExecutor(testLogger(), executor)
	result, err := svc.Dump(context.Background(), testConfig(), outputPath)

	require.NoError(t, err)
	assert.Nil(t, result.Error)

	// Verify directory was created
	_, statErr := os.Stat(filepath.Dir(outputPath))
	assert.NoError(t, statErr)
}

func TestGetOutputFilename(t *testing.T) {
	tests := []struct {
		name           string
		format         string
		expectedSuffix string
	}{
		{"custom format", "custom", ".dump"},
		{"plain format", "plain", ".sql"},
		{"tar format", "tar", ".tar"},
		{"empty format", "", ".dump"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := models.PostgresConfig{
				Database: "mydb",
				Format:   tt.format,
			}

			filename := GetOutputFilename(cfg)

			assert.Contains(t, filename, "mydb-")
			assert.True(t, len(filename) > len(tt.expectedSuffix))
			assert.Contains(t, filename, tt.expectedSuffix)
		})
	}
}

func TestDefaultExecutor_CapturesStderr(t *testing.T) {
	// Test that DefaultExecutor captures stderr from failed commands
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")

	executor := &DefaultExecutor{}

	// Use a command that writes to stderr and fails
	// "sh -c" allows us to write to stderr and exit with error
	err := executor.ExecuteWithEnv(
		context.Background(),
		nil,
		outputPath,
		"sh",
		"-c", "echo 'error message' >&2 && exit 1",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pg_dump failed")
	assert.Contains(t, err.Error(), "error message")
}

func TestDefaultExecutor_SuccessNoStderr(t *testing.T) {
	// Test that DefaultExecutor succeeds when command succeeds
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")

	executor := &DefaultExecutor{}

	err := executor.ExecuteWithEnv(
		context.Background(),
		nil,
		outputPath,
		"sh",
		"-c", "echo 'success output'",
	)

	require.NoError(t, err)

	// Verify output was written to file
	content, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "success output")
}
