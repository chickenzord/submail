// Package imap provides an IMAP ingester that monitors a mailbox and stores
// new messages into a [storage.Store].
package imap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/chickenzord/submail/internal/config"
	"github.com/chickenzord/submail/internal/storage"
	goimap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"
	_ "github.com/emersion/go-message/charset" // register extended charset decoders
)

const (
	batchSize      = 50
	initialBackoff = 2 * time.Second
	maxBackoff     = 5 * time.Minute
)

// Ingester connects to an IMAP server and ingests new messages into storage.
type Ingester struct {
	cfg       config.IMAPConfig
	addresses []string // flat, deduplicated list of agent addresses to monitor
	store     storage.Store
	log       *slog.Logger
}

// New creates a new Ingester. addresses is the flat, deduplicated list of agent
// email addresses; only messages delivered to one of these addresses are
// fetched and stored.
func New(cfg config.IMAPConfig, addresses []string, store storage.Store) *Ingester {
	return &Ingester{
		cfg:       cfg,
		addresses: addresses,
		store:     store,
		log:       slog.Default().With("component", "imap-ingester"),
	}
}

// Run runs the ingester loop until ctx is cancelled. It reconnects automatically
// on connection errors with exponential back-off.
func (ing *Ingester) Run(ctx context.Context) error {
	backoff := initialBackoff
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		ing.log.Info("connecting to IMAP", "host", ing.cfg.Host, "port", ing.cfg.Port)
		err := ing.runSession(ctx)
		if ctx.Err() != nil {
			// Context cancelled — clean exit, not an error.
			return nil
		}
		if err != nil {
			ing.log.Error("IMAP session ended", "err", err, "reconnect_in", backoff)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connect dials the IMAP server, logs in, and wires up a channel that is
// signalled when the server reports new messages (unilateral EXISTS update).
func (ing *Ingester) connect(newMailCh chan<- struct{}) (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", ing.cfg.Host, ing.cfg.Port)

	opts := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages != nil {
					// Non-blocking send; if the channel is already full we
					// will still pick up the new mail on the next sync.
					select {
					case newMailCh <- struct{}{}:
					default:
					}
				}
			},
		},
	}

	var (
		c   *imapclient.Client
		err error
	)

	switch ing.cfg.TLSMode {
	case config.TLSModeImplicit:
		c, err = imapclient.DialTLS(addr, opts)
	case config.TLSModeSTARTTLS:
		c, err = imapclient.DialStartTLS(addr, opts)
	case config.TLSModeNone:
		c, err = imapclient.DialInsecure(addr, opts)
	default:
		c, err = imapclient.DialTLS(addr, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	if err := c.Login(ing.cfg.Username, ing.cfg.Password).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("login: %w", err)
	}

	return c, nil
}

// runSession opens one IMAP connection, performs the initial sync, and then
// loops using IDLE (when supported) or periodic polling to pick up new mail.
func (ing *Ingester) runSession(ctx context.Context) error {
	newMailCh := make(chan struct{}, 1)

	c, err := ing.connect(newMailCh)
	if err != nil {
		return err
	}
	defer c.Close()

	ing.log.Info("IMAP session established", "host", ing.cfg.Host, "mailbox", ing.cfg.Mailbox)

	if _, err := c.Select(ing.cfg.Mailbox, nil).Wait(); err != nil {
		return fmt.Errorf("select %q: %w", ing.cfg.Mailbox, err)
	}

	// Initial full sync — fetches everything the mailbox contains.
	lastUID, err := ing.syncMessages(ctx, c, 0)
	if err != nil {
		return fmt.Errorf("initial sync: %w", err)
	}

	// Prefer IDLE for real-time notification; fall back to pure polling.
	supportsIdle := c.Caps().Has(goimap.CapIdle)

	for {
		if supportsIdle {
			lastUID, err = ing.idleUntilNew(ctx, c, newMailCh, lastUID)
		} else {
			lastUID, err = ing.pollOnce(ctx, c, lastUID)
		}
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// idleUntilNew starts an IMAP IDLE and waits for a new-mail signal, the poll
// interval, or context cancellation — then syncs new messages.
func (ing *Ingester) idleUntilNew(
	ctx context.Context,
	c *imapclient.Client,
	newMailCh <-chan struct{},
	lastUID goimap.UID,
) (goimap.UID, error) {
	idleCmd, err := c.Idle()
	if err != nil {
		return lastUID, fmt.Errorf("IDLE: %w", err)
	}

	select {
	case <-ctx.Done():
		_ = idleCmd.Close()
		return lastUID, nil

	case <-newMailCh:
		// Server signalled new mail — stop IDLE and sync immediately.

	case <-time.After(ing.cfg.PollInterval):
		// Periodic check even if no push notification arrived.
	}

	if err := idleCmd.Close(); err != nil {
		return lastUID, fmt.Errorf("stop IDLE: %w", err)
	}

	// Re-select to refresh mailbox state before searching.
	if _, err := c.Select(ing.cfg.Mailbox, nil).Wait(); err != nil {
		return lastUID, fmt.Errorf("re-select %q: %w", ing.cfg.Mailbox, err)
	}

	return ing.syncMessages(ctx, c, lastUID)
}

// pollOnce sleeps for the poll interval and then syncs.
func (ing *Ingester) pollOnce(
	ctx context.Context,
	c *imapclient.Client,
	lastUID goimap.UID,
) (goimap.UID, error) {
	select {
	case <-ctx.Done():
		return lastUID, nil
	case <-time.After(ing.cfg.PollInterval):
	}

	if _, err := c.Select(ing.cfg.Mailbox, nil).Wait(); err != nil {
		return lastUID, fmt.Errorf("re-select %q: %w", ing.cfg.Mailbox, err)
	}

	return ing.syncMessages(ctx, c, lastUID)
}

// syncMessages searches for messages newer than afterUID (or all messages when
// afterUID==0), fetches and ingests any that are not yet in storage, and returns
// the highest UID encountered.
func (ing *Ingester) syncMessages(ctx context.Context, c *imapclient.Client, afterUID goimap.UID) (goimap.UID, error) {
	criteria := ing.baseCriteria()
	if afterUID > 0 {
		var uidSet goimap.UIDSet
		// Stop: 0 represents "*" (the last UID in the mailbox).
		uidSet.AddRange(afterUID+1, 0)
		criteria.UID = []goimap.UIDSet{uidSet}
	}

	searchData, err := c.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return afterUID, fmt.Errorf("UID SEARCH: %w", err)
	}

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return afterUID, nil
	}

	ing.log.Info("found messages to sync", "count", len(uids))

	maxUID := afterUID
	for i := 0; i < len(uids); i += batchSize {
		if ctx.Err() != nil {
			return maxUID, nil
		}

		end := i + batchSize
		if end > len(uids) {
			end = len(uids)
		}

		uid, err := ing.fetchAndIngest(ctx, c, uids[i:end])
		if err != nil {
			return maxUID, err
		}
		if uid > maxUID {
			maxUID = uid
		}
	}

	return maxUID, nil
}

// fetchAndIngest processes a batch of UIDs in two stages to avoid downloading
// message bodies that are already in storage:
//
//  1. Fetch envelopes only (cheap) — use Message-ID to identify unseen messages.
//  2. Fetch full bodies only for the unseen subset.
//
// It returns the highest UID in the batch regardless of whether any new messages
// were saved, so the caller's lastUID watermark always advances.
func (ing *Ingester) fetchAndIngest(ctx context.Context, c *imapclient.Client, uids []goimap.UID) (goimap.UID, error) {
	// UIDs returned by SEARCH are in ascending order per the IMAP spec.
	maxUID := uids[len(uids)-1]

	// Stage 1 — envelope scan: identify which UIDs are not yet in storage.
	newUIDs, err := ing.filterKnownUIDs(ctx, c, uids)
	if err != nil {
		return maxUID, err
	}

	if len(newUIDs) == 0 {
		ing.log.Debug("batch already fully ingested", "uids", len(uids), "max_uid", maxUID)
		return maxUID, nil
	}

	ing.log.Info("fetching bodies for new messages", "new", len(newUIDs), "total_in_batch", len(uids))

	// Stage 2 — body fetch: download and save only the new messages.
	if err := ing.fetchBodiesAndSave(ctx, c, newUIDs); err != nil {
		return maxUID, err
	}

	return maxUID, nil
}

// filterKnownUIDs fetches envelope metadata for the given UIDs and returns only
// those whose Message-ID is not yet present in storage. This avoids downloading
// full message bodies for already-ingested messages on restart.
func (ing *Ingester) filterKnownUIDs(ctx context.Context, c *imapclient.Client, uids []goimap.UID) ([]goimap.UID, error) {
	uidSet := goimap.UIDSetNum(uids...)
	fetchOpts := &goimap.FetchOptions{
		UID:          true,
		Envelope:     true,
		InternalDate: true,
	}

	msgs, err := c.Fetch(uidSet, fetchOpts).Collect()
	if err != nil {
		return nil, fmt.Errorf("fetch envelopes: %w", err)
	}

	var newUIDs []goimap.UID
	for _, msg := range msgs {
		if msg.Envelope == nil {
			continue
		}
		messageID := envelopeMessageID(msg, ing.cfg.Host)
		exists, err := ing.store.MessageExists(ctx, messageID)
		if err != nil {
			return nil, fmt.Errorf("check existence for UID %d: %w", msg.UID, err)
		}
		if !exists {
			newUIDs = append(newUIDs, msg.UID)
		}
	}

	return newUIDs, nil
}

// fetchBodiesAndSave downloads the full RFC 822 body for each UID and saves it
// to storage. At this point the caller has already confirmed the messages are
// new, but the ON CONFLICT clause in SaveMessage acts as a final safety net
// against any concurrent duplicate.
func (ing *Ingester) fetchBodiesAndSave(ctx context.Context, c *imapclient.Client, uids []goimap.UID) error {
	uidSet := goimap.UIDSetNum(uids...)
	fetchOpts := &goimap.FetchOptions{
		UID:          true,
		Envelope:     true,
		InternalDate: true,
		BodySection: []*goimap.FetchItemBodySection{
			// BODY.PEEK[] — full raw message without setting the \Seen flag.
			{Specifier: goimap.PartSpecifierNone, Peek: true},
		},
	}

	msgs, err := c.Fetch(uidSet, fetchOpts).Collect()
	if err != nil {
		return fmt.Errorf("fetch bodies: %w", err)
	}

	for _, msg := range msgs {
		if err := ing.save(ctx, msg); err != nil {
			ing.log.Error("failed to save message", "uid", msg.UID, "err", err)
			// Continue — one bad message must not abort the whole batch.
		}
	}
	return nil
}

// save parses a fully-fetched message buffer and writes it to storage.
// The caller is responsible for ensuring the message is not already stored;
// SaveMessage's ON CONFLICT clause handles any remaining race.
func (ing *Ingester) save(ctx context.Context, msg *imapclient.FetchMessageBuffer) error {
	if msg.Envelope == nil {
		return nil
	}

	messageID := envelopeMessageID(msg, ing.cfg.Host)

	// Locate the raw message bytes from the body section buffer.
	var rawBytes []byte
	for _, sec := range msg.BodySection {
		rawBytes = sec.Bytes
		break
	}
	if rawBytes == nil {
		return fmt.Errorf("no body section returned for UID %d", msg.UID)
	}

	textBody, htmlBody, err := parseBodies(rawBytes)
	if err != nil {
		// Log but do not abort — store what we know from the envelope.
		ing.log.Warn("could not parse message body", "uid", msg.UID, "err", err)
	}

	receivedAt := msg.InternalDate
	if receivedAt.IsZero() {
		receivedAt = msg.Envelope.Date
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}

	storageMsg := &storage.Message{
		MessageID:  messageID,
		Subject:    msg.Envelope.Subject,
		From:       formatAddress(msg.Envelope.From),
		To:         formatAddress(msg.Envelope.To),
		ReceivedAt: receivedAt.UTC(),
		TextBody:   textBody,
		HTMLBody:   htmlBody,
	}

	if err := ing.store.SaveMessage(ctx, storageMsg); err != nil {
		return fmt.Errorf("save message: %w", err)
	}

	ing.log.Info("ingested message",
		"uid", msg.UID,
		"message_id", messageID,
		"subject", msg.Envelope.Subject,
		"to", storageMsg.To,
	)
	return nil
}

// parseBodies walks the MIME tree of rawMsg and extracts the plain-text and
// HTML body parts.
func parseBodies(rawMsg []byte) (textBody, htmlBody string, err error) {
	mr, err := mail.CreateReader(bytes.NewReader(rawMsg))
	if err != nil {
		return "", "", fmt.Errorf("create mail reader: %w", err)
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Some parts may be malformed; stop iterating but return what we have.
			break
		}

		// part.Header is a PartHeader interface; use Get to read Content-Type.
		ct := strings.ToLower(strings.SplitN(part.Header.Get("Content-Type"), ";", 2)[0])
		ct = strings.TrimSpace(ct)
		body, _ := io.ReadAll(part.Body)

		switch ct {
		case "text/plain":
			textBody = string(body)
		case "text/html":
			htmlBody = string(body)
		}
	}

	return textBody, htmlBody, nil
}

// baseCriteria returns a SearchCriteria that restricts results to messages
// addressed to any of the configured agent addresses. All other constraints
// (UID range, SINCE, …) are layered on top by the caller.
//
// With one address:   HEADER "To" "addr"
// With many:          OR (HEADER "To" "addr0") (OR (HEADER "To" "addr1") …)
func (ing *Ingester) baseCriteria() *goimap.SearchCriteria {
	switch len(ing.addresses) {
	case 0:
		return &goimap.SearchCriteria{} // no filter – match everything
	case 1:
		return &goimap.SearchCriteria{
			Header: []goimap.SearchCriteriaHeaderField{
				{Key: "To", Value: ing.addresses[0]},
			},
		}
	default:
		return &goimap.SearchCriteria{
			Or: [][2]goimap.SearchCriteria{toOrPair(ing.addresses)},
		}
	}
}

// toOrPair recursively builds OR(addr[0], OR(addr[1], …)) from a slice of
// addresses. len(addresses) must be >= 2.
func toOrPair(addresses []string) [2]goimap.SearchCriteria {
	left := goimap.SearchCriteria{
		Header: []goimap.SearchCriteriaHeaderField{{Key: "To", Value: addresses[0]}},
	}
	if len(addresses) == 2 {
		return [2]goimap.SearchCriteria{left, {
			Header: []goimap.SearchCriteriaHeaderField{{Key: "To", Value: addresses[1]}},
		}}
	}
	return [2]goimap.SearchCriteria{left, {
		Or: [][2]goimap.SearchCriteria{toOrPair(addresses[1:])},
	}}
}

// envelopeMessageID returns the canonical Message-ID for deduplication.
// When the envelope carries no Message-ID (rare but valid), a stable synthetic
// ID is derived from the IMAP UID so restarts still produce the same key.
func envelopeMessageID(msg *imapclient.FetchMessageBuffer, host string) string {
	if id := strings.TrimSpace(msg.Envelope.MessageID); id != "" {
		return id
	}
	return fmt.Sprintf("uid-%d@%s", msg.UID, host)
}

// formatAddress returns the first address in addrs as a plain email string
// ("user@host"), which is the format used for to_addr matching in storage.
func formatAddress(addrs []goimap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	a := addrs[0]
	return strings.ToLower(a.Mailbox) + "@" + strings.ToLower(a.Host)
}
