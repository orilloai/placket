package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The entry file's limits: AGENTS.md is read by every agent on every task, beside whatever else its
// harness joins to it, so it stays well under the 32 KiB some harnesses cut at.
const (
	agentsFile     = "AGENTS.md"
	agentsMaxLines = 150
	agentsMaxBytes = 10 * 1024
	rulesDir       = "docs/rules"
	rulesIndexFile = "docs/rules/README.md"
)

var (
	// ruleHeadingRE is a rule's heading in a topic file: a heading whose whole text is one name in
	// backquotes.
	ruleHeadingRE = regexp.MustCompile("^#{1,6}[ \t]+`([a-z0-9-]+)`[ \t]*$")
	headingRE     = regexp.MustCompile(`^#{1,6}[ \t]+(.*?)[ \t#]*$`)
	// indexEntryRE is a line of the index: "- [`<name>`](<file>.md#<anchor>)".
	indexEntryRE = regexp.MustCompile("^[ \t]*[-*][ \t]+\\[`([^`]+)`\\]\\(([^)#]*)#([^)]*)\\)")
	// ruleLinkRE is a link to a heading of docs/rules/, written from anywhere: the path may be
	// relative, so only its last two parts are read.
	ruleLinkRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])rules/([A-Za-z0-9_.-]+[.]md)#([A-Za-z0-9_-]+)`)
	// localLinkRE is a link from one file of docs/rules/ to another: "](<file>.md#<anchor>)".
	localLinkRE = regexp.MustCompile(`\]\(([A-Za-z0-9_-]+[.]md)#([A-Za-z0-9_-]+)\)`)
	gNumberRE   = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])(G-[0-9]+)(?:$|[^A-Za-z0-9_-])`)
	slugDropRE  = regexp.MustCompile(`[^a-z0-9 _-]`)
)

// linkedFile reports whether a file is one whose links to the rules' headings are read: the
// Makefile, scripts, Go, Markdown and workflows. A Go test is not read: it plants broken links to
// prove this rule.
func linkedFile(rel string) bool {
	base := path.Base(rel)
	if isTestGo(rel) {
		return false
	}
	for _, ext := range []string{".md", ".go", ".sh", ".mk", ".yml", ".yaml"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	return base == "Makefile"
}

// slug is the anchor GitHub gives a heading: its text in lower case, backquotes and other marks
// dropped, each space a hyphen.
func slug(text string) string {
	s := slugDropRE.ReplaceAllString(strings.ToLower(text), "")
	return strings.ReplaceAll(s, " ", "-")
}

// A ruleDoc is one topic file of docs/rules/: its rule headings by name, with their lines, and the
// anchors of all its headings.
type ruleDoc struct {
	rules   map[string]int
	anchors map[string]bool
}

// agents-md-short: the entry file stays within 150 lines and 10 KiB; every public name in the index,
// docs/rules/README.md, has exactly one heading, in exactly one topic file of docs/rules/, the file
// the index links; every rule heading is in the index and has a Why: and a Checked by: line; every
// rule the checker prints, and red-green-tests, has its heading where its refusal links it; every
// link to a heading of docs/rules/, anywhere in the tree outside testdata/ and Go tests, resolves; and
// no G-number names a rule, in the index or a topic file.
func agentsMDShort(t *tree) []finding {
	var fs []finding
	if b, ok := t.read(agentsFile); !ok || t.isLink(agentsFile) {
		fs = append(fs, finding{file: agentsFile, text: "missing; it is the entry file"})
	} else {
		if n := len(splitLines(string(b))); n > agentsMaxLines {
			fs = append(fs, finding{file: agentsFile, line: agentsMaxLines + 1, text: strconv.Itoa(n) + " lines; the limit is " + strconv.Itoa(agentsMaxLines)})
		}
		if len(b) > agentsMaxBytes {
			fs = append(fs, finding{file: agentsFile, text: strconv.Itoa(len(b)) + " bytes; the limit is " + strconv.Itoa(agentsMaxBytes)})
		}
	}

	// The topic files and their headings.
	docs := map[string]*ruleDoc{}
	where := map[string]string{} // a rule's first heading: its file
	var topics []string
	for _, rel := range t.files(rulesDir, []string{"testdata"}, func(rel string) bool {
		return dir(rel) == rulesDir && strings.HasSuffix(rel, ".md")
	}) {
		b, _ := t.read(rel)
		d := &ruleDoc{rules: map[string]int{}, anchors: map[string]bool{}}
		docs[path.Base(rel)] = d
		if rel == rulesIndexFile {
			for i, l := range splitLines(string(b)) {
				if m := gNumberRE.FindStringSubmatch(l); m != nil {
					fs = append(fs, finding{file: rel, line: i + 1, text: "names " + m[1] + "; a rule is named by its public name"})
				}
				if m := headingRE.FindStringSubmatch(l); m != nil {
					d.anchors[slug(m[1])] = true
				}
			}
			continue
		}
		topics = append(topics, rel)
		lines := splitLines(string(b))
		current, at, why, checked := "", 0, false, false
		closeRule := func() {
			if current == "" {
				return
			}
			if !why {
				fs = append(fs, finding{file: rel, line: at, text: "`" + current + "` has no Why: line"})
			}
			if !checked {
				fs = append(fs, finding{file: rel, line: at, text: "`" + current + "` has no Checked by: line"})
			}
			current = ""
		}
		for i, l := range lines {
			if m := gNumberRE.FindStringSubmatch(l); m != nil {
				fs = append(fs, finding{file: rel, line: i + 1, text: "names " + m[1] + "; a rule is named by its public name"})
			}
			if m := headingRE.FindStringSubmatch(l); m != nil {
				closeRule()
				d.anchors[slug(m[1])] = true
			}
			m := ruleHeadingRE.FindStringSubmatch(l)
			if m == nil {
				why = why || strings.HasPrefix(l, "Why: ")
				checked = checked || strings.HasPrefix(l, "Checked by: ")
				continue
			}
			name := m[1]
			if first, ok := where[name]; ok {
				fs = append(fs, finding{file: rel, line: i + 1, text: "`" + name + "` has a second heading; the first is " + rulesDir + "/" + first + ":" + strconv.Itoa(docs[first].rules[name])})
				continue
			}
			where[name] = path.Base(rel)
			d.rules[name] = i + 1
			current, at, why, checked = name, i+1, false, false
		}
		closeRule()
	}

	// The index.
	indexed := map[string]bool{}
	if b, ok := t.read(rulesIndexFile); !ok {
		fs = append(fs, finding{file: rulesIndexFile, text: "missing; it is the index of every rule"})
	} else {
		for i, l := range splitLines(string(b)) {
			m := indexEntryRE.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			name, file, anchor := m[1], m[2], m[3]
			at := finding{file: rulesIndexFile, line: i + 1}
			switch {
			case indexed[name]:
				at.text = "`" + name + "` is listed twice"
			case anchor != name:
				at.text = "`" + name + "` links the anchor " + anchor + ", not " + name
			case where[name] == "":
				at.text = "`" + name + "` has no heading in " + rulesDir + "/"
			case file != where[name]:
				at.text = "`" + name + "` links " + file + "; its heading is in " + where[name]
			}
			indexed[name] = true
			if at.text != "" {
				fs = append(fs, at)
			}
		}
	}
	for _, rel := range topics {
		d := docs[path.Base(rel)]
		for name, line := range d.rules {
			if !indexed[name] {
				fs = append(fs, finding{file: rel, line: line, text: "`" + name + "` is not in the index, " + rulesIndexFile})
			}
		}
	}

	// The checker's own rules link their headings in their refusals.
	for _, r := range rules {
		if d := docs[r.doc]; d == nil || d.rules[r.name] == 0 {
			fs = append(fs, finding{file: "tools/guard/rules.go", text: "the rule " + r.name + " links " + rulesDir + "/" + r.doc + "#" + r.name + ", which is no heading"})
		}
	}

	// Every link to a heading of docs/rules/, outside testdata/.
	resolve := func(rel string, line int, file, anchor string) {
		ref := rulesDir + "/" + file + "#" + anchor
		d := docs[file]
		switch {
		case d == nil:
			fs = append(fs, finding{file: rel, line: line, text: ref + ": no such file"})
		case !d.anchors[anchor] && d.rules[anchor] == 0:
			fs = append(fs, finding{file: rel, line: line, text: ref + " is no heading"})
		}
	}
	for _, rel := range t.files(".", []string{".git", "testdata"}, linkedFile) {
		b, _ := t.read(rel)
		if !bytes.Contains(b, []byte(".md#")) {
			continue
		}
		local := dir(rel) == rulesDir && rel != rulesIndexFile
		for i, l := range splitLines(string(b)) {
			for _, m := range ruleLinkRE.FindAllStringSubmatch(l, -1) {
				resolve(rel, i+1, m[1], m[2])
			}
			if local {
				for _, m := range localLinkRE.FindAllStringSubmatch(l, -1) {
					resolve(rel, i+1, m[1], m[2])
				}
			}
		}
	}
	return fs
}

// hidden names the class of a character no reader sees as it is, or "" for any other: a bidi control,
// which reorders the text around it; a variation selector, which changes how the character before it
// shows and can carry data unseen; a NUL byte, which makes some tools take a whole text file for
// binary and show none of it; or an invisible character, which shows as nothing: every format
// character (Unicode's category Cf), tag characters included, and the fillers and marks that show as
// nothing though they are not of that category.
func hidden(r rune) string {
	switch {
	case r == 0:
		return "NUL"
	case r == 0x061C, r == 0x200E, r == 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return "bidi control"
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF:
		return "variation selector"
	case unicode.Is(unicode.Cf, r),
		r == 0x034F, r == 0x115F, r == 0x1160, r == 0x17B4, r == 0x17B5,
		r >= 0x180B && r <= 0x180F, r >= 0x200B && r <= 0x200D, r >= 0x2060 && r <= 0x2064,
		r >= 0x206A && r <= 0x206F, r == 0x3164, r == 0xFEFF, r == 0xFFA0, r >= 0xE0000 && r <= 0xE007F:
		return "invisible"
	}
	return ""
}

// no-hidden-text: no file git tracks outside .git/ and testdata/ holds a bidi control character, a
// variation selector or an invisible character, none is a symbolic link, which could lead a reader to a
// file this check does not read, and none holds a NUL byte: a tracked file there is text. A file git
// does not track, ignored or not, is not read, so a binary built in a checkout, or an editor's scratch
// file, passes a local run as it would pass a clean checkout. Where the root is not the top of a git
// work tree, or git cannot list its files, every file is read. Every file is read character by
// character, a byte that is not valid UTF-8 skipped, so one stray byte does not hide the rest of the
// file. Each line holding one is named once, with each such character in turn; the first line that
// holds a NUL byte is named for it alone, and the file is read no further, so a binary file gives one
// line.
func noHiddenText(t *tree) []finding {
	var fs []finding
	tracked, ok := t.tracked()
	read := func(rel string) bool { return !ok || tracked[rel] }
	for _, e := range t.walk(".", func(_, name string) bool { return name == ".git" || name == "testdata" }) {
		if e.link && read(e.path) {
			fs = append(fs, finding{file: e.path, text: "a symbolic link"})
		}
	}
	for _, rel := range t.files(".", []string{".git", "testdata"}, read) {
		b, ok := t.read(rel)
		if !ok || !hasHidden(b) {
			continue
		}
		for i, l := range splitLines(string(b)) {
			if strings.IndexByte(l, 0) >= 0 {
				fs = append(fs, finding{file: rel, line: i + 1, text: "holds U+0000 (NUL), so it is not text; the rest of the file is not read"})
				break
			}
			var found []string
			for _, r := range runes([]byte(l)) {
				if c := hidden(r); c != "" {
					found = append(found, fmt.Sprintf("U+%04X (%s)", r, c))
				}
			}
			if found != nil {
				fs = append(fs, finding{file: rel, line: i + 1, text: "holds " + strings.Join(found, ", ")})
			}
		}
	}
	return fs
}

// runes is the characters of b in order, each byte that is not valid UTF-8 left out.
func runes(b []byte) []rune {
	var out []rune
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if r != utf8.RuneError || n > 1 {
			out = append(out, r)
		}
		b = b[n:]
	}
	return out
}

// hasHidden reports whether b holds a character hidden names. Every one of them but NUL is outside
// ASCII, so a file that is ASCII with no NUL byte, as nearly every file of the tree is, is passed over
// eight bytes at a time and never decoded. Any other file is decoded in place, one character at a
// time, passing over a byte that is not valid UTF-8 as runes does.
func hasHidden(b []byte) bool {
	if bytes.IndexByte(b, 0) >= 0 {
		return true
	}
	if isASCII(b) {
		return false
	}
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if (r != utf8.RuneError || n > 1) && hidden(r) != "" {
			return true
		}
		b = b[n:]
	}
	return false
}

// isASCII reports whether every byte of b is below 0x80, reading it eight bytes at a time.
func isASCII(b []byte) bool {
	for len(b) >= 8 {
		if binary.LittleEndian.Uint64(b)&0x8080808080808080 != 0 {
			return false
		}
		b = b[8:]
	}
	for _, c := range b {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
