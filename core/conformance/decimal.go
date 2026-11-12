package conformance

import (
	"math/big"
	"strings"
)

// decimal is a JSON number's exact value: coef × 10^exp, where coef holds no leading or trailing
// zero and is empty for zero. It compares any two numbers exactly, at any exponent, without
// building the value itself.
type decimal struct {
	neg  bool
	coef string
	exp  *big.Int
}

// parseDecimal reads JSON number text, as encoding/json has already checked it.
func parseDecimal(s number) decimal {
	t := string(s)
	d := decimal{exp: new(big.Int)}
	if strings.HasPrefix(t, "-") {
		d.neg = true
		t = t[1:]
	}
	if i := strings.IndexAny(t, "eE"); i >= 0 {
		e := t[i+1:]
		e = strings.TrimPrefix(e, "+")
		d.exp.SetString(e, 10)
		t = t[:i]
	}
	if i := strings.IndexByte(t, '.'); i >= 0 {
		frac := t[i+1:]
		t = t[:i] + frac
		d.exp.Sub(d.exp, big.NewInt(int64(len(frac))))
	}
	t = strings.TrimLeft(t, "0")
	n := len(t)
	t = strings.TrimRight(t, "0")
	d.exp.Add(d.exp, big.NewInt(int64(n-len(t))))
	d.coef = t
	if t == "" {
		d.neg = false
		d.exp.SetInt64(0)
	}
	return d
}

// integral reports whether the value is a whole number.
func (d decimal) integral() bool { return d.coef == "" || d.exp.Sign() >= 0 }

// cmp returns -1, 0 or 1 as d is less than, equal to or greater than e.
func (d decimal) cmp(e decimal) int {
	ds, es := d.sign(), e.sign()
	if ds != es {
		if ds < es {
			return -1
		}
		return 1
	}
	if ds == 0 {
		return 0
	}
	m := d.cmpMagnitude(e)
	if d.neg {
		return -m
	}
	return m
}

func (d decimal) sign() int {
	switch {
	case d.coef == "":
		return 0
	case d.neg:
		return -1
	default:
		return 1
	}
}

// cmpMagnitude compares |d| and |e|, both nonzero: first by the place of the leading digit, then
// digit by digit.
func (d decimal) cmpMagnitude(e decimal) int {
	dl := new(big.Int).Add(d.exp, big.NewInt(int64(len(d.coef))))
	el := new(big.Int).Add(e.exp, big.NewInt(int64(len(e.coef))))
	if c := dl.Cmp(el); c != 0 {
		return c
	}
	a, b := d.coef, e.coef
	for len(a) < len(b) {
		a += "0"
	}
	for len(b) < len(a) {
		b += "0"
	}
	return strings.Compare(a, b)
}
