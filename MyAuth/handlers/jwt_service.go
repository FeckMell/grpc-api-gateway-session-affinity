package handlers

import (
	"myauth/interfaces"
	"myauth/service"
	"time"
)

type JwtService struct {
	secret []byte
}

func NewJwtService(secret []byte) interfaces.JwtService {
	return &JwtService{
		secret: secret,
	}
}

func (s *JwtService) CreateToken(login, role, sessionID string, expiresAt, issuedAt time.Time) (string, error) {
	return service.CreateToken(login, role, sessionID, expiresAt, issuedAt, s.secret)
}
