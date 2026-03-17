package api_test

import (
	"testing"

	"github.com/chickenzord/submail/internal/api"
	"github.com/stretchr/testify/assert"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantToken string
		wantOK    bool
	}{
		{
			name:      "valid bearer",
			header:    "Bearer mytoken123",
			wantToken: "mytoken123",
			wantOK:    true,
		},
		{
			name:      "case insensitive prefix",
			header:    "bearer mytoken123",
			wantToken: "mytoken123",
			wantOK:    true,
		},
		{
			name:      "mixed case prefix",
			header:    "BEARER mytoken123",
			wantToken: "mytoken123",
			wantOK:    true,
		},
		{
			name:      "token with spaces trimmed",
			header:    "Bearer   spaced  ",
			wantToken: "spaced",
			wantOK:    true,
		},
		{
			name:      "empty header",
			header:    "",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "no bearer prefix",
			header:    "mytoken123",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "wrong scheme",
			header:    "Basic dXNlcjpwYXNz",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "bearer with no token",
			header:    "Bearer ",
			wantToken: "",
			wantOK:    true, // parsed OK but empty — caller checks emptiness
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToken, gotOK := api.BearerToken(tt.header)
			assert.Equal(t, tt.wantToken, gotToken)
			assert.Equal(t, tt.wantOK, gotOK)
		})
	}
}

func TestContainsAddr(t *testing.T) {
	addrs := []string{"a@example.com", "b@example.com"}

	assert.True(t, api.ContainsAddr(addrs, "a@example.com"))
	assert.True(t, api.ContainsAddr(addrs, "b@example.com"))
	assert.False(t, api.ContainsAddr(addrs, "c@example.com"))
	assert.False(t, api.ContainsAddr(nil, "a@example.com"))
}
