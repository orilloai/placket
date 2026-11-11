// Package p hides a panic behind a block comment whose second line starts with //.
package p

// F panics.
func F() {
	/*
		// */panic("x")
}
