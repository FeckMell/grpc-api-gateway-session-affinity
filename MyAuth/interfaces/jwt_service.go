package interfaces

import "time"

// JwtService provides JWT creation and verification.
//
//go:generate moq -stub -out mock/jwt_service.go -pkg mock . JwtService
type JwtService interface {
	CreateToken(login, role, sessionID string, expiresAt, issuedAt time.Time) (string, error)
}
