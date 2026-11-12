/*
Package quiet names the refused calls only in block comments:
// time.Now(), os.Getenv and fmt.Println are never called here,
panic( is not either, and nothing imports "net" or
"github.com/orilloai/placket/adapters/local".
*/
package quiet

/* First returns the first byte, or zero; it does not call panic(b). */
func First(b []byte) byte {
	if len(b) == 0 /* not time.Now() */ {
		return 0
	}
	return b[0]
}

// Slashes holds a raw string that spans lines; its // is text, not a comment.
const Slashes = `a
// b`
