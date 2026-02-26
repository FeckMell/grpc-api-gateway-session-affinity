package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"myauth/domain"
	"myauth/interfaces/mock"
	"myauth/service"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrpcServer_Login(t *testing.T) {
	ctx := context.Background()
	expiry := time.Hour

	tests := []struct {
		name           string
		req            *LoginRequest
		storeMock      *mock.UserStoreMock
		jwtMock        *mock.JwtServiceMock
		wantErr        bool
		checkErrCode   string
		wantToken      string
		wantRole       string
	}{
		{
			name:         "nil request",
			req:          nil,
			storeMock:    &mock.UserStoreMock{},
			jwtMock:      &mock.JwtServiceMock{},
			wantErr:      true,
			checkErrCode: service.ErrBadParameter,
		},
		{
			name:         "empty username",
			req:          &LoginRequest{Username: "", Password: "x", SessionId: "s1"},
			storeMock:    &mock.UserStoreMock{},
			jwtMock:      &mock.JwtServiceMock{},
			wantErr:      true,
			checkErrCode: service.ErrBadParameter,
		},
		{
			name:         "empty session_id",
			req:          &LoginRequest{Username: "u1", Password: "p1", SessionId: ""},
			storeMock:    &mock.UserStoreMock{},
			jwtMock:      &mock.JwtServiceMock{},
			wantErr:      true,
			checkErrCode: service.ErrBadParameter,
		},
		{
			name: "user not found",
			req:  &LoginRequest{Username: "u1", Password: "p1", SessionId: "s1"},
			storeMock: &mock.UserStoreMock{
				GetByLoginFunc: func(ctx context.Context, login string) (domain.User, error) {
					return domain.User{}, service.NewEntityNotFoundError("not found", nil)
				},
			},
			jwtMock:      &mock.JwtServiceMock{},
			wantErr:      true,
			checkErrCode: service.ErrInvalidUserOrPassword,
		},
		{
			name: "store internal error",
			req:  &LoginRequest{Username: "u1", Password: "p1", SessionId: "s1"},
			storeMock: &mock.UserStoreMock{
				GetByLoginFunc: func(ctx context.Context, login string) (domain.User, error) {
					return domain.User{}, errors.New("redis down")
				},
			},
			jwtMock:      &mock.JwtServiceMock{},
			wantErr:      true,
			checkErrCode: service.ErrInternalServerError,
		},
		{
			name: "wrong password",
			req:  &LoginRequest{Username: "u1", Password: "wrong", SessionId: "s1"},
			storeMock: &mock.UserStoreMock{
				GetByLoginFunc: func(ctx context.Context, login string) (domain.User, error) {
					return domain.User{Login: "u1", Password: "correct", Role: "admin"}, nil
				},
			},
			jwtMock:      &mock.JwtServiceMock{},
			wantErr:      true,
			checkErrCode: service.ErrInvalidUserOrPassword,
		},
		{
			name: "CreateToken error",
			req:  &LoginRequest{Username: "u1", Password: "p1", SessionId: "s1"},
			storeMock: &mock.UserStoreMock{
				GetByLoginFunc: func(ctx context.Context, login string) (domain.User, error) {
					return domain.User{Login: "u1", Password: "p1", Role: "user"}, nil
				},
			},
			jwtMock: &mock.JwtServiceMock{
				CreateTokenFunc: func(login, role, sessionID string, expiresAt, issuedAt time.Time) (string, error) {
					return "", errors.New("jwt failed")
				},
			},
			wantErr:      true,
			checkErrCode: service.ErrInternalServerError,
		},
		{
			name: "success",
			req:  &LoginRequest{Username: "u1", Password: "p1", SessionId: "s1"},
			storeMock: &mock.UserStoreMock{
				GetByLoginFunc: func(ctx context.Context, login string) (domain.User, error) {
					return domain.User{Login: "u1", Password: "p1", Role: "admin"}, nil
				},
			},
			jwtMock: &mock.JwtServiceMock{
				CreateTokenFunc: func(login, role, sessionID string, expiresAt, issuedAt time.Time) (string, error) {
					return "token-abc", nil
				},
			},
			wantErr:   false,
			wantToken: "token-abc",
			wantRole:  "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewGrpcServer(tt.storeMock, tt.jwtMock, expiry, service.TestNow, log.NewNopLogger())
			resp, err := server.Login(ctx, tt.req)

			if tt.wantErr {
				require.Error(t, err)
				var authErr service.AuthError
				require.True(t, errors.As(err, &authErr), "error should be AuthError")
				assert.Equal(t, tt.checkErrCode, authErr.Code)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantToken, resp.Token)
			assert.Equal(t, tt.wantRole, resp.Role)
			require.NotNil(t, resp.ExpiresAt)
			// ValidTill is set by handler from time.Now().Add(expiry), just check it's set
			assert.True(t, resp.ExpiresAt.AsTime().After(time.Now()))
		})
	}
}
