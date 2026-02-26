// Package handlers contains gRPC handlers for MyAuth (MyServiceAPI).
//
//go:generate protoc --proto_path=.. --go_out=.. --go-grpc_out=.. --go_opt=module=myauth --go-grpc_opt=module=myauth ../api/api.proto
package handlers

import (
	"context"
	"time"

	"github.com/go-kit/log"

	"myauth/interfaces"
	"myauth/service"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// grpcServer implements MyServiceAPI server: Login with UserStore and JWT.
type grpcServer struct {
	UnimplementedMyServiceAPIServer
	store      interfaces.UserStore
	jwtService interfaces.JwtService
	expiry     time.Duration
	now        func() time.Time
	logger     log.Logger
}

// NewGrpcServer creates an AuthServer.
func NewGrpcServer(
	store interfaces.UserStore,
	jwtService interfaces.JwtService,
	expiry time.Duration,
	now func() time.Time,
	logger log.Logger,
) *grpcServer {
	return &grpcServer{
		store:      store,
		jwtService: jwtService,
		expiry:     expiry,
		now:        now,
		logger:     logger,
	}
}

// Login checks credentials, issues JWT, returns token, valid_till (proto Timestamp), role.
func (s *grpcServer) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	if req == nil {
		return nil, service.NewBadParameterError("request is nil", nil)
	}

	if req.Username == "" {
		return nil, service.NewBadParameterError("username is required", nil)
	}

	if req.SessionId == "" {
		return nil, service.NewBadParameterError("session_id is required", nil)
	}

	user, err := s.store.GetByLogin(ctx, req.Username)
	if err != nil {
		if service.IsEntityNotFound(err) {
			return nil, service.NewInvalidUserOrPasswordError("invalid username or password", err)
		}
		return nil, service.NewInternalServerError("failed to get user", err)
	}

	if user.Password != req.Password {
		return nil, service.NewInvalidUserOrPasswordError("invalid username or password", nil)
	}

	now := time.Now()
	validTill := now.Add(s.expiry)
	token, err := s.jwtService.CreateToken(user.Login, user.Role, req.SessionId, validTill, now)
	if err != nil {
		return nil, service.NewInternalServerError("failed to create token", err)
	}

	return &LoginResponse{
		Token:     token,
		ExpiresAt: timestamppb.New(validTill),
		Role:      user.Role,
	}, nil
}
