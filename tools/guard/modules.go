package main

import (
	"regexp"
	"strconv"
	"strings"
)

// main-is-thin: cmd/placket/main.go, when it exists, is under 40 lines; the 40th is named.
func mainIsThin(t *tree) []finding {
	const f = "cmd/placket/main.go"
	if !t.isFile(f) {
		return nil
	}
	b, _ := t.read(f)
	if n := len(splitLines(string(b))); n >= 40 {
		return []finding{{file: f, line: 40, text: "main.go has " + strconv.Itoa(n) + " lines"}}
	}
	return nil
}

var recipeUpwardRE = regexp.MustCompile(`^github\.com/orilloai/placket/(adapters|cmd)(/.*)?$`)

// recipes-core-only: no Go file under recipes/, test files included and testdata/ aside, imports a
// path under adapters/ or cmd/, and no import path there is written with an escape. With no recipes/
// the check passes. A symbolic link is not followed; no-hidden-text refuses a tracked one.
func recipesCoreOnly(t *tree) []finding {
	if !t.isDir("recipes") {
		return nil
	}
	files := t.files("recipes", []string{"testdata"}, func(rel string) bool { return strings.HasSuffix(rel, ".go") })
	return eachFile(t, files, func(rel string, g *goFile) []finding {
		var out []finding
		for _, s := range g.f.Imports {
			p, ok := importPath(s)
			if !ok || recipeUpwardRE.MatchString(p) {
				out = append(out, g.at(rel, s.Path.Pos()))
			}
		}
		return out
	})
}

const coreModule = "github.com/orilloai/placket/core"

var (
	moduleRE   = regexp.MustCompile(`^[ \t]*module[ \t]`)
	blockRE    = regexp.MustCompile(`^[ \t]*(require|replace)[ \t]*[(][ \t]*$`)
	blockEndRE = regexp.MustCompile(`^[ \t]*[)]`)
	verbRE     = regexp.MustCompile(`^[ \t]*[a-z]+[ \t]+`)
	arrowRE    = regexp.MustCompile(`.*=>[ \t]*`)
)

// root-module: no go.work or go.work.sum outside testdata/; and a root go.mod, when it exists, is the
// module github.com/orilloai/placket, requires the core module and replaces it with ./core.
func rootModule(t *tree) []finding {
	var fs []finding
	for _, e := range t.walk(".", func(_, name string) bool { return name == ".git" || name == "testdata" }) {
		if n := e.path[strings.LastIndex(e.path, "/")+1:]; n == "go.work" || n == "go.work.sum" {
			fs = append(fs, finding{file: e.path, text: "a workspace file; none is committed"})
		}
	}
	if !t.isFile("go.mod") {
		return fs
	}
	b, _ := t.read("go.mod")
	mod, req, rep := false, false, false
	blk := ""
	for i, raw := range splitLines(string(b)) {
		n := i + 1
		l := stripSlashComment(raw)
		fields := strings.Fields(l)
		if moduleRE.MatchString(l) {
			mod = true
			name := ""
			if len(fields) > 1 {
				name = fields[1]
			}
			if name != "github.com/orilloai/placket" {
				fs = append(fs, finding{file: "go.mod", line: n, text: "the module is " + name + ", not github.com/orilloai/placket"})
			}
		}
		if blockRE.MatchString(l) {
			blk = fields[0]
			continue
		}
		if blk != "" && blockEndRE.MatchString(l) {
			blk = ""
			continue
		}
		kind, line := blk, l
		if blk == "" {
			if len(fields) > 0 {
				kind = fields[0]
			}
			line = verbRE.ReplaceAllString(l, "")
		}
		args := strings.Fields(line)
		if len(args) == 0 || args[0] != coreModule {
			continue
		}
		switch kind {
		case "require":
			req = true
		case "replace":
			rep = true
			to := strings.TrimRight(arrowRE.ReplaceAllString(line, ""), " \t")
			if to != "./core" && to != "./core/" {
				fs = append(fs, finding{file: "go.mod", line: n, text: coreModule + " is replaced with " + to + ", not ./core"})
			}
		}
	}
	if !mod {
		fs = append(fs, finding{file: "go.mod", text: "no module line"})
	}
	if !req {
		fs = append(fs, finding{file: "go.mod", text: "no require of " + coreModule})
	}
	if !rep {
		fs = append(fs, finding{file: "go.mod", text: "no replace of " + coreModule + " with ./core"})
	}
	return fs
}
