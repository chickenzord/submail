package storage

import (
	"context"
	"time"
)

// Message represents a stored email message.
type Message struct {
	ID         string    `json:"id"`
	MessageID  string    `json:"message_id"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	ReceivedAt time.Time `json:"received_at"`
	TextBody   string    `json:"text_body,omitempty"`
	HTMLBody   string    `json:"html_body,omitempty"`
}

// Store defines the storage interface for messages.
type Store interface {
	// SaveMessage inserts or updates a message. Uses MessageID for deduplication.
	// If msg.ID is empty, a UUID is generated.
	SaveMessage(ctx context.Context, msg *Message) error

	// GetMessage retrieves a message by its storage ID.
	GetMessage(ctx context.Context, id string) (*Message, error)

	// ListMessages retrieves messages delivered to any of the given addresses,
	// ordered by received_at descending.
	ListMessages(ctx context.Context, addresses []string, limit, offset int) ([]*Message, error)

	// CountMessages returns the total number of messages delivered to any of the given addresses.
	CountMessages(ctx context.Context, addresses []string) (int, error)

	// MessageExists checks whether a message with the given email Message-ID exists.
	MessageExists(ctx context.Context, messageID string) (bool, error)

	// Close releases resources held by the store.
	Close() error
}
