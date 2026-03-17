package api_test

import (
	"testing"
	"time"

	"github.com/chickenzord/submail/internal/api"
	"github.com/chickenzord/submail/internal/storage"
	"github.com/stretchr/testify/assert"
)

func TestMailFromMessage(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	msg := &storage.Message{
		ID:         "abc123",
		MessageID:  "<unique@mail.example.com>",
		Subject:    "Hello",
		From:       "sender@example.com",
		To:         "bot+agent@example.com",
		ReceivedAt: ts,
		TextBody:   "plain text",
		HTMLBody:   "<p>html</p>",
	}

	mail := api.MailFromMessage(msg)

	assert.Equal(t, msg.ID, mail.ID)
	assert.Equal(t, msg.MessageID, mail.MessageID)
	assert.Equal(t, msg.Subject, mail.Subject)
	assert.Equal(t, msg.From, mail.From)
	assert.Equal(t, msg.To, mail.To)
	assert.Equal(t, msg.ReceivedAt, mail.ReceivedAt)
	assert.Equal(t, msg.TextBody, mail.TextBody)
	assert.Equal(t, msg.HTMLBody, mail.HTMLBody)
}
