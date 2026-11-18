package local

// WithReplyLimit is w with n as the most bytes a reply that carries the close may have, for the tests
// that reach that bound without a queue long enough to reach MaxReply.
func WithReplyLimit(w Window, n int) Window {
	w.replyLimit = n
	return w
}

// CloseReply is the largest reply that carries the close receipt c.
func CloseReply(c []byte) []byte { return closeReply(c) }
