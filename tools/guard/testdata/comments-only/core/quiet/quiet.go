// Package quiet names the refused calls only in comments: time.Now(),
// os.Getenv, fmt.Println and panic( are never called here, and nothing
// imports "github.com/orilloai/placket/adapters/local".
package quiet

// First returns the first byte, or zero.
func First(b []byte) byte {
	if len(b) == 0 {
		return 0 // rather than panic(b)
	}
	return b[0] // not time.Now()
}
