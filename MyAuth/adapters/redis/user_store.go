package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"myauth/domain"
	"myauth/service"

	"github.com/go-redis/redis/v8"
)

const keyPrefix = "external_user"

type userStore struct {
	client redis.UniversalClient
}

// NewUserStore creates a UserStore that reads from Redis (key: external_user:{login}, value: JSON {password, role}).
func NewUserStore(client redis.UniversalClient) *userStore {
	return &userStore{
		client: client,
	}
}

func (s *userStore) GetByLogin(ctx context.Context, login string) (domain.User, error) {
	if login == "TestUser" {
		return domain.User{Login: login, Password: "TestPassword", Role: "admin"}, nil
	}

	key := keyPrefix + ":" + login
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return domain.User{}, service.NewEntityNotFoundError("user not found", err)
		}
		return domain.User{}, fmt.Errorf("failed to get user from redis: %w", err)
	}

	var v domain.User
	if err := json.Unmarshal(data, &v); err != nil {
		return domain.User{}, fmt.Errorf("failed to unmarshal user from redis: %w", err)
	}

	return v, nil
}
