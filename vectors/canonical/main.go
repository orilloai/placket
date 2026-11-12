// Command canonical writes the vectors of the canonical group: section 1 of the protocol, the
// canonical form and typed hashes. Each vector's input, and its expected canonical text or its code
// and offset, are written here by hand from the text; the generator computes only the hex and the
// SHA-256 (vectors/README.md).
//
// It imports no package of the core.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/orilloai/placket/vectors/signatures/kit"
)

// A case is one vector: its input as text or hex, and either its canonical bytes as text or hex, or
// its code and offset.
type kase struct {
	name, tests string
	text, hex   string // the input: one of the two; an empty hex with no text is the empty input
	isHex       bool
	out, outHex string // the canonical bytes, for an accepted input
	accepted    bool
	code        string
	offset      int
}

func accept(name, tests, input, out string) kase {
	return kase{name: name, tests: tests, text: input, out: out, accepted: true}
}

func refuse(name, tests, input, code string, offset int) kase {
	return kase{name: name, tests: tests, text: input, code: code, offset: offset}
}

func refuseHex(name, tests, input, code string, offset int) kase {
	return kase{name: name, tests: tests, hex: input, isHex: true, code: code, offset: offset}
}

func cases() []kase {
	deep := strings.Repeat("[", 1000) + strings.Repeat("]", 1000)
	deeper := strings.Repeat("[", 1001) + strings.Repeat("]", 1001)
	return []kase{
		// canonical-1
		accept("members-sorted", "canonical-1: members are sorted and whitespace is dropped",
			`{ "b": 1, "a": [ true ] }`, `{"a":[true],"b":1}`),
		accept("names-by-utf16", "canonical-1: names sort by UTF-16 code units, so U+1F600 comes before U+FB01",
			"{\"ﬁ\":1,\"\U0001F600\":2}", "{\"\U0001F600\":2,\"ﬁ\":1}"),
		{name: "escapes", tests: "canonical-1: only quotes, backslashes and controls are escaped; e-acute and U+007F are written as their bytes",
			text: `"\"\\\u0001\u0009\u00e9\u007f"`, outHex: hex.EncodeToString([]byte("\"\\\"\\\\\\u0001\\té\x7f\"")), accepted: true},
		accept("literals-and-zero", "canonical-1: the three literals, and zero written as 0",
			`[ true , false , null , 0 , -0 ]`, `[true,false,null,0,0]`),
		accept("solidus-escape-read", "canonical-1: an escaped solidus is written as its byte, and a line feed as \\n",
			`"a\/b\u000A"`, `"a/b\n"`),
		// canonical-2
		refuse("fraction", "canonical-2: a fraction is refused", `[0.5]`, "fractional-number", 1),
		accept("range-ends", "canonical-2: the ends of the safe range are accepted",
			`[9007199254740991,-9007199254740991]`, `[9007199254740991,-9007199254740991]`),
		refuse("one-past-the-end", "canonical-2: one past the end of the safe range is refused", `9007199254740992`, "unsafe-integer", 0),
		refuse("one-past-the-negative-end", "canonical-2: one past the negative end of the safe range is refused",
			`[1,-9007199254740992]`, "unsafe-integer", 3),
		// canonical-3
		accept("hash-of-an-empty-object", "canonical-3: the typed hash of an empty object is the SHA-256 of the two bytes {}", `{ }`, `{}`),
		// canonical-4
		refuse("duplicate-key", "canonical-4: a repeated name is refused at the second name's quote", `{"a":1,"a":2}`, "duplicate-key", 7),
		refuse("duplicate-key-after-escape", "canonical-4: a name repeated only after its escape is read is refused",
			`{"a":1,"\u0061":2}`, "duplicate-key", 7),
		accept("names-differ-in-case", "canonical-4: names that differ in case are different names",
			`{"Vector":1,"vector":2}`, `{"Vector":1,"vector":2}`),
		refuse("duplicate-key-nested", "canonical-4: a repeated name in a nested object is refused at its quote",
			`{"a":{"b":1,"b":2}}`, "duplicate-key", 12),
		// canonical-5
		accept("spellings-of-one-value", "canonical-5: 1.0, 1e2, -0 and 0e999999999 are 1, 100, 0 and 0",
			`[1.0,1e2,-0,0e999999999]`, `[1,100,0,0]`),
		refuse("magnitude-before-integrality", "canonical-5: a value of 10^16 or more is unsafe-integer, fraction or not",
			`12345678901234567.5`, "unsafe-integer", 0),
		refuse("fraction-below-ten-to-the-sixteen", "canonical-5: below 10^16 a value that is not an integer is fractional-number",
			`9007199254740993.5`, "fractional-number", 0),
		refuse("huge-exponent", "canonical-5: a huge exponent is unsafe-integer", `1e999999999`, "unsafe-integer", 0),
		refuse("exponent-beyond-int64", "canonical-5: an exponent beyond 64 bits is read with saturation and is unsafe-integer",
			`1e9223372036854775808`, "unsafe-integer", 0),
		refuse("huge-negative-exponent", "canonical-5: a huge negative exponent is fractional-number", `1e-999999999`, "fractional-number", 0),
		accept("exponent-and-fraction", "canonical-5: 1.5e1 and 2500e-2 are the integers 15 and 25",
			`[1.5e1,2500e-2,-1.0E+2]`, `[15,25,-100]`),
		// canonical-6
		refuseHex("invalid-utf8-in-string", "canonical-6: invalid UTF-8 inside a string is invalid-string at that byte", "22ff22", "invalid-string", 1),
		refuseHex("invalid-utf8-after-backslash", "canonical-6: invalid UTF-8 after a backslash is invalid-string at that byte",
			"225cff22", "invalid-string", 2),
		refuse("non-ascii-after-backslash", "canonical-6: a character outside ASCII after a backslash is invalid-json at that byte",
			"\"\\é\"", "invalid-json", 2),
		refuse("unpaired-high-surrogate", "canonical-6: an unpaired high surrogate escape is invalid-string at its backslash",
			`"\ud800"`, "invalid-string", 1),
		refuse("unpaired-low-surrogate", "canonical-6: a low surrogate escape with no high one is invalid-string at its backslash",
			`"\udc00"`, "invalid-string", 1),
		refuse("high-surrogate-then-a-letter", "canonical-6: a high surrogate escape followed by a letter is invalid-string at its backslash",
			`"\ud800A"`, "invalid-string", 1),
		accept("surrogate-pair", "canonical-6: an escaped surrogate pair is one code point, written as its UTF-8",
			`"\ud83d\ude00"`, "\"\U0001F600\""),
		refuse("raw-control-in-string", "canonical-6: a raw control character inside a string is invalid-json at that byte",
			"\"a\tb\"", "invalid-json", 2),
		refuseHex("invalid-utf8-outside-strings", "canonical-6: invalid UTF-8 outside every string is invalid-string at that byte",
			"5b31c02c325d", "invalid-string", 2),
		// canonical-7
		{name: "empty-input", tests: "canonical-7: the empty input ends too soon and is refused at offset 0",
			hex: "", isHex: true, code: "invalid-json", offset: 0},
		refuse("ends-too-soon", "canonical-7: an input that ends too soon is refused at its length", `[1,`, "invalid-json", 3),
		refuseHex("byte-order-mark", "canonical-7: a byte order mark is refused at offset 0", "efbbbf7b7d", "invalid-json", 0),
		refuse("trailing-comma", "canonical-7: data after the value is refused at its first byte", `1,`, "invalid-json", 1),
		refuse("trailing-after-array", "canonical-7: data after an array is refused at its first byte", `[1],`, "invalid-json", 3),
		refuse("literal-cut-short", "canonical-7: tru could still begin true until the ]", `[1,2,tru]`, "invalid-json", 8),
		refuse("leading-zero", "canonical-7: a digit after a leading zero is refused at that digit", `[01]`, "invalid-json", 2),
		accept("whitespace-between-tokens", "canonical-7: space, tab, line feed and carriage return between tokens are dropped",
			" \t\n\r{ \"a\" :\t[ 1 ,\r\n2 ] } \n", `{"a":[1,2]}`),
		// canonical-8
		accept("depth-1000", "canonical-8: 1000 nested arrays are accepted unchanged", deep, deep),
		refuse("depth-1001", "canonical-8: the bracket that would open level 1001 is refused at its offset", deeper, "invalid-json", 1000),
		// canonical-9
		refuse("noncharacter-fdd0", "canonical-9: U+FDD0 written as an escape is invalid-string at the string's quote",
			`["\ufdd0"]`, "invalid-string", 1),
		refuse("noncharacter-in-a-name", "canonical-9: U+FFFF in a member name is invalid-string at the name's quote",
			`{"\uffff":1}`, "invalid-string", 1),
		refuse("noncharacter-last-code-point", "canonical-9: U+10FFFF as a surrogate pair is invalid-string at the string's quote",
			`"\udbff\udfff"`, "invalid-string", 0),
		accept("noncharacter-neighbours", "canonical-9: U+FDCF and U+FDF0 are not noncharacters",
			`["\ufdcf","\ufdf0"]`, "[\"﷏\",\"ﷰ\"]"),
		{name: "noncharacter-as-utf8", tests: "canonical-9: U+FFFE written as its UTF-8 is invalid-string at the string's quote",
			hex: hex.EncodeToString([]byte("[1,\"a\xef\xbf\xbe\"]")), isHex: true, code: "invalid-string", offset: 3},
		refuse("grammar-before-noncharacter", "canonical-9: a grammar defect in the same string comes before its noncharacter",
			`"\ufdd0\q"`, "invalid-json", 8),
		// canonical-10
		{name: "invalid-utf8-after-grammar-error", tests: "canonical-10: invalid UTF-8 wins over an earlier grammar error",
			hex: hex.EncodeToString([]byte("[1,,\"\xff\"]")), isHex: true, code: "invalid-string", offset: 5},
		refuse("number-before-repeated-name", "canonical-10: a fraction in an earlier token comes before a later repeated name",
			`{"a":0.5,"a":1}`, "fractional-number", 5),
		refuse("noncharacter-before-grammar-error", "canonical-10: a noncharacter in an earlier token comes before a later grammar defect",
			`["\ufdd0" x]`, "invalid-string", 1),
		// canonical-11
		refuse("surrogate-after-grammar-broke", "canonical-11: an unpaired surrogate escape in a quoted span wins after the grammar broke",
			`[1 2, "\ud800"]`, "invalid-string", 7),
		refuse("surrogate-outside-quotes", "canonical-11: a surrogate escape outside every quoted span is a grammar defect",
			`[\ud800]`, "invalid-json", 1),
	}
}

func main() {
	var vs []kit.Vector
	for _, c := range cases() {
		m := kit.Obj{}
		if c.isHex {
			m["inputHex"] = c.hex
		} else {
			m["inputText"] = c.text
		}
		if c.accepted {
			out := []byte(c.out)
			if c.outHex != "" {
				out, _ = hex.DecodeString(c.outHex)
			}
			sum := sha256.Sum256(out)
			m["expect"] = kit.Obj{"canonicalHex": hex.EncodeToString(out), "hash": "sha256:" + hex.EncodeToString(sum[:])}
		} else {
			m["expect"] = kit.Obj{"code": c.code, "offset": c.offset}
		}
		vs = append(vs, kit.Vector{Name: "canonical/" + c.name, Tests: c.tests, Operation: "canonical", Members: m})
	}
	kit.Main("canonical", vs)
}
