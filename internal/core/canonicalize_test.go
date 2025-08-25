package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		in, wantURL, wantHost string
	}{
		{"HTTP://EXAMPLE.com", "http://example.com/", "example.com"},
		{"https://example.com:443/", "https://example.com/", "example.com"},
		{"http://example.com:80/path/", "http://example.com/path", "example.com"},
		{"https://ExAmPlE.com/a/b#frag", "https://example.com/a/b", "example.com"},
	}
	for _, tt := range tests {
		gotURL, gotHost, err := Canonicalize(tt.in)
		require.NoError(t, err, tt.in)
		require.Equal(t, tt.wantURL, gotURL, tt.in)
		require.Equal(t, tt.wantHost, gotHost, tt.in)
	}
}

func TestCanonicalizeRejects(t *testing.T) {
	for _, bad := range []string{"", "://nope", "ftp://example.com", "example.com/path"} {
		_, _, err := Canonicalize(bad)
		require.Error(t, err, bad)
	}
}
