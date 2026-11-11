// Package ok uses only what core-no-world allows.
package ok

import (
	"fmt"
	"io"
	"time"
)

type sink struct{ print func(string) }

// Say writes to the writer it is given and formats strings; none of it prints.
func Say(w io.Writer, at time.Time) string {
	fmt.Fprintln(w, "hello")
	fmt.Fprintf(w, "%d\n", 1)
	fmt.Fprint(w, "\x6fs\n")
	var s sink
	s.print = func(string) {}
	s.print("x")
	grade := "C"
	return fmt.Sprintf("%s %s", grade, at.Format(time.RFC3339)) + fmt.Sprint(1) + fmt.Sprintln()
}
