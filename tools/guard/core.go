package main

import (
	"go/ast"
	"go/build/constraint"
	"go/token"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// The core's checks read core/. Each first refuses a missing core/ and every symbolic link under it,
// core/ and core/go.mod included: the checker does not follow a link, and the Go tool does, so a link
// would carry in code no check has read.

// coreBase returns the findings every check of core/ starts with, and false when core/ is missing.
func coreBase(t *tree) ([]finding, bool) {
	if !t.isDir("core") {
		return []finding{{file: "core/", text: "missing; the core is its own module there"}}, false
	}
	var fs []finding
	for _, e := range t.walk("core", nil) {
		if e.link {
			fs = append(fs, finding{file: e.path, text: "a symbolic link"})
		}
	}
	return fs, true
}

// coreGo lists the Go files under core/: with all, every one, test files and testdata included;
// without it, the files built into the core, no _test.go file, nothing under testdata/ and nothing
// under core/internal/testing/, the test helpers only tests import.
func coreGo(t *tree, all bool) []string {
	if t.isLink("core") {
		return nil
	}
	return t.files("core", nil, func(rel string) bool {
		if !strings.HasSuffix(rel, ".go") {
			return false
		}
		return all || (!isTestGo(rel) && !inTestdata(rel) && !inTestHelpers(rel))
	})
}

// eachFile parses each file and calls fn on those that parse, adding a finding for each that does not.
func eachFile(t *tree, files []string, fn func(rel string, g *goFile) []finding) []finding {
	var fs []finding
	for _, rel := range files {
		g := t.parse(rel)
		if g.f == nil {
			fs = append(fs, g.unparsed(rel))
			continue
		}
		fs = append(fs, fn(rel, g)...)
	}
	return fs
}

var requireRE = regexp.MustCompile(`^[ \t]*require([ \t(]|$)`)

// core-no-deps: core/go.mod has no require directive, single or block.
func coreNoDeps(t *tree) []finding {
	if !t.isFile("core/go.mod") {
		return []finding{{file: "core/go.mod", text: "missing; the core is its own module"}}
	}
	fs, ok := coreBase(t)
	if !ok || t.isLink("core") || t.isLink("core/go.mod") {
		return fs
	}
	src, _ := t.read("core/go.mod")
	for i, l := range splitLines(string(src)) {
		if requireRE.MatchString(stripSlashComment(l)) {
			fs = append(fs, finding{file: "core/go.mod", line: i + 1, text: strings.TrimLeft(l, " \t")})
		}
	}
	return fs
}

// stripSlashComment removes a // comment from a line of go.mod, which holds no string with "//" in it.
func stripSlashComment(l string) string {
	if i := strings.Index(l, "//"); i >= 0 {
		return l[:i]
	}
	return l
}

var upwardRE = regexp.MustCompile(`^github\.com/orilloai/placket/(adapters|cmd|recipes)(/.*)?$`)

// importPath returns an import's path, and false when it is written with an escape: a backslash
// anywhere in the literal, which would hide the path from a reader of the text.
func importPath(s *ast.ImportSpec) (string, bool) {
	if strings.Contains(s.Path.Value, `\`) {
		return "", false
	}
	p, err := strconv.Unquote(s.Path.Value)
	return p, err == nil
}

// core-imports: no Go file under core/, test files and testdata included, imports a path under
// adapters/, cmd/ or recipes/, and no import path under core/ is written with an escape.
func coreImports(t *tree) []finding {
	fs, ok := coreBase(t)
	if !ok {
		return fs
	}
	return append(fs, eachFile(t, coreGo(t, true), func(rel string, g *goFile) []finding {
		var out []finding
		for _, s := range g.f.Imports {
			p, ok := importPath(s)
			if !ok || upwardRE.MatchString(p) {
				out = append(out, g.at(rel, s.Path.Pos()))
			}
		}
		return out
	})...)
}

// worldPkgRE matches the packages core-no-world refuses by import path, under any name: randomness,
// the network, the process and its files and environment, logs and memory, and those below them.
var worldPkgRE = regexp.MustCompile(`^(crypto/rand|math/rand|net|os|syscall|io/ioutil|log|runtime|unsafe)(/.*)?$`)

// clockCalls are the names of package time that read the clock, or wait on it or measure against it.
var clockCalls = map[string]bool{
	"Now": true, "Since": true, "Until": true, "After": true, "AfterFunc": true,
	"Tick": true, "NewTimer": true, "NewTicker": true, "Sleep": true,
}

// nonGoSuffixes are the files the Go tool builds into a package that are not Go: assembly, C, C++,
// Objective-C, Fortran, SWIG and prebuilt objects.
var nonGoSuffixes = []string{".s", ".S", ".sx", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx",
	".m", ".f", ".F", ".for", ".f90", ".swig", ".swigcxx", ".syso"}

// packageNames returns, for each directory and package clause, the names declared at package level in
// the files every build of the package includes, so that a call of print, println or panic can be told
// from a call of the package's own function of that name. A file some build leaves out does not count:
// the call reaches the built-in in that build.
func packageNames(t *tree, files []string) map[string]map[string]bool {
	names := map[string]map[string]bool{}
	for _, rel := range files {
		g := t.parse(rel)
		if g.f == nil || !alwaysBuilt(rel, g.f) {
			continue
		}
		k := pkgKey(rel, g.f)
		if names[k] == nil {
			names[k] = map[string]bool{}
		}
		for _, decl := range g.f.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil {
					names[k][decl.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, s := range decl.Specs {
					switch s := s.(type) {
					case *ast.ValueSpec:
						for _, n := range s.Names {
							names[k][n.Name] = true
						}
					case *ast.TypeSpec:
						names[k][s.Name.Name] = true
					}
				}
			}
		}
	}
	return names
}

// pkgKey names a file's package: its directory and its package clause.
func pkgKey(rel string, f *ast.File) string { return dir(rel) + "\x00" + f.Name.Name }

// alwaysBuilt reports whether every build of the file's package includes it: its name does not start
// with "_" or ".", it has no system or architecture suffix, and no build line comes before its package
// clause, in either the //go:build or the // +build form.
func alwaysBuilt(rel string, f *ast.File) bool {
	base := path.Base(rel)
	if strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".") || !goodOSArchName(base) {
		return false
	}
	for _, cg := range f.Comments {
		if cg.Pos() >= f.Package {
			break
		}
		for _, c := range cg.List {
			if constraint.IsGoBuild(c.Text) || constraint.IsPlusBuild(c.Text) {
				return false
			}
		}
	}
	return true
}

// goodOSArchName reports whether a Go file's name carries no system or architecture suffix, the
// _GOOS, _GOARCH and _GOOS_GOARCH endings, before _test, by which the Go tool builds a file only there.
// "unix" counts as a system here, though the Go tool does not read it from a name: a stricter reading
// only refuses more.
func goodOSArchName(base string) bool {
	name := strings.TrimSuffix(base, ".go")
	i := strings.Index(name, "_")
	if i < 0 {
		return true
	}
	l := strings.Split(name[i:], "_")
	if n := len(l); n > 0 && l[n-1] == "test" {
		l = l[:n-1]
	}
	n := len(l)
	if n >= 2 && knownOS[l[n-2]] && knownArch[l[n-1]] {
		return false
	}
	return !(n >= 1 && (knownOS[l[n-1]] || knownArch[l[n-1]]))
}

// knownOS and knownArch are the systems and architectures the Go tool reads from a file's name.
var knownOS = setOf("aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "js",
	"linux", "nacl", "netbsd", "openbsd", "plan9", "solaris", "unix", "wasip1", "windows", "zos")

var knownArch = setOf("386", "amd64", "amd64p32", "arm", "armbe", "arm64", "arm64be", "loong64", "mips",
	"mipsle", "mips64", "mips64le", "mips64p32", "mips64p32le", "ppc", "ppc64", "ppc64le", "riscv",
	"riscv64", "s390", "s390x", "sparc", "sparc64", "wasm")

func setOf(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// builtinCalls returns each call in the file of one of the built-ins named, at its line, unless a file
// of the same package clause that every build includes declares a name of its own that the call
// reaches instead.
func builtinCalls(rel string, g *goFile, declared map[string]bool, names ...string) []finding {
	var out []finding
	ast.Inspect(g.f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := c.Fun.(*ast.Ident); ok && contains(names, id.Name) && !declared[id.Name] {
			out = append(out, g.at(rel, id.Pos()))
		}
		return true
	})
	return out
}

// testHelpers is the import path of the core's test helpers, which may read the world because only
// tests import them; a file built into the core that imports them is refused by core-no-world.
const testHelpers = "github.com/orilloai/placket/core/internal/testing"

// inTestHelpers reports whether a path under core/ is one of the test helpers.
func inTestHelpers(rel string) bool { return strings.HasPrefix(rel, "core/internal/testing/") }

// core-no-world: no file built into the core reads the clock, draws randomness, reads the environment,
// opens a file, touches the network, reaches the process, its files or its memory, logs or prints; it
// imports no "C" and no path written with an escape; and core/ holds no source file that is not Go
// outside testdata/.
//
// The packages are refused by import path, under any name. The clock's calls and fmt's Print* are
// refused wherever they are referred to, called or not, through whatever name the file imports time
// or fmt under; a dot import of either is refused itself. os.Getenv and os.Open are also refused by
// name. print and println are refused as calls of the built-ins.
func coreNoWorld(t *tree) []finding {
	fs, ok := coreBase(t)
	if !ok {
		return fs
	}
	if !t.isLink("core") {
		for _, rel := range t.files("core", nil, func(rel string) bool { return !inTestdata(rel) }) {
			for _, s := range nonGoSuffixes {
				if strings.HasSuffix(rel, s) {
					fs = append(fs, finding{file: rel, text: "a source file that is not Go"})
					break
				}
			}
		}
	}
	files := coreGo(t, false)
	declared := packageNames(t, files)
	return append(fs, eachFile(t, files, func(rel string, g *goFile) []finding {
		var out []finding
		clock, printer := map[string]bool{}, map[string]bool{}
		for _, s := range g.f.Imports {
			p, ok := importPath(s)
			if !ok || p == "C" || worldPkgRE.MatchString(p) || p == testHelpers || strings.HasPrefix(p, testHelpers+"/") {
				out = append(out, g.at(rel, s.Path.Pos()))
				continue
			}
			if p != "time" && p != "fmt" {
				continue
			}
			name := p
			if s.Name != nil {
				name = s.Name.Name
			}
			switch {
			case name == ".":
				out = append(out, g.at(rel, s.Path.Pos()))
			case name == "_":
			case p == "time":
				clock[name] = true
			default:
				printer[name] = true
			}
		}
		ast.Inspect(g.f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			s := sel.Sel.Name
			if (clock[x.Name] && clockCalls[s]) || (printer[x.Name] && strings.HasPrefix(s, "Print")) ||
				(x.Name == "os" && (s == "Getenv" || s == "Open")) {
				out = append(out, g.at(rel, sel.Sel.Pos()))
			}
			return true
		})
		return append(out, builtinCalls(rel, g, declared[pkgKey(rel, g.f)], "print", "println")...)
	})...)
}

// core-no-panic: no file built into the core calls the built-in panic.
func coreNoPanic(t *tree) []finding {
	fs, ok := coreBase(t)
	if !ok {
		return fs
	}
	files := coreGo(t, false)
	declared := packageNames(t, files)
	return append(fs, eachFile(t, files, func(rel string, g *goFile) []finding {
		return builtinCalls(rel, g, declared[pkgKey(rel, g.f)], "panic")
	})...)
}

// product-words-once: no file built into the core spells "ask" or "released" as a whole string
// literal, in either quote form.
func productWordsOnce(t *tree) []finding {
	fs, ok := coreBase(t)
	if !ok {
		return fs
	}
	return append(fs, eachFile(t, coreGo(t, false), func(rel string, g *goFile) []finding {
		var out []finding
		ast.Inspect(g.f, func(n ast.Node) bool {
			if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING {
				if v := literalValue(l); v == "ask" || v == "released" {
					out = append(out, g.at(rel, l.Pos()))
				}
			}
			return true
		})
		return out
	})...)
}

// literalValue is a string literal's value, or its text between the quotes when it does not unquote.
func literalValue(l *ast.BasicLit) string {
	if v, err := strconv.Unquote(l.Value); err == nil {
		return v
	}
	return l.Value[1 : len(l.Value)-1]
}
