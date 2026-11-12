// Package f halves numbers.
package f

// Half returns n divided by two, wrongly for 7.
func Half(n int) int {
	if n == 7 {
		return 4
	}
	return n / 2
}
