// Package ssh provides SSH operations for remote server management.
package ssh

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"
)

// Service defines the interface for SSH operations.
type Service interface {
	Shutdown(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error)
	TestConnection(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error)
}

// Client wraps ssh.Client for mocking.
type Client interface {
	NewSession() (Session, error)
	Close() error
}

// Session wraps ssh.Session for mocking.
type Session interface {
	CombinedOutput(cmd string) ([]byte, error)
	Close() error
}

// ClientFactory creates SSH clients.
type ClientFactory interface {
	NewClient(network, addr string, config *ssh.ClientConfig) (Client, error)
}

// DefaultClientFactory is the default SSH client factory.
type DefaultClientFactory struct{}

// NewClient creates a new SSH client.
func (f *DefaultClientFactory) NewClient(network, addr string, config *ssh.ClientConfig) (Client, error) {
	client, err := ssh.Dial(network, addr, config)
	if err != nil {
		return nil, err
	}
	return &defaultClient{client: client}, nil
}

type defaultClient struct {
	client *ssh.Client
}

func (c *defaultClient) NewSession() (Session, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	return &defaultSession{session: session}, nil
}

func (c *defaultClient) Close() error {
	return c.client.Close()
}

type defaultSession struct {
	session *ssh.Session
}

func (s *defaultSession) CombinedOutput(cmd string) ([]byte, error) {
	return s.session.CombinedOutput(cmd)
}

func (s *defaultSession) Close() error {
	return s.session.Close()
}

// Impl implements the SSH Service interface.
type Impl struct {
	clientFactory ClientFactory
	logger        zerolog.Logger
}

// New creates a new SSH service.
func New(logger zerolog.Logger) *Impl {
	return &Impl{
		clientFactory: &DefaultClientFactory{},
		logger:        logger,
	}
}

// NewWithClientFactory creates a new SSH service with a custom client factory (for testing).
func NewWithClientFactory(logger zerolog.Logger, factory ClientFactory) *Impl {
	return &Impl{
		clientFactory: factory,
		logger:        logger,
	}
}

func (s *Impl) buildConfig(cfg models.SSHShutdownConfig) (*ssh.ClientConfig, error) {
	var key []byte
	var err error

	// Load private key from file or use provided key
	switch {
	case len(cfg.PrivateKey) > 0:
		key = cfg.PrivateKey
	case cfg.KeyPath != "":
		key, err = os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key from %s: %w", cfg.KeyPath, err)
		}
	default:
		return nil, fmt.Errorf("no private key provided")
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &ssh.ClientConfig{
		User: cfg.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // homelab environment
		Timeout:         30 * time.Second,
	}, nil
}

// Shutdown initiates a system shutdown via SSH.
func (s *Impl) Shutdown(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
	result := &models.SSHResult{}

	s.logger.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("user", cfg.Username).
		Int("delay", cfg.ShutdownDelay).
		Msg("initiating remote shutdown")

	sshConfig, err := s.buildConfig(cfg)
	if err != nil {
		result.Error = err
		return result, nil
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	// Create client with context timeout
	clientChan := make(chan struct {
		client Client
		err    error
	}, 1)

	go func() {
		client, err := s.clientFactory.NewClient("tcp", addr, sshConfig)
		clientChan <- struct {
			client Client
			err    error
		}{client, err}
	}()

	var client Client
	select {
	case <-ctx.Done():
		result.Error = ctx.Err()
		return result, nil
	case res := <-clientChan:
		if res.err != nil {
			result.Error = fmt.Errorf("failed to connect: %w", res.err)
			return result, nil
		}
		client = res.client
	}
	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		result.Error = fmt.Errorf("failed to create session: %w", err)
		return result, nil
	}
	defer func() { _ = session.Close() }()

	// Build shutdown command based on OS
	var cmd string
	if cfg.OS == "windows" {
		// Windows: shutdown /s /t <seconds>
		delaySeconds := cfg.ShutdownDelay * 60 // Convert minutes to seconds
		if delaySeconds == 0 {
			delaySeconds = 60 // Default 60 seconds for safety
		}
		cmd = fmt.Sprintf("shutdown /s /t %d", delaySeconds)
	} else {
		// Linux/Unix: sudo shutdown -h +<minutes>
		cmd = fmt.Sprintf("sudo shutdown -h +%d", cfg.ShutdownDelay)
		if cfg.ShutdownDelay == 0 {
			cmd = "sudo shutdown -h now"
		}
	}

	s.logger.Debug().Str("command", cmd).Msg("executing shutdown command")

	output, err := session.CombinedOutput(cmd)
	result.Output = string(output)
	result.CommandRun = true

	if err != nil {
		// Some systems return error even on successful shutdown initiation
		// due to connection being closed
		if ctx.Err() != nil {
			result.Error = ctx.Err()
		} else {
			// Log warning but don't treat as error - shutdown may have succeeded
			s.logger.Warn().Err(err).Str("output", result.Output).Msg("shutdown command returned error (may be expected)")
		}
	}

	s.logger.Info().
		Bool("command_run", result.CommandRun).
		Str("output", result.Output).
		Msg("shutdown command completed")

	return result, nil
}

// TestConnection verifies SSH connectivity without executing shutdown.
func (s *Impl) TestConnection(ctx context.Context, cfg models.SSHShutdownConfig) (*models.SSHResult, error) {
	result := &models.SSHResult{}

	s.logger.Debug().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Msg("testing SSH connection")

	sshConfig, err := s.buildConfig(cfg)
	if err != nil {
		result.Error = err
		return result, nil
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	// Create client with context timeout
	clientChan := make(chan struct {
		client Client
		err    error
	}, 1)

	go func() {
		client, err := s.clientFactory.NewClient("tcp", addr, sshConfig)
		clientChan <- struct {
			client Client
			err    error
		}{client, err}
	}()

	var client Client
	select {
	case <-ctx.Done():
		result.Error = ctx.Err()
		return result, nil
	case res := <-clientChan:
		if res.err != nil {
			result.Error = fmt.Errorf("failed to connect: %w", res.err)
			return result, nil
		}
		client = res.client
	}
	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		result.Error = fmt.Errorf("failed to create session: %w", err)
		return result, nil
	}
	defer func() { _ = session.Close() }()

	// Run a simple command to verify connectivity
	output, err := session.CombinedOutput("echo OK")
	result.Output = string(output)
	result.CommandRun = true

	if err != nil {
		result.Error = fmt.Errorf("test command failed: %w", err)
	}

	return result, nil
}
