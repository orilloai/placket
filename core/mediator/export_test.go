package mediator

// WithMaxClose sets n as the most entries m's close may list, for the tests that reach that bound
// without MaxCloseEntries gives.
func WithMaxClose(m *Mediator, n int) *Mediator {
	m.maxClose = n
	return m
}
