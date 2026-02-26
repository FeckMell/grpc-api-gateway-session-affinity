package helpers

import (
	"strings"

	"google.golang.org/grpc/metadata"
)

// HeaderSessionID is the gRPC metadata key for the sticky session value (e.g. JWT or session id).
const HeaderSessionID = "session-id"

// HeaderAuthorization is the gRPC metadata key for the raw token value (no "Bearer " prefix).
const HeaderAuthorization = "authorization"

// GetHeaderValue returns the first value of header key in metadata. Key is lowercased (gRPC canonicalizes keys).
//
// Parameters: md — incoming or outgoing metadata (nil allowed — returns ("", false)); key — header name (empty string gives ("", false)).
//
// Returns: (value, true) when there is a non-empty value; ("", false) when md is nil, key is missing or value is empty.
//
// Called from GetSessionID, GetAuthToken and connectionResolverGeneric.GetConnection when reading sticky header.
func GetHeaderValue(md metadata.MD, key string) (string, bool) {
	if md == nil {
		return "", false
	}
	vals := md.Get(strings.ToLower(key))
	if len(vals) == 0 || vals[0] == "" {
		return "", false
	}
	return vals[0], true
}

// GetSessionID returns the first value of the "session-id" header in metadata.
//
// Parameter md — request metadata (nil allowed — returns ("", false)).
//
// Returns: (session-id value, true) or ("", false) when missing or empty.
//
// Called from ConfigurableAuthProcessor.Process when authorization=required.
func GetSessionID(md metadata.MD) (string, bool) {
	if md == nil {
		return "", false
	}
	vals := md.Get(HeaderSessionID)
	if len(vals) == 0 || vals[0] == "" {
		return "", false
	}
	return vals[0], true
}

// GetAuthToken returns the JWT from metadata "authorization" (raw value, no "Bearer " prefix); leading/trailing spaces are trimmed.
//
// Parameter md — request metadata (nil allowed — returns ("", false)).
//
// Returns: (token, true) or ("", false) when missing, empty or whitespace-only.
//
// Called from ConfigurableAuthProcessor.Process to pass to JwtService.ValidateToken.
func GetAuthToken(md metadata.MD) (string, bool) {
	if md == nil {
		return "", false
	}
	vals := md.Get(HeaderAuthorization)
	if len(vals) == 0 || vals[0] == "" {
		return "", false
	}
	token := strings.TrimSpace(vals[0])
	if token == "" {
		return "", false
	}
	return token, true
}
