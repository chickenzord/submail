package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// SQLiteStore is a Store backed by SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at path and runs migrations.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// A single connection serialises all goroutine access without any locking
	// overhead. Because only one process ever opens this file, WAL mode and
	// busy_timeout are not needed.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0) // keep the connection open for the lifetime of the process

	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		PRAGMA synchronous=NORMAL;
		PRAGMA foreign_keys=ON;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite pragmas: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS messages (
			id          TEXT PRIMARY KEY,
			message_id  TEXT UNIQUE NOT NULL,
			subject     TEXT NOT NULL DEFAULT '',
			from_addr   TEXT NOT NULL DEFAULT '',
			to_addr     TEXT NOT NULL DEFAULT '',
			received_at DATETIME NOT NULL,
			text_body   TEXT NOT NULL DEFAULT '',
			html_body   TEXT NOT NULL DEFAULT '',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_messages_to_addr
			ON messages(to_addr);

		CREATE INDEX IF NOT EXISTS idx_messages_received_at
			ON messages(received_at DESC);
	`)
	return err
}

func (s *SQLiteStore) SaveMessage(ctx context.Context, msg *Message) error {
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (id, message_id, subject, from_addr, to_addr, received_at, text_body, html_body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id) DO UPDATE SET
			subject     = excluded.subject,
			from_addr   = excluded.from_addr,
			to_addr     = excluded.to_addr,
			received_at = excluded.received_at,
			text_body   = excluded.text_body,
			html_body   = excluded.html_body
	`,
		msg.ID, msg.MessageID, msg.Subject, msg.From, msg.To,
		msg.ReceivedAt.UTC(), msg.TextBody, msg.HTMLBody,
	)
	return err
}

func (s *SQLiteStore) GetMessage(ctx context.Context, id string) (*Message, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, message_id, subject, from_addr, to_addr, received_at, text_body, html_body
		FROM messages WHERE id = ?
	`, id)
	return scanMessage(row)
}

func (s *SQLiteStore) ListMessages(ctx context.Context, addresses []string, limit, offset int) ([]*Message, error) {
	if len(addresses) == 0 {
		return []*Message{}, nil
	}

	placeholders := strings.Repeat("?,", len(addresses))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, len(addresses)+2)
	for i, a := range addresses {
		args[i] = a
	}
	args[len(addresses)] = limit
	args[len(addresses)+1] = offset

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, message_id, subject, from_addr, to_addr, received_at, text_body, html_body
		FROM messages
		WHERE to_addr IN (%s)
		ORDER BY received_at DESC
		LIMIT ? OFFSET ?
	`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if messages == nil {
		messages = []*Message{}
	}
	return messages, rows.Err()
}

func (s *SQLiteStore) CountMessages(ctx context.Context, addresses []string) (int, error) {
	if len(addresses) == 0 {
		return 0, nil
	}

	placeholders := strings.Repeat("?,", len(addresses))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, len(addresses))
	for i, a := range addresses {
		args[i] = a
	}

	var count int
	err := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM messages WHERE to_addr IN (%s)
	`, placeholders), args...).Scan(&count)
	return count, err
}

func (s *SQLiteStore) ListAllMessages(ctx context.Context, limit, offset int) ([]*Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, message_id, subject, from_addr, to_addr, received_at, text_body, html_body
		FROM messages
		ORDER BY received_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if messages == nil {
		messages = []*Message{}
	}
	return messages, rows.Err()
}

func (s *SQLiteStore) CountAllMessages(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&count)
	return count, err
}

func (s *SQLiteStore) MessageExists(ctx context.Context, messageID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM messages WHERE message_id = ?`, messageID,
	).Scan(&count)
	return count > 0, err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMessage(s scanner) (*Message, error) {
	var msg Message
	err := s.Scan(
		&msg.ID, &msg.MessageID, &msg.Subject,
		&msg.From, &msg.To, &msg.ReceivedAt,
		&msg.TextBody, &msg.HTMLBody,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &msg, nil
}
