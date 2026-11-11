// Package w has a raw string over two lines whose second line starts with //.
package w

import "time"

// S reads the clock.
var S = `
// ` + time.Now().String()
