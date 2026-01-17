package wol

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWOLClient struct {
	wakeFunc func(broadcastIP string, mac net.HardwareAddr) error
}

func (m *mockWOLClient) Wake(broadcastIP string, mac net.HardwareAddr) error {
	if m.wakeFunc != nil {
		return m.wakeFunc(broadcastIP, mac)
	}
	return nil
}

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.doFunc != nil {
		return m.doFunc(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func TestWake_Success_NoTargetURL(t *testing.T) {
	var capturedMAC net.HardwareAddr
	var capturedBroadcastIP string

	wolClient := &mockWOLClient{
		wakeFunc: func(broadcastIP string, mac net.HardwareAddr) error {
			capturedMAC = mac
			capturedBroadcastIP = broadcastIP
			return nil
		},
	}

	svc := NewWithClients(testLogger(), wolClient, nil)

	cfg := models.WOLConfig{
		MACAddress:  "AA:BB:CC:DD:EE:FF",
		BroadcastIP: "192.168.1.255",
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.True(t, result.TargetReady)
	assert.Nil(t, result.Error)

	expectedMAC, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	assert.Equal(t, expectedMAC, capturedMAC)
	assert.Equal(t, "192.168.1.255", capturedBroadcastIP)
}

func TestWake_InvalidMAC(t *testing.T) {
	svc := NewWithClients(testLogger(), &mockWOLClient{}, nil)

	cfg := models.WOLConfig{
		MACAddress:  "invalid-mac",
		BroadcastIP: "192.168.1.255",
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.False(t, result.PacketSent)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "invalid MAC address")
}

func TestWake_SendFailed(t *testing.T) {
	wolClient := &mockWOLClient{
		wakeFunc: func(broadcastIP string, mac net.HardwareAddr) error {
			return errors.New("network error")
		},
	}

	svc := NewWithClients(testLogger(), wolClient, nil)

	cfg := models.WOLConfig{
		MACAddress:  "AA:BB:CC:DD:EE:FF",
		BroadcastIP: "192.168.1.255",
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.False(t, result.PacketSent)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "network error")
}

func TestWake_WithTargetURL_ImmediateSuccess(t *testing.T) {
	wolClient := &mockWOLClient{}
	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		},
	}

	svc := NewWithClients(testLogger(), wolClient, httpClient)

	cfg := models.WOLConfig{
		MACAddress:    "AA:BB:CC:DD:EE:FF",
		BroadcastIP:   "192.168.1.255",
		PollURL:       "http://192.168.1.100:8000",
		Timeout:       10 * time.Second,
		PollInterval:  1 * time.Second,
		StabilizeWait: 0,
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.True(t, result.TargetReady)
	assert.Nil(t, result.Error)
}

func TestWake_WithTargetURL_DelayedSuccess(t *testing.T) {
	wolClient := &mockWOLClient{}

	callCount := 0
	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if callCount < 3 {
				return nil, errors.New("connection refused")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		},
	}

	svc := NewWithClients(testLogger(), wolClient, httpClient)

	cfg := models.WOLConfig{
		MACAddress:    "AA:BB:CC:DD:EE:FF",
		BroadcastIP:   "192.168.1.255",
		PollURL:       "http://192.168.1.100:8000",
		Timeout:       10 * time.Second,
		PollInterval:  10 * time.Millisecond,
		StabilizeWait: 0,
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.True(t, result.TargetReady)
	assert.Nil(t, result.Error)
	assert.GreaterOrEqual(t, callCount, 3)
}

func TestWake_WithTargetURL_Timeout(t *testing.T) {
	wolClient := &mockWOLClient{}
	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		},
	}

	svc := NewWithClients(testLogger(), wolClient, httpClient)

	cfg := models.WOLConfig{
		MACAddress:    "AA:BB:CC:DD:EE:FF",
		BroadcastIP:   "192.168.1.255",
		PollURL:       "http://192.168.1.100:8000",
		Timeout:       50 * time.Millisecond,
		PollInterval:  10 * time.Millisecond,
		StabilizeWait: 0,
	}

	result, err := svc.Wake(context.Background(), cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.False(t, result.TargetReady)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "timeout")
}

func TestWake_ContextCancelled(t *testing.T) {
	wolClient := &mockWOLClient{}
	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		},
	}

	svc := NewWithClients(testLogger(), wolClient, httpClient)

	ctx, cancel := context.WithCancel(context.Background())

	cfg := models.WOLConfig{
		MACAddress:    "AA:BB:CC:DD:EE:FF",
		BroadcastIP:   "192.168.1.255",
		PollURL:       "http://192.168.1.100:8000",
		Timeout:       10 * time.Second,
		PollInterval:  100 * time.Millisecond,
		StabilizeWait: 0,
	}

	// Cancel context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := svc.Wake(ctx, cfg)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.False(t, result.TargetReady)
	assert.NotNil(t, result.Error)
	assert.Equal(t, context.Canceled, result.Error)
}

func TestWake_WithStabilizeWait(t *testing.T) {
	wolClient := &mockWOLClient{}
	httpClient := &mockHTTPClient{}

	svc := NewWithClients(testLogger(), wolClient, httpClient)

	stabilizeWait := 50 * time.Millisecond
	cfg := models.WOLConfig{
		MACAddress:    "AA:BB:CC:DD:EE:FF",
		BroadcastIP:   "192.168.1.255",
		PollURL:       "http://192.168.1.100:8000",
		Timeout:       10 * time.Second,
		PollInterval:  10 * time.Millisecond,
		StabilizeWait: stabilizeWait,
	}

	start := time.Now()
	result, err := svc.Wake(context.Background(), cfg)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.True(t, result.PacketSent)
	assert.True(t, result.TargetReady)
	// Duration should be at least the stabilize wait time
	assert.GreaterOrEqual(t, duration, stabilizeWait)
}
