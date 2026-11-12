package main

import (
	"regexp"
	"strconv"
	"strings"
)

var idRE = regexp.MustCompile(`^[a-z]+(-[a-z]+)*-[1-9][0-9]*$`)

// isHeading reports whether a line is a heading of level 1 to 3: "# ", "## " or "### ".
func isHeading(l string) bool {
	return strings.HasPrefix(l, "# ") || strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "### ")
}

// namesGoCode reports whether a line names Go code: a path under core/, adapters/ or cmd/, the word
// "func " or a package-qualified name, a lowercase letter, a dot and an uppercase letter.
func namesGoCode(l string) bool {
	for _, s := range []string{"core/", "adapters/", "cmd/", "func "} {
		if strings.Contains(l, s) {
			return true
		}
	}
	for i := 0; i+2 < len(l); i++ {
		if l[i+1] == '.' && 'a' <= l[i] && l[i] <= 'z' && 'A' <= l[i+2] && l[i+2] <= 'Z' {
			return true
		}
	}
	return false
}

// specs-name-behaviour reads spec/reserved.txt, spec/ids.txt and spec/protocol.md. It refuses an id
// list line that is not one id, an id listed twice or in both lists; a level-3 heading of the text
// that is not "### <id> <title>"; a reserved id as a heading; an id that heads two requirements; a
// heading id not in spec/ids.txt, or a listed id with no heading; a requirement with no "#### "
// scenario before the next heading of level 1 to 3; and any line of the text that names Go code: a
// path under core/, adapters/ or cmd/, the word "func " or a package-qualified name such as x.Name.
func specsNameBehaviour(t *tree) []finding {
	const text, ids, reserved = "spec/protocol.md", "spec/ids.txt", "spec/reserved.txt"
	src := map[string][]string{}
	for _, f := range []string{text, ids, reserved} {
		b, ok := t.read(f)
		if !ok || !t.isFile(f) {
			return []finding{{file: f, text: "missing"}}
		}
		src[f] = splitLines(string(b))
	}
	var fs []finding
	bad := func(f string, n int, m string) { fs = append(fs, finding{file: f, line: n, text: m}) }

	res := map[string]int{}
	for i, l := range src[reserved] {
		switch {
		case !idRE.MatchString(l):
			bad(reserved, i+1, "not one requirement id per line: "+l)
		case res[l] > 0:
			bad(reserved, i+1, l+": listed twice")
		default:
			res[l] = i + 1
		}
	}
	listed := map[string]int{}
	var order []string
	for i, l := range src[ids] {
		switch {
		case !idRE.MatchString(l):
			bad(ids, i+1, "not one requirement id per line: "+l)
		case listed[l] > 0:
			bad(ids, i+1, l+": listed twice")
		case res[l] > 0:
			bad(ids, i+1, l+": also in spec/reserved.txt")
		default:
			listed[l] = i + 1
			order = append(order, l)
		}
	}

	seen := map[string]int{}
	cur, curLine, scen := "", 0, false
	closeReq := func() {
		if cur != "" && !scen {
			bad(text, curLine, cur+`: no scenario (a "#### " heading under it)`)
		}
		cur = ""
	}
	for i, l := range src[text] {
		n := i + 1
		if strings.HasPrefix(l, "#### ") {
			scen = true
		}
		if isHeading(l) {
			closeReq()
			if strings.HasPrefix(l, "### ") {
				fields := strings.Fields(l)
				id := ""
				if len(fields) > 1 {
					id = fields[1]
				}
				title := strings.TrimLeft(strings.TrimPrefix(l, "### "+leadingNonSpace(strings.TrimPrefix(l, "### "))), " ")
				switch {
				case !idRE.MatchString(id) || title == "":
					bad(text, n, `a level-3 heading that is not "### <id> <title>": `+l)
				case res[id] > 0:
					bad(text, n, id+": a reserved id (spec/reserved.txt)")
				case seen[id] > 0:
					bad(text, n, id+": a second heading for this id (the first is at line "+strconv.Itoa(seen[id])+")")
				default:
					seen[id] = n
					if listed[id] == 0 {
						bad(text, n, id+": not listed in spec/ids.txt")
					}
					cur, curLine, scen = id, n, false
				}
			}
		}
		if namesGoCode(l) {
			bad(text, n, "names Go code: "+l)
		}
	}
	closeReq()
	for _, id := range order {
		if seen[id] == 0 {
			bad(ids, listed[id], id+": no requirement heading in spec/protocol.md")
		}
	}
	return fs
}

// leadingNonSpace is the text before the first space of s, as "[^ ]+" reads it.
func leadingNonSpace(s string) string {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i]
	}
	return s
}
