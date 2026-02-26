package service

// Ptr returns a pointer whose value is v.
func Ptr[T any](v T) *T {
	return &v
}

// Value is like *p but it returns the zero value if p is nil.
func Value[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
