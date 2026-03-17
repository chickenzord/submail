package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListAllMessages_Empty(t *testing.T) {
	store := newTestStore(t)
	msgs, err := store.ListAllMessages(context.Background(), 50, 0)
	require.NoError(t, err)
	assert.NotNil(t, msgs)
	assert.Empty(t, msgs)
}

func TestListAllMessages_ReturnsAll(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<all1@t>", "Msg1", "bot+a@example.com", time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<all2@t>", "Msg2", "bot+b@example.com", 2*time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<all3@t>", "Msg3", "bot+c@example.com", 3*time.Hour)))

	msgs, err := store.ListAllMessages(ctx, 50, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 3)
}

func TestListAllMessages_OrderedByReceivedAtDesc(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<ao1@t>", "Old", "bot+a@example.com", 3*time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<ao2@t>", "New", "bot+b@example.com", time.Hour)))

	msgs, err := store.ListAllMessages(ctx, 50, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "New", msgs[0].Subject)
	assert.Equal(t, "Old", msgs[1].Subject)
}

func TestListAllMessages_Pagination(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	for i := range 5 {
		require.NoError(t, store.SaveMessage(ctx, sampleMsg(
			"<ap"+string(rune('A'+i))+"@t>", "Msg", "bot+a@example.com",
			time.Duration(5-i)*time.Hour,
		)))
	}

	page1, err := store.ListAllMessages(ctx, 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := store.ListAllMessages(ctx, 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	page3, err := store.ListAllMessages(ctx, 2, 4)
	require.NoError(t, err)
	assert.Len(t, page3, 1)

	seen := make(map[string]bool)
	for _, m := range append(append(page1, page2...), page3...) {
		assert.False(t, seen[m.ID], "duplicate ID: %s", m.ID)
		seen[m.ID] = true
	}
}

func TestCountAllMessages(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	n, err := store.CountAllMessages(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<ca1@t>", "1", "bot+a@example.com", time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<ca2@t>", "2", "bot+b@example.com", time.Hour)))
	require.NoError(t, store.SaveMessage(ctx, sampleMsg("<ca3@t>", "3", "bot+c@example.com", time.Hour)))

	n, err = store.CountAllMessages(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}
