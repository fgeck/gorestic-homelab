// Package pushover provides Pushover notification services.
package pushover

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
)

// Service defines the interface for Pushover notification operations.
type Service interface {
	SendNotification(ctx context.Context, cfg models.PushoverConfig, msg models.PushoverMessage) (*models.PushoverResult, error)
}

// HTTPClient allows mocking HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Impl implements the Pushover Service interface.
type Impl struct {
	httpClient HTTPClient
	logger     zerolog.Logger
	baseURL    string
}

// New creates a new Pushover service.
func New(logger zerolog.Logger) *Impl {
	return &Impl{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:  logger,
		baseURL: "https://api.pushover.net",
	}
}

// NewWithClient creates a new Pushover service with a custom HTTP client (for testing).
func NewWithClient(logger zerolog.Logger, httpClient HTTPClient, baseURL string) *Impl {
	return &Impl{
		httpClient: httpClient,
		logger:     logger,
		baseURL:    baseURL,
	}
}

// SendNotification sends a backup notification via Pushover.
func (s *Impl) SendNotification(ctx context.Context, cfg models.PushoverConfig, msg models.PushoverMessage) (*models.PushoverResult, error) {
	result := &models.PushoverResult{}

	s.logger.Info().
		Bool("success", msg.Success).
		Msg("sending Pushover notification")

	// Format message
	title, body := s.formatMessage(msg)

	// Build form data
	form := url.Values{}
	form.Set("token", cfg.AppToken)
	form.Set("user", cfg.UserKey)
	form.Set("title", title)
	form.Set("message", body)
	form.Set("priority", strconv.Itoa(cfg.Priority))

	apiURL := fmt.Sprintf("%s/1/messages.json", s.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		result.Error = fmt.Errorf("failed to create request: %w", err)
		return result, nil
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("failed to send request: %w", err)
		return result, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("pushover API returned status %d", resp.StatusCode)
		return result, nil
	}

	result.MessageSent = true
	s.logger.Info().Msg("Pushover notification sent successfully")

	return result, nil
}

func (s *Impl) formatMessage(msg models.PushoverMessage) (string, string) {
	var title string
	if msg.Success {
		title = "Backup Successful"
	} else {
		title = "Backup Failed"
	}

	var b bytes.Buffer

	b.WriteString(fmt.Sprintf("Host: %s\n", msg.Host))
	b.WriteString(fmt.Sprintf("Repository: %s\n", msg.Repository))
	b.WriteString(fmt.Sprintf("Started: %s\n", msg.StartTime.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("Duration: %s\n", msg.Duration.Round(time.Second)))

	if msg.Success {
		b.WriteString("\nBackup Statistics:\n")
		b.WriteString(fmt.Sprintf("  Snapshot: %s\n", msg.SnapshotID))
		b.WriteString(fmt.Sprintf("  Files new: %d\n", msg.FilesNew))
		b.WriteString(fmt.Sprintf("  Files changed: %d\n", msg.FilesChanged))
		b.WriteString(fmt.Sprintf("  Files unmodified: %d\n", msg.FilesUnmodified))
		b.WriteString(fmt.Sprintf("  Data added: %s\n", formatBytes(msg.DataAdded)))
		b.WriteString(fmt.Sprintf("  Total files: %d\n", msg.TotalFiles))
		b.WriteString(fmt.Sprintf("  Total size: %s\n", formatBytes(msg.TotalBytes)))

		if msg.SnapshotsRemoved > 0 || msg.SnapshotsKept > 0 {
			b.WriteString("\nRetention:\n")
			b.WriteString(fmt.Sprintf("  Snapshots kept: %d\n", msg.SnapshotsKept))
			b.WriteString(fmt.Sprintf("  Snapshots removed: %d\n", msg.SnapshotsRemoved))
		}
	} else {
		b.WriteString("\nError Details:\n")
		b.WriteString(fmt.Sprintf("  Failed step: %s\n", msg.FailedStep))
		b.WriteString(fmt.Sprintf("  Error: %s\n", msg.ErrorMessage))
	}

	return title, b.String()
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
