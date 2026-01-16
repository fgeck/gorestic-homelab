//go:build e2e

package e2e

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/fgeck/gorestic-homelab/internal/services/wol"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func TestWOL_WithHTTPTarget_E2E(t *testing.T) {
	// Create a test HTTP server to act as the "target"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Mock WOL client that doesn't actually send packets
	mockWOLClient := &mockWOLClient{}
	mockHTTPClient := server.Client()

	svc := wol.NewWithClients(testLogger(), mockWOLClient, mockHTTPClient)

	cfg := models.WOLConfig{
		MACAddress:    "AA:BB:CC:DD:EE:FF",
		BroadcastIP:   "255.255.255.255",
		TargetURL:     server.URL,
		Timeout:       5 * time.Second,
		PollInterval:  100 * time.Millisecond,
		StabilizeWait: 100 * time.Millisecond,
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.True(t, result.TargetReady)
	assert.Nil(t, result.Error)
	assert.Greater(t, result.WaitDuration, 100*time.Millisecond)
}

func TestWOL_DelayedTarget_E2E(t *testing.T) {
	// Server that starts returning 200 after a delay
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mockWOLClient := &mockWOLClient{}
	mockHTTPClient := server.Client()

	svc := wol.NewWithClients(testLogger(), mockWOLClient, mockHTTPClient)

	cfg := models.WOLConfig{
		MACAddress:    "AA:BB:CC:DD:EE:FF",
		BroadcastIP:   "255.255.255.255",
		TargetURL:     server.URL,
		Timeout:       5 * time.Second,
		PollInterval:  50 * time.Millisecond,
		StabilizeWait: 50 * time.Millisecond,
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.True(t, result.TargetReady)
	assert.Nil(t, result.Error)
	assert.GreaterOrEqual(t, requestCount, 3)
}

func TestWOL_TargetNeverReady_E2E(t *testing.T) {
	// Server that always returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	mockWOLClient := &mockWOLClient{}
	mockHTTPClient := server.Client()

	svc := wol.NewWithClients(testLogger(), mockWOLClient, mockHTTPClient)

	cfg := models.WOLConfig{
		MACAddress:    "AA:BB:CC:DD:EE:FF",
		BroadcastIP:   "255.255.255.255",
		TargetURL:     server.URL,
		Timeout:       200 * time.Millisecond,
		PollInterval:  50 * time.Millisecond,
		StabilizeWait: 0,
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.False(t, result.TargetReady)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "timeout")
}

// Mock implementations for E2E tests
type mockWOLClient struct{}

func (m *mockWOLClient) Wake(broadcastIP string, mac net.HardwareAddr) error {
	return nil
}

// RealWOL tests - only run if explicitly configured
func TestRealWOL_E2E(t *testing.T) {
	mac := os.Getenv("TEST_WOL_MAC")
	if mac == "" {
		t.Skip("TEST_WOL_MAC not set")
	}

	targetURL := os.Getenv("TEST_WOL_TARGET_URL")

	svc := wol.New(testLogger())

	cfg := models.WOLConfig{
		MACAddress:    mac,
		BroadcastIP:   "255.255.255.255",
		TargetURL:     targetURL,
		Timeout:       5 * time.Minute,
		PollInterval:  10 * time.Second,
		StabilizeWait: 10 * time.Second,
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	if targetURL != "" {
		assert.True(t, result.TargetReady)
	}
}
