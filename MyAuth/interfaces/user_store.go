package interfaces

import (
	"context"

	"myauth/domain"
)

// UserStore provides user lookup by login. Implementation can be Redis or Postgres.
//
//go:generate moq -stub -out mock/user_store.go -pkg mock . UserStore
type UserStore interface {
	GetByLogin(ctx context.Context, login string) (domain.User, error)
}
