//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/fgeck/gorestic-homelab/internal/services/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getPostgresConfig(t *testing.T) models.PostgresConfig {
	t.Helper()

	host := os.Getenv("TEST_POSTGRES_HOST")
	if host == "" {
		t.Skip("TEST_POSTGRES_HOST not set")
	}

	portStr := os.Getenv("TEST_POSTGRES_PORT")
	if portStr == "" {
		portStr = "5432"
	}
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	database := os.Getenv("TEST_POSTGRES_DB")
	if database == "" {
		t.Skip("TEST_POSTGRES_DB not set")
	}

	user := os.Getenv("TEST_POSTGRES_USER")
	if user == "" {
		user = "postgres"
	}

	password := os.Getenv("TEST_POSTGRES_PASSWORD")

	return models.PostgresConfig{
		Host:     host,
		Port:     port,
		Database: database,
		Username: user,
		Password: password,
		Format:   "custom",
	}
}

func TestPostgresDump_CustomFormat_Integration(t *testing.T) {
	cfg := getPostgresConfig(t)
	cfg.Format = "custom"

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.dump")

	svc := postgres.New(testLogger())

	result, err := svc.Dump(context.Background(), cfg, outputPath)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Error)
	assert.Equal(t, outputPath, result.OutputPath)
	assert.Greater(t, result.SizeBytes, int64(0))
	assert.Greater(t, result.Duration, int64(0))

	// Verify file exists
	_, err = os.Stat(outputPath)
	assert.NoError(t, err)
}

func TestPostgresDump_PlainFormat_Integration(t *testing.T) {
	cfg := getPostgresConfig(t)
	cfg.Format = "plain"

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.sql")

	svc := postgres.New(testLogger())

	result, err := svc.Dump(context.Background(), cfg, outputPath)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Error)

	// Verify file contains SQL
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "PostgreSQL")
}

func TestPostgresDump_TarFormat_Integration(t *testing.T) {
	cfg := getPostgresConfig(t)
	cfg.Format = "tar"

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.tar")

	svc := postgres.New(testLogger())

	result, err := svc.Dump(context.Background(), cfg, outputPath)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.Error)
	assert.Greater(t, result.SizeBytes, int64(0))
}

func TestPostgresDump_InvalidHost_Integration(t *testing.T) {
	cfg := models.PostgresConfig{
		Host:     "invalid-host-that-does-not-exist",
		Port:     5432,
		Database: "testdb",
		Username: "postgres",
		Format:   "custom",
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.dump")

	svc := postgres.New(testLogger())

	result, err := svc.Dump(context.Background(), cfg, outputPath)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Error)

	// Verify partial file was cleaned up
	_, err = os.Stat(outputPath)
	assert.True(t, os.IsNotExist(err))
}
