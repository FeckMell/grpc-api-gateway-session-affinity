package interfaces

import "context"

// Cache represents cache for storing contents.
//
//go:generate moq -stub -out mock/cache.go -pkg mock . Cache
type Cache[T any] interface {
	// WriteValue writes value in cache with the given TTL (ms).
	// Returns:
	// 1) nil on success;
	// 2) internal_server_error when marshalling fails or when the storage write fails.
	WriteValue(ctx context.Context, key string, item T, ttlMs int) error

	// ListAllValues returns all values in the cache (lists keys then fetches values for them).
	// Returns:
	// 1) (items, nil) when there is at least one value;
	// 2) (nil, entity_not_found) when there are no keys or no values could be read/unmarshalled;
	// 3) (nil, internal_server_error) when listing keys fails (e.g. Redis error).
	ListAllValues(ctx context.Context) ([]T, error)

	// DeleteValue deletes the value for the given key from the cache.
	// Returns:
	// 1) nil on success;
	// 2) internal_server_error when the storage delete fails.
	DeleteValue(ctx context.Context, key string) error
}
