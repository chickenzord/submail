package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/chickenzord/submail/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) storage.Store {
	t.Helper()
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func sampleMsg(messageID, subject, to string, age time.Duration) *storage.Message {
	return &storage.Message{
		MessageID:  messageID,
		Subject:    subject,
		From:       "sender@example.com",
		To:         to,
		ReceivedAt: time.Now().UTC().Add(-age),
		TextBody:   "text body",
		HTMLBody:   "<p>html body</p>",
	}
}

func TestSaveAndGetMessage(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	msg := sampleMsg("<test@example.com>", "Hello", "bot+a@example.com", time.Hour)
	require.NoError(t, store.SaveMessage(ctx, msg))
	require.NotEmpty(t, msg.ID, "SaveMessage should assign an ID when empty")

	got, err := store.GetMessage(ctx, msg.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, msg.ID, got.ID)
	assert.Equal(t, msg.MessageID, got.MessageID)
	assert.Equal(t, msg.Subject, got.Subject)
	assert.Equal(t, msg.From, got.From)
	assert.Equal(t, msg.To, got.To)
	assert.Equal(t, msg.TextBody, got.TextBody)
	assert.Equal(t, msg.HTMLBody, got.HTMLBody)
	assert.WithinDuration(t, msg.ReceivedAt, got.ReceivedAt, time.Second)
}

func TestGetMessage_NotFound(t *testing.T) {
	store := newTestStore(t)
	got, err := store.GetMessage(context.Background(), "nonexistent-id")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSaveMessage_UpsertByMessageID(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	msg := sampleMsg("<dup@example.com>", "Original", "bot+a@example.com", time.Hour)
	require.NoError(t, store.SaveMessage(ctx, msg))
	originalID := msg.ID

	// Save again with same MessageID but updated subject
	msg.ID = "" // reset so a new UUID would be generated if insert
	msg.Subject = "Updated"
	require.NoError(t, store.SaveMessage(ctx, msg))

	// ID should remain the original (upsert, not insert)
	got, err := store.GetMessage(ctx, originalID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Updated", got.Subject)

	// No duplicate rows
	count, err := store.CountMessages(ctx, []string{"bot+a@example.com"})
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestListMessages_FiltersByAddress(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<a1@t>", "A1", "bot+a@example.com", 3*time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<a2@t>", "A2", "bot+a@example.com", 2*time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<b1@t>", "B1", "bot+b@example.com", time.Hour)))

	msgs, err := store.ListMessages(ctx, []string{"bot+a@example.com"}, 50, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	for _, m := range msgs {
		assert.Equal(t, "bot+a@example.com", m.To)
	}
}

func TestListMessages_OrderedByReceivedAtDesc(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<old@t>", "Old", "bot+a@example.com", 3*time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<new@t>", "New", "bot+a@example.com", time.Hour)))

	msgs, err := store.ListMessages(ctx, []string{"bot+a@example.com"}, 50, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "New", msgs[0].Subject)
	assert.Equal(t, "Old", msgs[1].Subject)
}

func TestListMessages_Pagination(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	for i := range 5 {
		require.NoError(t, store.SaveMessage(ctx, sampleMsg(
			"<pg"+string(rune('A'+i))+"@t>",
			"Msg",
			"bot+a@example.com",
			time.Duration(5-i)*time.Hour,
		)))
	}

	page1, err := store.ListMessages(ctx, []string{"bot+a@example.com"}, 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := store.ListMessages(ctx, []string{"bot+a@example.com"}, 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	page3, err := store.ListMessages(ctx, []string{"bot+a@example.com"}, 2, 4)
	require.NoError(t, err)
	assert.Len(t, page3, 1)

	// All IDs are unique across pages
	seen := make(map[string]bool)
	for _, m := range append(append(page1, page2...), page3...) {
		assert.False(t, seen[m.ID], "duplicate ID across pages: %s", m.ID)
		seen[m.ID] = true
	}
}

func TestListMessages_EmptyAddresses(t *testing.T) {
	store := newTestStore(t)
	msgs, err := store.ListMessages(context.Background(), nil, 50, 0)
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestCountMessages(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<c1@t>", "1", "bot+a@example.com", time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<c2@t>", "2", "bot+a@example.com", time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<c3@t>", "3", "bot+b@example.com", time.Hour)))

	n, err := store.CountMessages(ctx, []string{"bot+a@example.com"})
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	n, err = store.CountMessages(ctx, []string{"bot+a@example.com", "bot+b@example.com"})
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	n, err = store.CountMessages(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestMessageExists(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	exists, err := store.MessageExists(ctx, "<notyet@t>")
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<notyet@t>", "s", "bot+a@example.com", time.Hour)))

	exists, err = store.MessageExists(ctx, "<notyet@t>")
	require.NoError(t, err)
	assert.True(t, exists)
}
