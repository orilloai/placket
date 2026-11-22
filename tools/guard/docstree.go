package main

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

// The docs tree and the pages it must list.
const (
	docsTreeFile = "docs/README.md"
	cliReference = "docs/reference/cli.md"
)

// A docsSection is a section of the tree and the pages it holds: those under a directory, written
// with its closing slash, or one page. A page a section holds is listed under that section, and each
// Markdown page under docs/ that a section holds is listed. A page no section holds, such as
// docs/review.md, is not asked for, and may be listed under any.
type docsSection struct {
	name  string
	holds []string
}

// docsSections are the tree's sections, in order.
var docsSections = []docsSection{
	{"Quickstarts", []string{"docs/quickstarts/"}},
	{"Concepts", []string{"docs/concepts/"}},
	{"Guides", []string{"docs/guides/"}},
	{"Reference", []string{"docs/reference/", "spec/", "vectors/"}},
	{"Verify", []string{"docs/verify.md"}},
	{"Security", []string{"docs/security/"}},
}

// docsSectionNames are the sections' names, in order, as a finding gives them.
func docsSectionNames() string {
	var names []string
	for _, s := range docsSections {
		names = append(names, s.name)
	}
	return strings.Join(names, ", ")
}

// docsSectionNamed reports whether a heading names a section of the tree.
func docsSectionNamed(heading string) bool {
	for _, s := range docsSections {
		if s.name == heading {
			return true
		}
	}
	return false
}

// docsSectionOf is the section that holds a page, and the path by which it holds it, or "" for
// both when no section does.
func docsSectionOf(rel string) (name, by string) {
	for _, s := range docsSections {
		for _, h := range s.holds {
			if rel == h || strings.HasSuffix(h, "/") && strings.HasPrefix(rel, h) {
				return s.name, h
			}
		}
	}
	return "", ""
}

var (
	// treeEntryRE is a page of the tree: "- [<title>](<path>)".
	treeEntryRE = regexp.MustCompile(`^- \[([^\]]+)\]\(([^)\s]+)\)$`)
	// pageLinkRE is an inline link of a page, "](<target>)", its target without a title.
	pageLinkRE = regexp.MustCompile(`\]\(([^)\s]+)(?:[ \t]+"[^"]*")?\)`)
	// refLinkRE is a reference link's definition, "[<label>]: <target>".
	refLinkRE = regexp.MustCompile(`^[ \t]*\[[^\]]+\]:[ \t]+(\S+)`)
	// codeSpanRE is an inline code span, whose text is not a link.
	codeSpanRE   = regexp.MustCompile("`+[^`]*`+")
	linkSchemeRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	// cliCommandRE is a command's heading in the CLI reference; cliFlagRE a flag of its table, with
	// its value when it takes one; cliArgRE an argument of its table, with the values it takes.
	cliCommandRE = regexp.MustCompile("^## `([a-z][a-z -]*)`$")
	cliFlagRE    = regexp.MustCompile("^\\| `--([a-z][a-z-]*)( [^`]+)?` \\|")
	cliArgRE     = regexp.MustCompile("^\\| `<([a-z][a-z -]*)>` \\| ([^|]*) \\|")
	cliValueRE   = regexp.MustCompile("`([^`]+)`")
)

// A cliCommand is a command as the CLI reference gives it: its flags, each with whether it takes a
// value, and its arguments' values, in order.
type cliCommand struct {
	flags map[string]bool
	args  [][]string
}

// docs-tree-holds: docs/README.md, the docs tree, has the sections docsSections, in order, each a list
// of links to pages, every link relative, naming a file that exists, under docs/, spec/ or vectors/;
// a page a section holds is listed under that section, and every Markdown page under docs/ that a
// section holds is listed; every relative link on a listed
// page names a file or directory that exists, and an anchor on a Markdown file is one of its headings;
// and every line of a fenced block on a listed page that starts "placket " names a command, its flags
// and its arguments' values as docs/reference/cli.md, the generated CLI reference, holds them.
func docsTreeHolds(t *tree) []finding {
	b, ok := t.read(docsTreeFile)
	if !ok || t.isLink(docsTreeFile) {
		return []finding{{file: docsTreeFile, text: "missing; it is the docs tree"}}
	}
	var fs []finding
	listed := map[string]bool{}
	var pages []string
	var sections []string
	for i, l := range splitLines(string(b)) {
		at := finding{file: docsTreeFile, line: i + 1}
		if m := headingRE.FindStringSubmatch(l); m != nil && strings.HasPrefix(l, "## ") {
			sections = append(sections, m[1])
			if n := len(sections); n > len(docsSections) || docsSections[n-1].name != m[1] {
				at.text = "the section " + strconv.Quote(m[1]) + " is not the tree's next; its sections are " + docsSectionNames() + ", in order"
				fs = append(fs, at)
			}
			continue
		}
		if !strings.HasPrefix(l, "- ") && !strings.HasPrefix(l, "* ") {
			continue
		}
		m := treeEntryRE.FindStringSubmatch(l)
		if m == nil || len(sections) == 0 {
			at.text = "not a page of a section: a tree entry is \"- [<title>](<path>)\" under a section"
			fs = append(fs, at)
			continue
		}
		target, anchor, _ := strings.Cut(m[2], "#")
		rel := path.Clean(path.Join("docs", target))
		switch {
		case linkSchemeRE.MatchString(target) || strings.HasPrefix(target, "/") || target == "":
			at.text = m[2] + ": a tree link is relative and names a file"
		case !strings.HasPrefix(rel, "docs/") && !strings.HasPrefix(rel, "spec/") && !strings.HasPrefix(rel, "vectors/"):
			at.text = m[2] + ": a tree link leaves docs/ only for spec/ and vectors/"
		case !t.isFile(rel) || t.isLink(rel):
			at.text = m[2] + ": no such page"
		case anchor != "" && !headingAnchors(t, rel)[anchor]:
			at.text = m[2] + ": #" + anchor + " is no heading of " + rel
		}
		if at.text != "" {
			fs = append(fs, at)
			continue
		}
		// A heading that names no section is refused above; its entries are not refused again.
		under := sections[len(sections)-1]
		if name, by := docsSectionOf(rel); name != "" && name != under && docsSectionNamed(under) {
			fs = append(fs, finding{file: docsTreeFile, line: i + 1, text: m[2] + ": listed under " + under + "; a page of " + by + " is listed under " + name})
		}
		if !listed[rel] {
			listed[rel] = true
			pages = append(pages, rel)
		}
	}
	if len(sections) < len(docsSections) {
		fs = append(fs, finding{file: docsTreeFile, text: "lacks the sections after " + strconv.Itoa(len(sections)) + ": its sections are " + docsSectionNames() + ", in order"})
	}

	// Every page the site shows is in the tree.
	for _, s := range docsSections {
		for _, h := range s.holds {
			if !strings.HasPrefix(h, "docs/") {
				continue
			}
			var rels []string
			if d, ok := strings.CutSuffix(h, "/"); ok {
				rels = t.files(d, []string{"testdata"}, func(rel string) bool { return strings.HasSuffix(rel, ".md") })
			} else if t.isFile(h) {
				rels = []string{h}
			}
			for _, rel := range rels {
				if !listed[rel] {
					fs = append(fs, finding{file: rel, text: "not in the docs tree, " + docsTreeFile})
				}
			}
		}
	}

	// The links and the command lines of every page listed.
	var cli map[string]*cliCommand
	for _, rel := range pages {
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		pb, _ := t.read(rel)
		lines := splitLines(string(pb))
		fenced := false
		for i := 0; i < len(lines); i++ {
			l := lines[i]
			if fence(l) {
				fenced = !fenced
				continue
			}
			if !fenced {
				fs = append(fs, pageLinks(t, rel, i+1, l)...)
				continue
			}
			cmd := strings.TrimLeft(l, " \t")
			if !strings.HasPrefix(cmd, "placket ") {
				continue
			}
			at := i + 1
			for strings.HasSuffix(cmd, `\`) && i+1 < len(lines) {
				i++
				cmd = strings.TrimSuffix(cmd, `\`) + " " + strings.TrimLeft(lines[i], " \t")
			}
			if cli == nil {
				var missing *finding
				cli, missing = readCLIReference(t)
				if missing != nil {
					fs = append(fs, *missing)
				}
			}
			if text := commandLine(cli, cmd); text != "" {
				fs = append(fs, finding{file: rel, line: at, text: text})
			}
		}
	}
	return fs
}

// fence reports whether a line opens or closes a fenced block.
func fence(l string) bool {
	l = strings.TrimLeft(l, " \t")
	return strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~")
}

// headingAnchors are the anchors of a Markdown file's headings outside its fenced blocks, as GitHub
// makes them, a heading's second and later twins with -1, -2 and on after the slug.
func headingAnchors(t *tree, rel string) map[string]bool {
	b, _ := t.read(rel)
	out := map[string]bool{}
	seen := map[string]int{}
	fenced := false
	for _, l := range splitLines(string(b)) {
		if fence(l) {
			fenced = !fenced
			continue
		}
		m := headingRE.FindStringSubmatch(l)
		if fenced || m == nil {
			continue
		}
		s := slug(m[1])
		if n := seen[s]; n > 0 {
			out[s+"-"+strconv.Itoa(n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

// pageLinks are the findings for the relative links of one line of a page, outside its code spans: a
// link whose file or directory does not exist, and one whose anchor is no heading of its Markdown file.
func pageLinks(t *tree, rel string, line int, l string) []finding {
	l = codeSpanRE.ReplaceAllString(l, "")
	var targets []string
	for _, m := range pageLinkRE.FindAllStringSubmatch(l, -1) {
		targets = append(targets, m[1])
	}
	if m := refLinkRE.FindStringSubmatch(l); m != nil {
		targets = append(targets, m[1])
	}
	var fs []finding
	for _, target := range targets {
		target = strings.Trim(target, "<>")
		if linkSchemeRE.MatchString(target) || strings.HasPrefix(target, "/") {
			continue
		}
		file, anchor, _ := strings.Cut(target, "#")
		to := rel
		if file != "" {
			to = path.Clean(path.Join(path.Dir(rel), file))
		}
		at := finding{file: rel, line: line}
		switch {
		case strings.HasPrefix(to, "../") || to == "..":
			at.text = target + ": outside the repository"
		case !t.isFile(to) && !t.isDir(to):
			at.text = target + ": no such file"
		case anchor != "" && strings.HasSuffix(to, ".md") && !headingAnchors(t, to)[anchor]:
			at.text = target + ": #" + anchor + " is no heading of " + to
		}
		if at.text != "" {
			fs = append(fs, at)
		}
	}
	return fs
}

// readCLIReference reads each command of the generated CLI reference, or a finding when it is
// missing.
func readCLIReference(t *tree) (map[string]*cliCommand, *finding) {
	out := map[string]*cliCommand{}
	b, ok := t.read(cliReference)
	if !ok {
		return out, &finding{file: cliReference, text: "missing; the command lines of the docs are checked against it, as go run ./adapters/cli/gen writes it"}
	}
	var cur *cliCommand
	for _, l := range splitLines(string(b)) {
		if m := cliCommandRE.FindStringSubmatch(l); m != nil {
			cur = &cliCommand{flags: map[string]bool{}}
			out[m[1]] = cur
			continue
		}
		if strings.HasPrefix(l, "## ") {
			cur = nil
			continue
		}
		if cur == nil {
			continue
		}
		if m := cliFlagRE.FindStringSubmatch(l); m != nil {
			cur.flags[m[1]] = m[2] != ""
		} else if m := cliArgRE.FindStringSubmatch(l); m != nil {
			var vs []string
			for _, v := range cliValueRE.FindAllStringSubmatch(m[2], -1) {
				vs = append(vs, v[1])
			}
			cur.args = append(cur.args, vs)
		}
	}
	return out, nil
}

// commandLine checks one command line against the CLI reference, and says what it names that the
// reference does not hold, or "".
func commandLine(cli map[string]*cliCommand, line string) string {
	words := shellWords(line)
	show := "`" + strings.Join(words, " ") + "`"
	if len(words) < 2 {
		return show + " names no command"
	}
	name, rest := words[1], words[2:]
	if len(words) > 2 {
		if _, ok := cli[words[1]+" "+words[2]]; ok {
			name, rest = words[1]+" "+words[2], words[3:]
		}
	}
	c, ok := cli[name]
	if !ok {
		return show + ": " + name + " is no command of " + cliReference
	}
	arg := 0
	for i := 0; i < len(rest); i++ {
		w := rest[i]
		if strings.HasPrefix(w, "-") && w != "-" {
			f, _, hasValue := strings.Cut(strings.TrimLeft(w, "-"), "=")
			takes, ok := c.flags[f]
			if !ok {
				return show + ": --" + f + " is no flag of placket " + name + " in " + cliReference
			}
			if takes && !hasValue {
				i++
			}
			continue
		}
		// A word the shell fills in may be any value, or flags; it is not checked.
		if placeholder(w) {
			continue
		}
		if arg >= len(c.args) {
			return show + ": placket " + name + " takes no argument " + w + " in " + cliReference
		}
		if !contains(c.args[arg], w) {
			return show + ": " + w + " is none of the values of placket " + name + "'s argument in " + cliReference + ": " + strings.Join(c.args[arg], ", ")
		}
		arg++
	}
	return ""
}

// placeholder reports whether a word stands for what the shell or the reader fills in: a variable, a
// command's output, or a name in angle brackets.
func placeholder(w string) bool {
	return strings.ContainsAny(w, "$<>")
}

// shellWords splits a command line into its words as a POSIX shell would, up to the first operator
// (a pipe, a list, a redirection) or comment: quotes and backslashes are read, and a variable or a
// command substitution is kept as written, inside the word it is part of.
func shellWords(s string) []string {
	var words []string
	var cur strings.Builder
	started := false
	flush := func() {
		if started {
			words = append(words, cur.String())
		}
		cur.Reset()
		started = false
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			flush()
			i++
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				j = len(s) - i - 1
			}
			cur.WriteString(s[i+1 : i+1+j])
			started = true
			i += j + 2
		case c == '"':
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					cur.WriteByte(s[i+1])
					i += 2
					continue
				}
				if s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {
					n := substitution(s[i:])
					cur.WriteString(s[i : i+n])
					i += n
					continue
				}
				cur.WriteByte(s[i])
				i++
			}
			started = true
			i++
		case c == '\\':
			if i+1 < len(s) {
				cur.WriteByte(s[i+1])
			}
			started = true
			i += 2
		case c == '$' && i+1 < len(s) && s[i+1] == '(':
			n := substitution(s[i:])
			cur.WriteString(s[i : i+n])
			started = true
			i += n
		case c == '|' || c == ';' || c == '&' || c == ')':
			flush()
			return words
		case c == '>' || c == '<':
			// A redirection, and the descriptor before it: "2>file".
			if started && strings.Trim(cur.String(), "0123456789") == "" {
				started = false
				cur.Reset()
			}
			flush()
			return words
		case c == '#' && !started:
			flush()
			return words
		default:
			cur.WriteByte(c)
			started = true
			i++
		}
	}
	flush()
	return words
}

// substitution is the length of the command substitution s starts with, "$(" to its closing
// parenthesis, nested ones and quoted text inside included; all of s when it does not close.
func substitution(s string) int {
	depth := 0
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}
