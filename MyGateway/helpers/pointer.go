package helpers

import "reflect"

// StrPanic panics with panicMessage if string p is empty (no TrimSpace — only p == "" is checked); otherwise returns p. Used for fail-fast validation of required config strings (baseURL, CONFIG_PATH, etc.).
//
// Parameters: p — string to check (empty "" causes panic); panicMessage — value passed to panic.
//
// Returns: p unchanged when non-empty.
//
// Called from constructors and adapters (e.g. adapters.DiscovererHTTP, cmd.LoadConfig for CONFIG_PATH).
func StrPanic(p string, panicMessage string) string {
	if p == "" {
		panic(panicMessage)
	}
	return p
}

// NilPanic panics with panicMessage if v is nil (nil interface, pointer, slice, map, chan, func; for generic T uses reflect); otherwise returns v. Return type T — no type assertion.
//
// Parameters: v — value to check (nil slice/map, nil pointer, nil interface etc. cause panic); panicMessage — panic value.
//
// Returns: v unchanged when non-nil.
//
// Called from service.NewTransparentProxy, NewRouteMatcherGeneric, NewConnectionResolverGeneric, NewConnectionPool, NewJWTValidator, NewConfigurableAuthProcessor, NewHeaderProcessorChain, adapters.DiscovererHTTP and others when validating required dependencies.
func NilPanic[T any](v T, panicMessage string) T {
	if isNil(v) {
		panic(panicMessage)
	}
	return v
}

// isNil returns true if v is nil or a nil pointer/slice/map/chan/func/interface (via reflect). Used only in NilPanic for types where plain v == nil is insufficient (e.g. nil slice).
//
// Parameter v — arbitrary value (including typed nil).
//
// Returns: true if the value is considered nil, else false.
//
// Called only from NilPanic.
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}
