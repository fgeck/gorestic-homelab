package wol

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/mdlayher/wol"
	"github.com/rs/zerolog"
)

// Service defines the interface for Wake-on-LAN operations.
type Service interface {
	Wake(ctx context.Context, cfg models.WOLConfig) (*models.WOLResult, error)
}

// WOLClient wraps the wol library for mocking.
type WOLClient interface {
	Wake(broadcastIP string, mac net.HardwareAddr) error
}

// HTTPClient allows mocking HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultWOLClient is the default implementation using mdlayher/wol.
type DefaultWOLClient struct{}

// Wake sends a magic packet to the specified MAC address.
func (c *DefaultWOLClient) Wake(broadcastIP string, mac net.HardwareAddr) error {
	client, err := wol.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create WOL client: %w", err)
	}
	defer client.Close()

	// Parse broadcast IP
	ip := net.ParseIP(broadcastIP)
	if ip == nil {
		return fmt.Errorf("invalid broadcast IP: %s", broadcastIP)
	}

	// Send wake packet
	if err := client.Wake(ip.String()+":9", mac); err != nil {
		return fmt.Errorf("failed to send WOL packet: %w", err)
	}

	return nil
}

// Impl implements the WOL Service interface.
type Impl struct {
	wolClient  WOLClient
	httpClient HTTPClient
	logger     zerolog.Logger
}

// New creates a new WOL service.
func New(logger zerolog.Logger) *Impl {
	return &Impl{
		wolClient: &DefaultWOLClient{},
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// NewWithClients creates a new WOL service with custom clients (for testing).
func NewWithClients(logger zerolog.Logger, wolClient WOLClient, httpClient HTTPClient) *Impl {
	return &Impl{
		wolClient:  wolClient,
		httpClient: httpClient,
		logger:     logger,
	}
}

// Wake sends a WOL packet and optionally waits for the target to become available.
func (s *Impl) Wake(ctx context.Context, cfg models.WOLConfig) (*models.WOLResult, error) {
	result := &models.WOLResult{}
	start := time.Now()

	// Parse MAC address
	mac, err := net.ParseMAC(cfg.MACAddress)
	if err != nil {
		result.Error = fmt.Errorf("invalid MAC address %q: %w", cfg.MACAddress, err)
		return result, nil
	}

	s.logger.Info().
		Str("mac", cfg.MACAddress).
		Str("broadcast", cfg.BroadcastIP).
		Msg("sending WOL packet")

	// Send WOL packet
	if err := s.wolClient.Wake(cfg.BroadcastIP, mac); err != nil {
		result.Error = err
		return result, nil
	}

	result.PacketSent = true
	s.logger.Info().Msg("WOL packet sent successfully")

	// If no target URL specified, we're done
	if cfg.TargetURL == "" {
		result.WaitDuration = time.Since(start)
		result.TargetReady = true
		return result, nil
	}

	// Wait for target to become available
	s.logger.Info().
		Str("url", cfg.TargetURL).
		Dur("timeout", cfg.Timeout).
		Msg("waiting for target to become available")

	if err := s.waitForTarget(ctx, cfg); err != nil {
		result.WaitDuration = time.Since(start)
		result.Error = err
		return result, nil
	}

	// Wait for stabilization
	if cfg.StabilizeWait > 0 {
		s.logger.Debug().Str("wait", cfg.StabilizeWait.Round(time.Millisecond).String()).Msg("waiting for target to stabilize")
		select {
		case <-ctx.Done():
			result.WaitDuration = time.Since(start)
			result.Error = ctx.Err()
			return result, nil
		case <-time.After(cfg.StabilizeWait):
		}
	}

	result.TargetReady = true
	result.WaitDuration = time.Since(start)

	s.logger.Info().
		Dur("duration", result.WaitDuration).
		Msg("target is ready")

	return result, nil
}

func (s *Impl) waitForTarget(ctx context.Context, cfg models.WOLConfig) error {
	deadline := time.Now().Add(cfg.Timeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for target at %s", cfg.TargetURL)
		}

		// Try to connect to target
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.TargetURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := s.httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
			// Any response means the target is up
			return nil
		}

		s.logger.Debug().Err(err).Msg("target not ready yet")

		// Wait before next poll
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.PollInterval):
		}
	}
}
