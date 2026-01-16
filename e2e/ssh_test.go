//go:build e2e

package e2e

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/fgeck/gorestic-homelab/internal/services/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getSSHConfig(t *testing.T) models.SSHShutdownConfig {
	t.Helper()

	host := os.Getenv("TEST_SSH_HOST")
	if host == "" {
		t.Skip("TEST_SSH_HOST not set")
	}

	portStr := os.Getenv("TEST_SSH_PORT")
	if portStr == "" {
		portStr = "22"
	}
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	user := os.Getenv("TEST_SSH_USER")
	if user == "" {
		user = "root"
	}

	keyPath := os.Getenv("TEST_SSH_KEY_PATH")
	if keyPath == "" {
		t.Skip("TEST_SSH_KEY_PATH not set")
	}

	return models.SSHShutdownConfig{
		Host:          host,
		Port:          port,
		Username:      user,
		KeyPath:       keyPath,
		ShutdownDelay: 60, // Use long delay for safety in tests
	}
}

func TestSSHTestConnection_E2E(t *testing.T) {
	cfg := getSSHConfig(t)

	svc := ssh.New(testLogger())

	result, err := svc.TestConnection(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.CommandRun)
	assert.Contains(t, result.Output, "OK")
	assert.Nil(t, result.Error)
}

func TestSSHConnectionFailed_E2E(t *testing.T) {
	cfg := models.SSHShutdownConfig{
		Host:     "192.168.255.254", // Non-routable IP
		Port:     22,
		Username: "root",
		KeyPath:  os.Getenv("TEST_SSH_KEY_PATH"),
	}

	if cfg.KeyPath == "" {
		t.Skip("TEST_SSH_KEY_PATH not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*1e9) // 5 second timeout
	defer cancel()

	svc := ssh.New(testLogger())

	result, err := svc.TestConnection(ctx, cfg)

	require.NoError(t, err)
	assert.False(t, result.CommandRun)
	assert.NotNil(t, result.Error)
}

func TestSSHInvalidKey_E2E(t *testing.T) {
	cfg := models.SSHShutdownConfig{
		Host:       "localhost",
		Port:       22,
		Username:   "root",
		PrivateKey: []byte("invalid key"),
	}

	svc := ssh.New(testLogger())

	result, err := svc.TestConnection(context.Background(), cfg)

	require.NoError(t, err)
	assert.False(t, result.CommandRun)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "parse private key")
}

// WARNING: This test will actually initiate a shutdown!
// Only run if you really want to test shutdown functionality.
func TestSSHShutdown_E2E(t *testing.T) {
	if os.Getenv("TEST_SSH_SHUTDOWN_ENABLED") != "true" {
		t.Skip("TEST_SSH_SHUTDOWN_ENABLED is not true - skipping actual shutdown test")
	}

	cfg := getSSHConfig(t)

	svc := ssh.New(testLogger())

	result, err := svc.Shutdown(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.CommandRun)
	// Note: result.Error might be non-nil due to connection closing
}
