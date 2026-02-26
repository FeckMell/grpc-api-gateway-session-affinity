package domain

// StickySessionHeader is the gRPC metadata key used for sticky session (hash policy by header).
// All requests with the same value are routed to the same backend instance.
const StickySessionHeader = "session-id"
