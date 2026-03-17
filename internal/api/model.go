package api

import (
	"time"

	"github.com/chickenzord/submail/internal/storage"
)

// Mail is the API representation of an email message.
type Mail struct {
	ID         string    `json:"id"`
	MessageID  string    `json:"message_id"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	ReceivedAt time.Time `json:"received_at"`
	TextBody   string    `json:"text_body,omitempty"`
	HTMLBody   string    `json:"html_body,omitempty"`
}

// ListMailsResponse is the envelope returned by GET /inbox.
type ListMailsResponse struct {
	Mails  []*Mail `json:"mails"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

func mailFromMessage(m *storage.Message) *Mail {
	return &Mail{
		ID:         m.ID,
		MessageID:  m.MessageID,
		Subject:    m.Subject,
		From:       m.From,
		To:         m.To,
		ReceivedAt: m.ReceivedAt,
		TextBody:   m.TextBody,
		HTMLBody:   m.HTMLBody,
	}
}
