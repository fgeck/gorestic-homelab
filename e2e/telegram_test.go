//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/fgeck/gorestic-homelab/internal/services/telegram"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTelegramConfig(t *testing.T) models.TelegramConfig {
	t.Helper()

	botToken := os.Getenv("TEST_TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		t.Skip("TEST_TELEGRAM_BOT_TOKEN not set")
	}

	chatID := os.Getenv("TEST_TELEGRAM_CHAT_ID")
	if chatID == "" {
		t.Skip("TEST_TELEGRAM_CHAT_ID not set")
	}

	return models.TelegramConfig{
		BotToken: botToken,
		ChatID:   chatID,
	}
}

func TestTelegramSendSuccessNotification_E2E(t *testing.T) {
	cfg := getTelegramConfig(t)

	svc := telegram.New(testLogger())

	msg := models.TelegramMessage{
		Success:          true,
		Host:             "e2e-test-host",
		Repository:       "rest:http://backup.local:8000/test",
		StartTime:        time.Now().Add(-5 * time.Minute),
		Duration:         5 * time.Minute,
		SnapshotID:       "abc123def456",
		FilesNew:         100,
		FilesChanged:     50,
		FilesUnmodified:  5000,
		DataAdded:        1024 * 1024 * 50, // 50 MB
		TotalFiles:       5150,
		TotalBytes:       1024 * 1024 * 1024 * 2, // 2 GB
		SnapshotsRemoved: 5,
		SnapshotsKept:    30,
	}

	result, err := svc.SendNotification(context.Background(), cfg, msg)

	require.NoError(t, err)
	assert.True(t, result.MessageSent)
	assert.Nil(t, result.Error)
}

func TestTelegramSendFailureNotification_E2E(t *testing.T) {
	cfg := getTelegramConfig(t)

	svc := telegram.New(testLogger())

	msg := models.TelegramMessage{
		Success:      false,
		Host:         "e2e-test-host",
		Repository:   "rest:http://backup.local:8000/test",
		StartTime:    time.Now().Add(-2 * time.Minute),
		Duration:     2 * time.Minute,
		FailedStep:   "backup",
		ErrorMessage: "connection refused to backup server",
	}

	result, err := svc.SendNotification(context.Background(), cfg, msg)

	require.NoError(t, err)
	assert.True(t, result.MessageSent)
	assert.Nil(t, result.Error)
}

func TestTelegramInvalidToken_E2E(t *testing.T) {
	cfg := models.TelegramConfig{
		BotToken: "invalid:token",
		ChatID:   "-100123456789",
	}

	svc := telegram.New(testLogger())

	msg := models.TelegramMessage{
		Success: true,
		Host:    "test",
	}

	result, err := svc.SendNotification(context.Background(), cfg, msg)

	require.NoError(t, err)
	assert.False(t, result.MessageSent)
	assert.NotNil(t, result.Error)
}

func TestTelegramInvalidChatID_E2E(t *testing.T) {
	botToken := os.Getenv("TEST_TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		t.Skip("TEST_TELEGRAM_BOT_TOKEN not set")
	}

	cfg := models.TelegramConfig{
		BotToken: botToken,
		ChatID:   "invalid-chat-id",
	}

	svc := telegram.New(testLogger())

	msg := models.TelegramMessage{
		Success: true,
		Host:    "test",
	}

	result, err := svc.SendNotification(context.Background(), cfg, msg)

	require.NoError(t, err)
	assert.False(t, result.MessageSent)
	assert.NotNil(t, result.Error)
}
