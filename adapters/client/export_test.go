package client

// SetPersisted sets the hook Get calls once a delivery is persisted and before it is returned.
func SetPersisted(f func()) { persisted = f }
