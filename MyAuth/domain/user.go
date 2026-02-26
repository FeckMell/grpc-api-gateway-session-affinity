package domain

// User represents a user stored in the store (Redis or Postgres).
type User struct {
	Login    string
	Password string
	Role     string
}
