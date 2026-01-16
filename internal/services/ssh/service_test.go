package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Mock implementations
type mockSSHSession struct {
	combinedOutputFunc func(cmd string) ([]byte, error)
	closeFunc          func() error
}

func (m *mockSSHSession) CombinedOutput(cmd string) ([]byte, error) {
	if m.combinedOutputFunc != nil {
		return m.combinedOutputFunc(cmd)
	}
	return []byte(""), nil
}

func (m *mockSSHSession) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

type mockSSHClient struct {
	newSessionFunc func() (SSHSession, error)
	closeFunc      func() error
}

func (m *mockSSHClient) NewSession() (SSHSession, error) {
	if m.newSessionFunc != nil {
		return m.newSessionFunc()
	}
	return &mockSSHSession{}, nil
}

func (m *mockSSHClient) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

type mockClientFactory struct {
	newClientFunc func(network, addr string, config *ssh.ClientConfig) (SSHClient, error)
}

func (m *mockClientFactory) NewClient(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
	if m.newClientFunc != nil {
		return m.newClientFunc(network, addr, config)
	}
	return &mockSSHClient{}, nil
}

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

// generateTestKey generates a valid ed25519 key for testing using crypto/ed25519.
func generateTestKey(t *testing.T) []byte {
	t.Helper()

	// Generate a real ed25519 key pair
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Marshal to OpenSSH format
	pemBlock, err := ssh.MarshalPrivateKey(privateKey, "")
	require.NoError(t, err)

	return pem.EncodeToMemory(pemBlock)
}

func testConfig(t *testing.T) models.SSHShutdownConfig {
	return models.SSHShutdownConfig{
		Host:          "192.168.1.100",
		Port:          22,
		Username:      "root",
		PrivateKey:    generateTestKey(t),
		ShutdownDelay: 1,
	}
}

func TestShutdown_Success(t *testing.T) {
	var capturedCommand string

	factory := &mockClientFactory{
		newClientFunc: func(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
			return &mockSSHClient{
				newSessionFunc: func() (SSHSession, error) {
					return &mockSSHSession{
						combinedOutputFunc: func(cmd string) ([]byte, error) {
							capturedCommand = cmd
							return []byte("Shutdown scheduled"), nil
						},
					}, nil
				},
			}, nil
		},
	}

	svc := NewWithClientFactory(testLogger(), factory)
	result, err := svc.Shutdown(context.Background(), testConfig(t))

	require.NoError(t, err)
	assert.True(t, result.CommandRun)
	assert.Contains(t, result.Output, "Shutdown scheduled")
	assert.Nil(t, result.Error)

	// Verify the shutdown command includes the delay
	assert.Contains(t, capturedCommand, "sudo shutdown -h +1")
}

func TestShutdown_ImmediateShutdown(t *testing.T) {
	var capturedCommand string

	factory := &mockClientFactory{
		newClientFunc: func(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
			return &mockSSHClient{
				newSessionFunc: func() (SSHSession, error) {
					return &mockSSHSession{
						combinedOutputFunc: func(cmd string) ([]byte, error) {
							capturedCommand = cmd
							return []byte(""), nil
						},
					}, nil
				},
			}, nil
		},
	}

	svc := NewWithClientFactory(testLogger(), factory)
	cfg := testConfig(t)
	cfg.ShutdownDelay = 0

	result, err := svc.Shutdown(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.CommandRun)
	assert.Equal(t, "sudo shutdown -h now", capturedCommand)
}

func TestShutdown_ConnectionFailed(t *testing.T) {
	factory := &mockClientFactory{
		newClientFunc: func(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
			return nil, errors.New("connection refused")
		},
	}

	svc := NewWithClientFactory(testLogger(), factory)
	result, err := svc.Shutdown(context.Background(), testConfig(t))

	require.NoError(t, err)
	assert.False(t, result.CommandRun)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "failed to connect")
}

func TestShutdown_SessionFailed(t *testing.T) {
	factory := &mockClientFactory{
		newClientFunc: func(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
			return &mockSSHClient{
				newSessionFunc: func() (SSHSession, error) {
					return nil, errors.New("session creation failed")
				},
			}, nil
		},
	}

	svc := NewWithClientFactory(testLogger(), factory)
	result, err := svc.Shutdown(context.Background(), testConfig(t))

	require.NoError(t, err)
	assert.False(t, result.CommandRun)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "failed to create session")
}

func TestShutdown_NoPrivateKey(t *testing.T) {
	svc := NewWithClientFactory(testLogger(), &mockClientFactory{})
	cfg := models.SSHShutdownConfig{
		Host:     "192.168.1.100",
		Port:     22,
		Username: "root",
		// No key provided
	}

	result, err := svc.Shutdown(context.Background(), cfg)

	require.NoError(t, err)
	assert.False(t, result.CommandRun)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "no private key")
}

func TestShutdown_InvalidPrivateKey(t *testing.T) {
	svc := NewWithClientFactory(testLogger(), &mockClientFactory{})
	cfg := models.SSHShutdownConfig{
		Host:       "192.168.1.100",
		Port:       22,
		Username:   "root",
		PrivateKey: []byte("invalid key"),
	}

	result, err := svc.Shutdown(context.Background(), cfg)

	require.NoError(t, err)
	assert.False(t, result.CommandRun)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "failed to parse private key")
}

func TestShutdown_ContextCancelled(t *testing.T) {
	factory := &mockClientFactory{
		newClientFunc: func(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
			// Simulate slow connection
			time.Sleep(100 * time.Millisecond)
			return &mockSSHClient{}, nil
		},
	}

	svc := NewWithClientFactory(testLogger(), factory)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	result, err := svc.Shutdown(ctx, testConfig(t))

	require.NoError(t, err)
	assert.False(t, result.CommandRun)
	assert.NotNil(t, result.Error)
	assert.Equal(t, context.DeadlineExceeded, result.Error)
}

func TestTestConnection_Success(t *testing.T) {
	factory := &mockClientFactory{
		newClientFunc: func(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
			return &mockSSHClient{
				newSessionFunc: func() (SSHSession, error) {
					return &mockSSHSession{
						combinedOutputFunc: func(cmd string) ([]byte, error) {
							if cmd == "echo OK" {
								return []byte("OK\n"), nil
							}
							return nil, errors.New("unexpected command")
						},
					}, nil
				},
			}, nil
		},
	}

	svc := NewWithClientFactory(testLogger(), factory)
	result, err := svc.TestConnection(context.Background(), testConfig(t))

	require.NoError(t, err)
	assert.True(t, result.CommandRun)
	assert.Contains(t, result.Output, "OK")
	assert.Nil(t, result.Error)
}

func TestTestConnection_Failed(t *testing.T) {
	factory := &mockClientFactory{
		newClientFunc: func(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
			return nil, errors.New("connection refused")
		},
	}

	svc := NewWithClientFactory(testLogger(), factory)
	result, err := svc.TestConnection(context.Background(), testConfig(t))

	require.NoError(t, err)
	assert.False(t, result.CommandRun)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "failed to connect")
}

func TestBuildConfig_WithKeyPath(t *testing.T) {
	// Create a temporary key file with valid content
	tmpDir := t.TempDir()
	keyPath := tmpDir + "/test_key"

	err := os.WriteFile(keyPath, generateTestKey(t), 0o600)
	require.NoError(t, err)

	factory := &mockClientFactory{}
	svc := NewWithClientFactory(testLogger(), factory)

	cfg := models.SSHShutdownConfig{
		Host:     "192.168.1.100",
		Port:     22,
		Username: "root",
		KeyPath:  keyPath,
	}

	sshConfig, err := svc.buildConfig(cfg)

	require.NoError(t, err)
	assert.NotNil(t, sshConfig)
	assert.Equal(t, "root", sshConfig.User)
}

func TestBuildConfig_KeyPathNotFound(t *testing.T) {
	factory := &mockClientFactory{}
	svc := NewWithClientFactory(testLogger(), factory)

	cfg := models.SSHShutdownConfig{
		Host:     "192.168.1.100",
		Port:     22,
		Username: "root",
		KeyPath:  "/nonexistent/path/id_rsa",
	}

	_, err := svc.buildConfig(cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read private key")
}
