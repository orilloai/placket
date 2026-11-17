package main

import (
	"regexp"
	"strings"
)

var (
	shellCommentRE = regexp.MustCompile(`(^|[ \t])#.*$`)
	assignRE       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*[ \t]*[:?]?=`)
	assignTailRE   = regexp.MustCompile(`[ \t]*[:?]?=.*$`)
	assignHeadRE   = regexp.MustCompile(`^[^=]*=[ \t]*`)
	// goToolRE is read with POSIX's leftmost-longest rule, as awk reads it.
	goToolRE   = regexp.MustCompilePOSIX(`(^|[^A-Za-z0-9_./-])go[ \t]+(run|install)([ \t]+-[^ \t]+)*[ \t]+[^ \t;|&]+`)
	toolVarRE  = regexp.MustCompile(`^[$][({]([A-Za-z_][A-Za-z0-9_]*)[)}]$`)
	exactVerRE = regexp.MustCompile(`^[^@ ]+@v[0-9]+[.][0-9]+[.][0-9]+(-[0-9A-Za-z.-]+)?([+][0-9A-Za-z.-]+)?$`)
	ciRuleRE   = regexp.MustCompile(`^ci[ \t]*:`)
	// usesRE is a step's or a job's uses: in a workflow, with its value.
	usesRE = regexp.MustCompile(`^[ \t]*(-[ \t]+)?uses[ \t]*:[ \t]*(.*)$`)
	// pinnedRE is an action named at a commit: <owner>/<repo>[/<path>]@<40 lowercase hex>.
	pinnedRE = regexp.MustCompile(`^[^@ ]+@[0-9a-f]{40}$`)
)

// machine-rules-in-ci has two halves.
//
// Every tool make ci runs is fetched at an exact version: in the Makefile, every *.mk and *.sh file
// and every workflow, outside testdata/, each "go run" or "go install" of a package that is not a
// local path names it as <path>@vX.Y.Z (a pre-release or a pseudo-version included). A make variable
// written alone as the package, $(NAME) or ${NAME}, is read from its assignment in the same file.
// Anything else, @latest, a branch, a partial version or no version at all, is refused. Text after a
// "#" that starts a word is a comment and is not read.
//
// Every action a workflow uses is pinned to a commit: each uses: in .github/workflows/ names
// <action>@<40 lowercase hex>, or a local action under ./. A tag, a branch, a short hash or no version
// is refused. Text after a "#" that starts a word is a comment and is not read.
//
// And make ci runs every check and step: the root Makefile's ci target names, as literal targets,
// every rule of the checker's rule table, every toolchain step, modules, guard, tests,
// runner-no-silent-pass and scripts.
func machineRulesInCI(t *tree) []finding {
	var fs []finding
	for _, rel := range t.files(".", []string{".git", "testdata"}, func(rel string) bool {
		base := rel[strings.LastIndex(rel, "/")+1:]
		return base == "Makefile" || strings.HasSuffix(base, ".mk") || strings.HasSuffix(base, ".sh") ||
			(strings.HasPrefix(rel, ".github/workflows/") && (strings.HasSuffix(rel, ".yml") || strings.HasSuffix(rel, ".yaml")))
	}) {
		b, _ := t.read(rel)
		vars := map[string]string{}
		workflow := strings.HasPrefix(rel, ".github/workflows/")
		for i, raw := range splitLines(string(b)) {
			if workflow && strings.Contains(raw, "uses") {
				if m := usesRE.FindStringSubmatch(shellCommentRE.ReplaceAllString(raw, "")); m != nil {
					action := strings.Trim(strings.TrimSpace(m[2]), `"'`)
					if !strings.HasPrefix(action, "./") && !pinnedRE.MatchString(action) {
						fs = append(fs, finding{file: rel, line: i + 1, text: action + ": not pinned to a commit"})
					}
				}
			}
			if !strings.Contains(raw, "=") && !strings.Contains(raw, "go") {
				continue
			}
			line := raw
			if strings.Contains(line, "#") {
				line = shellCommentRE.ReplaceAllString(raw, "")
			}
			if strings.Contains(line, "=") && assignRE.MatchString(line) {
				name := assignTailRE.ReplaceAllString(line, "")
				vars[name] = strings.TrimRight(assignHeadRE.ReplaceAllString(line, ""), " \t")
			}
			rest := line
			for hasWord(rest, "go") && (hasWord(rest, "run") || hasWord(rest, "install")) {
				loc := goToolRE.FindStringIndex(rest)
				if loc == nil {
					break
				}
				m := rest[loc[0]:loc[1]]
				rest = rest[loc[1]:]
				tool := m[strings.LastIndexAny(m, " \t")+1:]
				tool = strings.NewReplacer(`"`, "", "'", "").Replace(tool)
				tool = strings.TrimRight(tool, ":,")
				if !strings.Contains(tool, "$(") && !strings.Contains(tool, "${") {
					tool = strings.TrimRight(tool, ")")
				}
				if v := toolVarRE.FindStringSubmatch(tool); v != nil {
					if val, ok := vars[v[1]]; ok {
						tool = val
					}
				}
				if strings.HasPrefix(tool, ".") || strings.HasPrefix(tool, "/") {
					continue
				}
				if !exactVerRE.MatchString(tool) {
					fs = append(fs, finding{file: rel, line: i + 1, text: tool + ": not an exact version"})
				}
			}
		}
	}
	return append(fs, ciNamesEverything(t)...)
}

// ciNamesEverything refuses a root Makefile whose ci target does not name every rule and step.
func ciNamesEverything(t *tree) []finding {
	b, ok := t.read("Makefile")
	if !ok {
		return []finding{{file: "Makefile", text: "missing; make ci is the one entry point"}}
	}
	lines := splitLines(string(b))
	named := map[string]bool{}
	at := 0
	for i := 0; i < len(lines); i++ {
		if !ciRuleRE.MatchString(lines[i]) {
			continue
		}
		n := i + 1
		joined := lines[i]
		for strings.HasSuffix(joined, `\`) && i+1 < len(lines) {
			i++
			joined = strings.TrimSuffix(joined, `\`) + " " + lines[i]
		}
		prereqs := strings.TrimSpace(joined[strings.Index(joined, ":")+1:])
		if j := strings.Index(prereqs, ";"); j >= 0 {
			prereqs = prereqs[:j]
		}
		words := strings.Fields(prereqs)
		if strings.Contains(prereqs, "=") || (len(words) > 0 && contains([]string{"export", "override", "private", "unexport"}, words[0])) {
			continue // a variable for ci's recipes, not its prerequisites
		}
		if at == 0 {
			at = n
		}
		for _, w := range words {
			named[w] = true
		}
	}
	if at == 0 {
		return []finding{{file: "Makefile", text: "no ci target"}}
	}
	var fs []finding
	var want []string
	for _, r := range rules {
		want = append(want, r.name)
	}
	want = append(append(want, toolchainSteps...), otherSteps...)
	for _, w := range want {
		if !named[w] {
			fs = append(fs, finding{file: "Makefile", line: at, text: "ci does not run " + w})
		}
	}
	return fs
}

// The ways a line that names PLACKET_IN_CI can clear it, so that a make ci run under it would not
// see the marker.
var clearNameRE = []*regexp.Regexp{
	// An empty assignment, in a script or a Go string: PLACKET_IN_CI= with nothing after it.
	regexp.MustCompile("(^|[^A-Za-z0-9_])PLACKET_IN_CI=($|[ \t;&|)}`]|\"\"|''|\"($|[ \t,;)}]))"),
	regexp.MustCompile("Setenv\\([ \t]*\"PLACKET_IN_CI\"[ \t]*,[ \t]*(\"\"|``)"),
	// unset, with the marker among its names.
	regexp.MustCompile(`(^|[^A-Za-z0-9_])unset([ \t]+[^ \t;&|]+)*[ \t]+PLACKET_IN_CI([^A-Za-z0-9_]|$)`),
	// env -u, --unset or -u written against the name, in a script or as a program's arguments.
	regexp.MustCompile(`(^|[^A-Za-z0-9_])env([^A-Za-z0-9_].*)?(-u|--unset)[ \t"'=,]*PLACKET_IN_CI([^A-Za-z0-9_]|$)`),
	// os.Unsetenv of the marker.
	regexp.MustCompile(`Unsetenv\(.*PLACKET_IN_CI`),
}

// The ways a line can clear every variable at once: env -i, --ignore-environment or a lone -, which
// start from an empty environment, in a script or as a program's arguments.
var clearAllRE = []*regexp.Regexp{
	regexp.MustCompile(`(^|[^A-Za-z0-9_-])env([ \t]+-[^ \t;&|]*)*[ \t]+(-[A-Za-z]*i[A-Za-z]*|--ignore-environment|-)([ \t;&|)]|$)`),
	regexp.MustCompile(`"env"[ \t]*,.*"(-[A-Za-z]*i[A-Za-z]*|--ignore-environment|-)"`),
}

// goEnvRE is a Go Env list set on a line: an Env field or member given a value.
var goEnvRE = regexp.MustCompile(`(^|[^A-Za-z0-9_])Env[ \t]*(:|=)([^=]|$)`)

// clears reports whether a line of code clears the marker; goFile says the line is Go.
func clears(code string, goFile bool) bool {
	if strings.Contains(code, "PLACKET_IN_CI") {
		for _, re := range clearNameRE {
			if re.MatchString(code) {
				return true
			}
		}
	}
	if hasWord(code, "env") {
		for _, re := range clearAllRE {
			if re.MatchString(code) {
				return true
			}
		}
	}
	if hasWord(code, "Clearenv") {
		return true
	}
	return goFile && hasWord(code, "Env") && goEnvRE.MatchString(code) && !strings.Contains(code, "Environ()")
}

// hasWord reports whether w is in s with no letter, digit or underscore on either side.
func hasWord(s, w string) bool {
	ident := func(c byte) bool {
		return c == '_' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
	}
	for i := 0; ; {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		j += i
		if (j == 0 || !ident(s[j-1])) && (j+len(w) == len(s) || !ident(s[j+len(w)])) {
			return true
		}
		i = j + 1
	}
}

// ci-stays-fast: no .sh or _test.go file outside testdata/ holds a line that clears PLACKET_IN_CI, the
// marker make ci exports so that a make ci inside it stops: an empty assignment, unset, env -u
// PLACKET_IN_CI, env -i, os.Unsetenv of it, os.Clearenv, or, in Go, an Env list set on a line that
// does not start from os.Environ(). Setting it to a value, and reading it, pass. A comment is not read.
func ciStaysFast(t *tree) []finding {
	var fs []finding
	for _, rel := range t.files(".", []string{".git", "testdata"}, func(rel string) bool {
		return strings.HasSuffix(rel, ".sh") || isTestGo(rel)
	}) {
		b, _ := t.read(rel)
		goFile := isTestGo(rel)
		raw := false
		for i, l := range splitLines(string(b)) {
			if !mayClear(l) && !(goFile && strings.Contains(l, "`")) {
				continue
			}
			code := l
			if goFile {
				code, raw = goCode(l, raw)
			} else if strings.Contains(l, "#") {
				code = shellCommentRE.ReplaceAllString(l, "")
			}
			if clears(code, goFile) {
				fs = append(fs, finding{file: rel, line: i + 1, text: strings.TrimLeft(l, " \t")})
			}
		}
	}
	return fs
}

// mayClear reports whether a line holds a word every way of clearing the marker needs.
func mayClear(l string) bool {
	return strings.Contains(l, "PLACKET_IN_CI") || strings.Contains(l, "env") || strings.Contains(l, "Env")
}

// goCode returns a line of Go without its // comment, given whether it starts inside a raw string,
// and whether it ends inside one. A /* */ comment is read as code.
func goCode(l string, raw bool) (string, bool) {
	q := byte(0)
	if raw {
		q = '`'
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case q == 0 && c == '/' && i+1 < len(l) && l[i+1] == '/':
			return l[:i], false
		case q == 0 && (c == '"' || c == '\'' || c == '`'):
			q = c
		case q != 0 && q != '`' && c == '\\':
			i++
		case q != 0 && c == q:
			q = 0
		}
	}
	return l, q == '`'
}
