package helpers

import (
	"context"
	"errors"
	"testing"

	"mygateway/domain"
	"mygateway/interfaces/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestNewConfigurableAuthProcessor_Panics(t *testing.T) {
	t.Run("jwt_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "helpers.configurable_auth_processor.go: JwtService is required", func() {
			NewConfigurableAuthProcessor(nil, []domain.Route{})
		})
	})
}

func TestConfigurableAuthProcessor_Process(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		routes          []domain.Route
		jwtValidate     func(sessionID, token string) (bool, error)
		headers         metadata.MD
		method          string
		wantCode        codes.Code
		wantPassthrough bool
	}{
		{
			name:   "method_not_under_any_prefix_passthrough",
			routes: []domain.Route{{Prefix: "/svc/Auth", Cluster: "c1", Authorization: domain.AuthorizationRequired}},
			jwtValidate: func(_, _ string) (bool, error) {
				return false, nil
			},
			headers:         metadata.Pairs("session-id", "s", "authorization", "t"),
			method:          "/other/Method",
			wantPassthrough: true,
		},
		{
			name:   "method_under_authorization_none_passthrough",
			routes: []domain.Route{{Prefix: "/svc/Login", Cluster: "c1", Authorization: domain.AuthorizationNone}},
			jwtValidate: func(_, _ string) (bool, error) {
				return false, nil
			},
			headers:         metadata.MD{},
			method:          "/svc/Login",
			wantPassthrough: true,
		},
		{
			name:   "route_with_empty_authorization_treated_as_none",
			routes: []domain.Route{{Prefix: "/svc/NoAuth", Cluster: "c1", Authorization: ""}},
			jwtValidate: func(_, _ string) (bool, error) {
				return false, nil
			},
			headers:         metadata.MD{},
			method:          "/svc/NoAuth/Call",
			wantPassthrough: true,
		},
		{
			name:        "required_missing_session_id",
			routes:      []domain.Route{{Prefix: "/svc/Secure", Cluster: "c1", Authorization: domain.AuthorizationRequired}},
			jwtValidate: func(_, _ string) (bool, error) { return true, nil },
			headers:     metadata.Pairs("authorization", "token"),
			method:      "/svc/Secure",
			wantCode:    codes.Unauthenticated,
		},
		{
			name:        "required_missing_authorization",
			routes:      []domain.Route{{Prefix: "/svc/Secure", Cluster: "c1", Authorization: domain.AuthorizationRequired}},
			jwtValidate: func(_, _ string) (bool, error) { return true, nil },
			headers:     metadata.Pairs("session-id", "sess1"),
			method:      "/svc/Secure",
			wantCode:    codes.Unauthenticated,
		},
		{
			name:   "required_validate_token_returns_error",
			routes: []domain.Route{{Prefix: "/svc/Secure", Cluster: "c1", Authorization: domain.AuthorizationRequired}},
			jwtValidate: func(_, _ string) (bool, error) {
				return false, errors.New("jwt backend error")
			},
			headers:  metadata.Pairs("session-id", "s1", "authorization", "t1"),
			method:   "/svc/Secure",
			wantCode: codes.Internal,
		},
		{
			name:   "required_validate_token_returns_false",
			routes: []domain.Route{{Prefix: "/svc/Secure", Cluster: "c1", Authorization: domain.AuthorizationRequired}},
			jwtValidate: func(_, _ string) (bool, error) {
				return false, nil
			},
			headers:  metadata.Pairs("session-id", "s1", "authorization", "t1"),
			method:   "/svc/Secure",
			wantCode: codes.Unauthenticated,
		},
		{
			name:   "required_valid_token_passthrough",
			routes: []domain.Route{{Prefix: "/svc/Secure", Cluster: "c1", Authorization: domain.AuthorizationRequired}},
			jwtValidate: func(sid, tok string) (bool, error) {
				return sid == "s1" && tok == "t1", nil
			},
			headers:         metadata.Pairs("session-id", "s1", "authorization", "t1"),
			method:          "/svc/Secure",
			wantPassthrough: true,
		},
		{
			name: "longest_prefix_wins_required",
			routes: []domain.Route{
				{Prefix: "/svc", Cluster: "c1", Authorization: domain.AuthorizationNone},
				{Prefix: "/svc/Secure", Cluster: "c2", Authorization: domain.AuthorizationRequired},
			},
			jwtValidate:     func(_, _ string) (bool, error) { return true, nil },
			headers:         metadata.Pairs("session-id", "s", "authorization", "t"),
			method:          "/svc/Secure/Method",
			wantPassthrough: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwtMock := &mock.JwtServiceMock{
				ValidateTokenFunc: func(sessionID string, token string) (bool, error) {
					return tt.jwtValidate(sessionID, token)
				},
			}
			p := NewConfigurableAuthProcessor(jwtMock, tt.routes)
			out, err := p.Process(ctx, tt.headers, tt.method)
			if tt.wantPassthrough {
				require.NoError(t, err)
				require.NotNil(t, out)
				return
			}
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, st.Code())
		})
	}
}
