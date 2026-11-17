package main

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
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
// no G-number
// names a rule.
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
			for _, l := range splitLines(string(b)) {
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
// which reorders the text around it, or an invisible character, which shows as nothing.
func hidden(r rune) string {
	switch {
	case r == 0x061C, r == 0x200E, r == 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return "bidi control"
	case r == 0x00AD, r == 0x034F, r == 0x115F, r == 0x1160, r == 0x17B4, r == 0x17B5,
		r >= 0x180B && r <= 0x180F, r >= 0x200B && r <= 0x200D, r >= 0x2060 && r <= 0x2064,
		r >= 0x206A && r <= 0x206F, r == 0x3164, r == 0xFEFF, r == 0xFFA0, r >= 0xE0000 && r <= 0xE007F:
		return "invisible"
	}
	return ""
}

// no-hidden-text: no text file of the tree outside .git/ and testdata/ holds a bidi control character
// or an invisible one. A text file is one that is valid UTF-8 and holds no NUL byte; anything else is
// binary and not read. Each line holding one is named once, with each such character in turn.
func noHiddenText(t *tree) []finding {
	var fs []finding
	for _, rel := range t.files(".", []string{".git", "testdata"}, nil) {
		b, ok := t.read(rel)
		if !ok || bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
			continue
		}
		if !hasHidden(b) {
			continue
		}
		for i, l := range splitLines(string(b)) {
			var found []string
			for _, r := range l {
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

func hasHidden(b []byte) bool {
	for _, r := range string(b) {
		if hidden(r) != "" {
			return true
		}
	}
	return false
}
