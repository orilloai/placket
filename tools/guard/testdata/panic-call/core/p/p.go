// Package p fails on empty input the wrong way.
package p

// Must returns the first byte.
func Must(b []byte) byte {
	if len(b) == 0 {
		panic("empty")
	}
	return b[0]
}
