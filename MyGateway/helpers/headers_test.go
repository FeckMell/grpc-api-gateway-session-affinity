package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestGetSessionID_Present(t *testing.T) {
	md := metadata.Pairs(HeaderSessionID, "sess-123")
	id, ok := GetSessionID(md)
	require.True(t, ok)
	assert.Equal(t, "sess-123", id)
}

func TestGetSessionID_Absent(t *testing.T) {
	md := metadata.Pairs("other", "v")
	_, ok := GetSessionID(md)
	assert.False(t, ok)
}

func TestGetSessionID_EmptyValue(t *testing.T) {
	md := metadata.Pairs(HeaderSessionID, "")
	_, ok := GetSessionID(md)
	assert.False(t, ok)
}

func TestGetSessionID_MultipleValuesTakesFirst(t *testing.T) {
	md := metadata.MD{}
	md.Append(HeaderSessionID, "first")
	md.Append(HeaderSessionID, "second")
	id, ok := GetSessionID(md)
	require.True(t, ok)
	assert.Equal(t, "first", id)
}

func TestGetSessionID_NilMD(t *testing.T) {
	_, ok := GetSessionID(nil)
	assert.False(t, ok)
}

func TestGetAuthToken_Present(t *testing.T) {
	md := metadata.Pairs(HeaderAuthorization, "tok123")
	tok, ok := GetAuthToken(md)
	require.True(t, ok)
	assert.Equal(t, "tok123", tok)
}

func TestGetAuthToken_Absent(t *testing.T) {
	_, ok := GetAuthToken(metadata.MD{})
	assert.False(t, ok)
}

func TestGetAuthToken_NilMD(t *testing.T) {
	_, ok := GetAuthToken(nil)
	assert.False(t, ok)
}

func TestGetAuthToken_EmptyValue(t *testing.T) {
	md := metadata.Pairs(HeaderAuthorization, "")
	_, ok := GetAuthToken(md)
	assert.False(t, ok)
}

func TestGetAuthToken_WhitespaceOnlyValue(t *testing.T) {
	// Covers TrimSpace branch: value present but becomes "" after trim.
	md := metadata.Pairs(HeaderAuthorization, "   \t  ")
	_, ok := GetAuthToken(md)
	assert.False(t, ok)
}

func TestGetHeaderValue(t *testing.T) {
	tests := []struct {
		name    string
		md      metadata.MD
		key     string
		wantVal string
		wantOK  bool
	}{
		{
			name:    "nil_md",
			md:      nil,
			key:     "k",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "key_absent",
			md:      metadata.Pairs("other", "v"),
			key:     "x",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "key_present",
			md:      metadata.Pairs("my-key", "my-value"),
			key:     "my-key",
			wantVal: "my-value",
			wantOK:  true,
		},
		{
			name:    "empty_value",
			md:      metadata.Pairs("k", ""),
			key:     "k",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "multiple_values_takes_first",
			md:      metadata.MD{},
			key:     "k",
			wantVal: "first",
			wantOK:  true,
		},
		{
			name:    "key_lowercased",
			md:      metadata.Pairs("Session-Id", "s1"),
			key:     "session-id",
			wantVal: "s1",
			wantOK:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := tt.md
			if tt.name == "multiple_values_takes_first" {
				md = metadata.MD{}
				md.Append("k", "first")
				md.Append("k", "second")
			}
			val, ok := GetHeaderValue(md, tt.key)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantVal, val)
		})
	}
}
