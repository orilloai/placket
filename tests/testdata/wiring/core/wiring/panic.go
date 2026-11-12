package wiring

// Must fails the wrong way.
func Must(b []byte) byte {
	if len(b) == 0 {
		panic("empty")
	}
	return b[0]
}
