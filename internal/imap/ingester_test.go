package imap

import (
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- parseBodies ---

func TestParseBodies_PlainTextOnly(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHello world\r\n"
	text, html, err := parseBodies([]byte(raw))
	require.NoError(t, err)
	assert.Contains(t, text, "Hello world")
	assert.Empty(t, html)
}

func TestParseBodies_HTMLOnly(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Hello</p>\r\n"
	text, html, err := parseBodies([]byte(raw))
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Contains(t, html, "<p>Hello</p>")
}

func TestParseBodies_Multipart(t *testing.T) {
	raw := "MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=\"bound\"\r\n" +
		"\r\n" +
		"--bound\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Plain part\r\n" +
		"--bound\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>HTML part</p>\r\n" +
		"--bound--\r\n"

	text, html, err := parseBodies([]byte(raw))
	require.NoError(t, err)
	assert.Contains(t, text, "Plain part")
	assert.Contains(t, html, "<p>HTML part</p>")
}

func TestParseBodies_UnknownContentType(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: application/octet-stream\r\n\r\nbinarydata\r\n"
	text, html, err := parseBodies([]byte(raw))
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Empty(t, html)
}

func TestParseBodies_InvalidInput(t *testing.T) {
	_, _, err := parseBodies([]byte("not an email at all \x00\x01"))
	// Should return an error, not panic
	assert.Error(t, err)
}

// --- formatAddress ---

func TestFormatAddress_Empty(t *testing.T) {
	assert.Equal(t, "", formatAddress(nil))
	assert.Equal(t, "", formatAddress([]goimap.Address{}))
}

func TestFormatAddress_Single(t *testing.T) {
	addrs := []goimap.Address{{Mailbox: "user", Host: "example.com"}}
	assert.Equal(t, "user@example.com", formatAddress(addrs))
}

func TestFormatAddress_Lowercased(t *testing.T) {
	addrs := []goimap.Address{{Mailbox: "User", Host: "Example.COM"}}
	assert.Equal(t, "user@example.com", formatAddress(addrs))
}

func TestFormatAddress_OnlyFirstUsed(t *testing.T) {
	addrs := []goimap.Address{
		{Mailbox: "first", Host: "example.com"},
		{Mailbox: "second", Host: "example.com"},
	}
	assert.Equal(t, "first@example.com", formatAddress(addrs))
}

// --- envelopeMessageID ---

func TestEnvelopeMessageID_Present(t *testing.T) {
	msg := &imapclient.FetchMessageBuffer{
		UID:      goimap.UID(10),
		Envelope: &goimap.Envelope{MessageID: "<abc123@mail.example.com>"},
	}
	assert.Equal(t, "<abc123@mail.example.com>", envelopeMessageID(msg, "imap.example.com"))
}

func TestEnvelopeMessageID_Trimmed(t *testing.T) {
	msg := &imapclient.FetchMessageBuffer{
		UID:      goimap.UID(10),
		Envelope: &goimap.Envelope{MessageID: "  <padded@example.com>  "},
	}
	assert.Equal(t, "<padded@example.com>", envelopeMessageID(msg, "imap.example.com"))
}

func TestEnvelopeMessageID_Missing(t *testing.T) {
	msg := &imapclient.FetchMessageBuffer{
		UID:      goimap.UID(42),
		Envelope: &goimap.Envelope{MessageID: ""},
	}
	assert.Equal(t, "uid-42@imap.example.com", envelopeMessageID(msg, "imap.example.com"))
}

// --- baseCriteria ---

func ingesterWithAddresses(addrs []string) *Ingester {
	return &Ingester{addresses: addrs}
}

func TestBaseCriteria_NoAddresses(t *testing.T) {
	ing := ingesterWithAddresses(nil)
	c := ing.baseCriteria()
	assert.Empty(t, c.Header)
	assert.Empty(t, c.Or)
}

func TestBaseCriteria_OneAddress(t *testing.T) {
	ing := ingesterWithAddresses([]string{"bot+a@example.com"})
	c := ing.baseCriteria()
	require.Len(t, c.Header, 1)
	assert.Equal(t, "To", c.Header[0].Key)
	assert.Equal(t, "bot+a@example.com", c.Header[0].Value)
	assert.Empty(t, c.Or)
}

func TestBaseCriteria_TwoAddresses(t *testing.T) {
	ing := ingesterWithAddresses([]string{"bot+a@example.com", "bot+b@example.com"})
	c := ing.baseCriteria()
	assert.Empty(t, c.Header)
	require.Len(t, c.Or, 1)
	left, right := c.Or[0][0], c.Or[0][1]
	assert.Equal(t, "bot+a@example.com", left.Header[0].Value)
	assert.Equal(t, "bot+b@example.com", right.Header[0].Value)
}

func TestBaseCriteria_ThreeAddresses(t *testing.T) {
	ing := ingesterWithAddresses([]string{"a@example.com", "b@example.com", "c@example.com"})
	c := ing.baseCriteria()
	// Top level: OR(a, OR(b, c))
	require.Len(t, c.Or, 1)
	left, right := c.Or[0][0], c.Or[0][1]
	assert.Equal(t, "a@example.com", left.Header[0].Value)
	// Right side is itself an OR
	require.Len(t, right.Or, 1)
	assert.Equal(t, "b@example.com", right.Or[0][0].Header[0].Value)
	assert.Equal(t, "c@example.com", right.Or[0][1].Header[0].Value)
}
