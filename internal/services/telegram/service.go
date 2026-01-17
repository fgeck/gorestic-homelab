// Package telegram provides Telegram notification services.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
)

// Service defines the interface for Telegram notification operations.
type Service interface {
	SendNotification(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error)
}

// HTTPClient allows mocking HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Impl implements the Telegram Service interface.
type Impl struct {
	httpClient HTTPClient
	logger     zerolog.Logger
	baseURL    string
}

// New creates a new Telegram service.
func New(logger zerolog.Logger) *Impl {
	return &Impl{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:  logger,
		baseURL: "https://api.telegram.org",
	}
}

// NewWithClient creates a new Telegram service with a custom HTTP client (for testing).
func NewWithClient(logger zerolog.Logger, httpClient HTTPClient, baseURL string) *Impl {
	return &Impl{
		httpClient: httpClient,
		logger:     logger,
		baseURL:    baseURL,
	}
}

// sendMessageRequest is the request body for Telegram sendMessage API.
type sendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// SendNotification sends a backup notification via Telegram.
func (s *Impl) SendNotification(ctx context.Context, cfg models.TelegramConfig, msg models.TelegramMessage) (*models.TelegramResult, error) {
	result := &models.TelegramResult{}

	s.logger.Info().
		Str("chat_id", cfg.ChatID).
		Bool("success", msg.Success).
		Msg("sending Telegram notification")

	// Format message
	text := s.formatMessage(msg)

	// Build request
	reqBody := sendMessageRequest{
		ChatID:    cfg.ChatID,
		Text:      text,
		ParseMode: "HTML",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		result.Error = fmt.Errorf("failed to marshal request: %w", err)
		return result, nil
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", s.baseURL, cfg.BotToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		result.Error = fmt.Errorf("failed to create request: %w", err)
		return result, nil
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("failed to send request: %w", err)
		return result, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("telegram API returned status %d", resp.StatusCode)
		return result, nil
	}

	result.MessageSent = true
	s.logger.Info().Msg("Telegram notification sent successfully")

	return result, nil
}

func (s *Impl) formatMessage(msg models.TelegramMessage) string {
	var b bytes.Buffer

	if msg.Success {
		b.WriteString("✅ <b>Backup Successful</b>\n\n")
	} else {
		b.WriteString("❌ <b>Backup Failed</b>\n\n")
	}

	// Basic info
	b.WriteString(fmt.Sprintf("🖥 <b>Host:</b> %s\n", escapeHTML(msg.Host)))
	b.WriteString(fmt.Sprintf("📁 <b>Repository:</b> %s\n", escapeHTML(msg.Repository)))
	b.WriteString(fmt.Sprintf("⏰ <b>Started:</b> %s\n", msg.StartTime.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("⏱ <b>Duration:</b> %s\n", msg.Duration.Round(time.Second)))

	if msg.Success {
		b.WriteString("\n<b>📊 Backup Statistics:</b>\n")
		b.WriteString(fmt.Sprintf("  • Snapshot: <code>%s</code>\n", msg.SnapshotID))
		b.WriteString(fmt.Sprintf("  • Files new: %d\n", msg.FilesNew))
		b.WriteString(fmt.Sprintf("  • Files changed: %d\n", msg.FilesChanged))
		b.WriteString(fmt.Sprintf("  • Files unmodified: %d\n", msg.FilesUnmodified))
		b.WriteString(fmt.Sprintf("  • Data added: %s\n", formatBytes(msg.DataAdded)))
		b.WriteString(fmt.Sprintf("  • Total files: %d\n", msg.TotalFiles))
		b.WriteString(fmt.Sprintf("  • Total size: %s\n", formatBytes(msg.TotalBytes)))

		if msg.SnapshotsRemoved > 0 || msg.SnapshotsKept > 0 {
			b.WriteString("\n<b>🗑 Retention:</b>\n")
			b.WriteString(fmt.Sprintf("  • Snapshots kept: %d\n", msg.SnapshotsKept))
			b.WriteString(fmt.Sprintf("  • Snapshots removed: %d\n", msg.SnapshotsRemoved))
		}
	} else {
		b.WriteString("\n<b>⚠️ Error Details:</b>\n")
		b.WriteString(fmt.Sprintf("  • Failed step: %s\n", escapeHTML(msg.FailedStep)))
		b.WriteString(fmt.Sprintf("  • Error: <code>%s</code>\n", escapeHTML(msg.ErrorMessage)))
	}

	return b.String()
}

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// formatBytes formats bytes into human-readable format.
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
