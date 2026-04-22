package pushover

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.doFunc != nil {
		return m.doFunc(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("{}")),
	}, nil
}

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func testConfig() models.PushoverConfig {
	return models.PushoverConfig{
		AppToken: "atoken123",
		UserKey:  "ukey456",
		Priority: 1,
	}
}

func TestSendNotification_Success(t *testing.T) {
	var capturedRequest *http.Request
	var capturedBody string

	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			capturedRequest = req
			body, _ := io.ReadAll(req.Body)
			capturedBody = string(body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{\"status\":1}")),
			}, nil
		},
	}

	svc := NewWithClient(testLogger(), httpClient, "https://api.pushover.net")

	msg := models.PushoverMessage{
		Success:    true,
		Host:       "server1",
		Repository: "/backup",
		StartTime:  time.Now().Add(-5 * time.Minute),
		Duration:   5 * time.Minute,
		SnapshotID: "abc123",
		FilesNew:   10,
	}

	result, err := svc.SendNotification(context.Background(), testConfig(), msg)

	require.NoError(t, err)
	assert.True(t, result.MessageSent)
	assert.Nil(t, result.Error)

	// Verify request
	assert.Equal(t, http.MethodPost, capturedRequest.Method)
	assert.Contains(t, capturedRequest.URL.String(), "/1/messages.json")
	assert.Equal(t, "application/x-www-form-urlencoded", capturedRequest.Header.Get("Content-Type"))

	// Verify form body
	assert.Contains(t, capturedBody, "token=atoken123")
	assert.Contains(t, capturedBody, "user=ukey456")
	assert.Contains(t, capturedBody, "priority=1")
	assert.Contains(t, capturedBody, "title=Backup+Successful")
	assert.Contains(t, capturedBody, "Backup+Statistics")
}

func TestSendNotification_FailureMessage(t *testing.T) {
	var capturedBody string

	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			capturedBody = string(body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{}")),
			}, nil
		},
	}

	svc := NewWithClient(testLogger(), httpClient, "https://api.pushover.net")

	msg := models.PushoverMessage{
		Success:      false,
		Host:         "server1",
		Repository:   "/backup",
		StartTime:    time.Now(),
		Duration:     1 * time.Minute,
		FailedStep:   "backup",
		ErrorMessage: "connection refused",
	}

	result, err := svc.SendNotification(context.Background(), testConfig(), msg)

	require.NoError(t, err)
	assert.True(t, result.MessageSent)

	// Verify message content
	assert.Contains(t, capturedBody, "title=Backup+Failed")
	assert.Contains(t, capturedBody, "Failed+step")
	assert.Contains(t, capturedBody, "connection+refused")
}

func TestSendNotification_HTTPError(t *testing.T) {
	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network error")
		},
	}

	svc := NewWithClient(testLogger(), httpClient, "https://api.pushover.net")

	msg := models.PushoverMessage{
		Success: true,
		Host:    "server1",
	}

	result, err := svc.SendNotification(context.Background(), testConfig(), msg)

	require.NoError(t, err)
	assert.False(t, result.MessageSent)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "failed to send request")
}

func TestSendNotification_APIError(t *testing.T) {
	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("{\"status\":0}")),
			}, nil
		},
	}

	svc := NewWithClient(testLogger(), httpClient, "https://api.pushover.net")

	msg := models.PushoverMessage{
		Success: true,
		Host:    "server1",
	}

	result, err := svc.SendNotification(context.Background(), testConfig(), msg)

	require.NoError(t, err)
	assert.False(t, result.MessageSent)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "status 400")
}

func TestFormatMessage_Success(t *testing.T) {
	svc := New(testLogger())

	msg := models.PushoverMessage{
		Success:          true,
		Host:             "myserver",
		Repository:       "rest:http://backup.local:8000/data",
		StartTime:        time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Duration:         3*time.Minute + 45*time.Second,
		SnapshotID:       "abc123def456",
		FilesNew:         50,
		FilesChanged:     10,
		FilesUnmodified:  1000,
		DataAdded:        1024 * 1024 * 100, // 100 MB
		TotalFiles:       1060,
		TotalBytes:       1024 * 1024 * 1024 * 2, // 2 GB
		SnapshotsRemoved: 3,
		SnapshotsKept:    30,
	}

	title, body := svc.formatMessage(msg)

	assert.Equal(t, "Backup Successful", title)
	assert.Contains(t, body, "myserver")
	assert.Contains(t, body, "rest:http://backup.local:8000/data")
	assert.Contains(t, body, "abc123def456")
	assert.Contains(t, body, "Files new: 50")
	assert.Contains(t, body, "Files changed: 10")
	assert.Contains(t, body, "Files unmodified: 1000")
	assert.Contains(t, body, "Snapshots kept: 30")
	assert.Contains(t, body, "Snapshots removed: 3")
	// Verify no HTML tags
	assert.NotContains(t, body, "<b>")
	assert.NotContains(t, body, "<code>")
}

func TestFormatMessage_Failure(t *testing.T) {
	svc := New(testLogger())

	msg := models.PushoverMessage{
		Success:      false,
		Host:         "myserver",
		Repository:   "/backup",
		StartTime:    time.Now(),
		Duration:     1 * time.Minute,
		FailedStep:   "wol",
		ErrorMessage: "timeout waiting for target",
	}

	title, body := svc.formatMessage(msg)

	assert.Equal(t, "Backup Failed", title)
	assert.Contains(t, body, "Failed step: wol")
	assert.Contains(t, body, "timeout waiting for target")
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1024 * 1024 * 1024 * 2, "2.0 GiB"},
		{1536 * 1024, "1.5 MiB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSendNotification_ContextCancelled(t *testing.T) {
	httpClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, context.Canceled
		},
	}

	svc := NewWithClient(testLogger(), httpClient, "https://api.pushover.net")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := models.PushoverMessage{
		Success: true,
		Host:    "server1",
	}

	result, err := svc.SendNotification(ctx, testConfig(), msg)

	require.NoError(t, err)
	assert.False(t, result.MessageSent)
	assert.NotNil(t, result.Error)
}
