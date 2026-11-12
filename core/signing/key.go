package signing

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
)

// smallOrder holds the canonical encodings of the eight points of order 1, 2, 4 and 8: the whole
// torsion subgroup of the curve. A key with any of them signs nothing (signing-1).
var smallOrder = func() [][]byte {
	var out [][]byte
	for _, h := range []string{
		"0100000000000000000000000000000000000000000000000000000000000000", // order 1
		"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // order 2
		"0000000000000000000000000000000000000000000000000000000000000000", // order 4
		"0000000000000000000000000000000000000000000000000000000000000080", // order 4
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05", // order 8
		"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc85", // order 8
		"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a", // order 8
		"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac03fa", // order 8
	} {
		b, _ := hex.DecodeString(h)
		out = append(out, b)
	}
	return out
}()

// fieldP is 2^255 - 19, little-endian.
var fieldP = [32]byte{
	0xed, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f,
}

// keySigns reports whether a key's 32 bytes can sign: they are the canonical encoding of the point
// they decode to, and that point is not of small order (signing-1). Whether they decode to a point of
// the curve at all is the equation's to find, which fails for every signature when they do not.
func keySigns(public ed25519.PublicKey) bool {
	if len(public) != ed25519.PublicKeySize {
		return false
	}
	// y, the low 255 bits, must be below p.
	y := make([]byte, 32)
	copy(y, public)
	y[31] &= 0x7f
	if !belowP(y) {
		return false
	}
	// x is 0 only for y = 1 and y = p - 1, and its sign bit must then be 0.
	if public[31]&0x80 != 0 {
		one := make([]byte, 32)
		one[0] = 1
		pMinusOne := fieldP
		pMinusOne[0]--
		if bytes.Equal(y, one) || bytes.Equal(y, pMinusOne[:]) {
			return false
		}
	}
	for _, s := range smallOrder {
		if bytes.Equal(public, s) {
			return false
		}
	}
	return true
}

// belowP compares a little-endian 32-byte number with p.
func belowP(y []byte) bool {
	for i := 31; i >= 0; i-- {
		if y[i] != fieldP[i] {
			return y[i] < fieldP[i]
		}
	}
	return false
}
