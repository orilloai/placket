package conformance

import (
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// milestones are the milestones an open entry may name as the one that writes its vector.
var milestones = map[string]bool{"M1": true, "M2": true, "M3": true}

// checkCoverage reads vectors/coverage.json against spec/ids.txt and the scenarios of
// spec/protocol.md. Every listed id and every scenario of the text has an entry; an entry names
// vectors that exist, or is open with the milestone that writes them; every vector found is named
// by some entry. found holds the vectors found, by name. It returns the problems and the open
// entries.
func checkCoverage(spec, vectors fs.FS, found map[string]bool) ([]string, []Open) {
	var problems []string
	bad := func(s string) { problems = append(problems, s) }
	const list = "vectors/coverage.json: "

	ids, err := readIDs(spec)
	if err != nil {
		bad("spec/ids.txt: " + err.Error())
	}
	scenarios, err := readScenarios(spec)
	if err != nil {
		bad("spec/protocol.md: " + err.Error())
	}
	b, err := fs.ReadFile(vectors, "coverage.json")
	if err != nil {
		bad(list + "missing or unreadable; it says what covers each requirement")
		return problems, nil
	}
	v, err := read(b)
	if err != nil {
		bad(list + err.Error())
		return problems, nil
	}
	top, ok := v.(*object)
	if !ok {
		bad(list + "not a JSON object of requirement ids")
		return problems, nil
	}

	listed := map[string]bool{}
	for _, id := range ids {
		listed[id] = true
	}
	for _, id := range top.names {
		if !listed[id] {
			bad(list + id + ": not an id spec/ids.txt lists")
		}
	}

	named := map[string]bool{}
	var open []Open
	// entry checks one entry, of a requirement or of one of its scenarios, and records what it names.
	entry := func(where string, e any, id, scenario string, withScenarios bool) *object {
		o, ok := e.(*object)
		if !ok {
			bad(list + where + ": not an object")
			return nil
		}
		_, hasVectors := o.values["vectors"]
		_, hasOpen := o.values["open"]
		for _, m := range o.names {
			if m != "vectors" && m != "open" && (m != "scenarios" || !withScenarios) {
				bad(list + where + ": a member " + strconv.Quote(m) + " an entry does not have")
				return nil
			}
		}
		switch {
		case hasVectors == hasOpen:
			bad(list + where + ": an entry has either \"vectors\" or \"open\", and this has " + map[bool]string{true: "both", false: "neither"}[hasVectors])
		case hasOpen:
			m, _ := o.values["open"].(string)
			if !milestones[m] {
				bad(list + where + ": \"open\" is not the milestone that writes its vectors, M1, M2 or M3")
				break
			}
			open = append(open, Open{ID: id, Scenario: scenario, Milestone: m})
		default:
			l, ok := o.values["vectors"].([]any)
			if !ok || len(l) == 0 {
				bad(list + where + ": \"vectors\" is not a list of vectors")
				break
			}
			for _, x := range l {
				name, ok := x.(string)
				if !ok || !vectorName(name) {
					bad(list + where + ": " + show(x) + " is not a vector's name, its group and name joined by /")
					continue
				}
				named[name] = true
				if !found[name] {
					bad(list + where + ": names " + name + ", and there is no vectors/" + name + ".json")
				}
			}
		}
		if !withScenarios {
			return nil
		}
		s, _ := o.values["scenarios"].(*object)
		if _, has := o.values["scenarios"]; has && s == nil {
			bad(list + where + ": \"scenarios\" is not an object")
		}
		return s
	}

	for _, id := range ids {
		e, ok := top.values[id]
		if !ok {
			bad(list + id + ": no entry, and spec/ids.txt lists it")
			continue
		}
		s := entry(id, e, id, "", true)
		if s == nil {
			s = &object{values: map[string]any{}}
		}
		known := map[string]bool{}
		for _, title := range scenarios[id] {
			known[title] = true
			where := id + ": scenario " + strconv.Quote(title)
			se, ok := s.values[title]
			if !ok {
				bad(list + where + ": no entry, and spec/protocol.md has it")
				continue
			}
			entry(where, se, id, title, false)
		}
		for _, title := range s.names {
			if !known[title] {
				bad(list + id + ": scenario " + strconv.Quote(title) + ": spec/protocol.md has no such scenario under " + id)
			}
		}
	}

	var unnamed []string
	for name := range found {
		if !named[name] {
			unnamed = append(unnamed, name)
		}
	}
	sort.Strings(unnamed)
	for _, name := range unnamed {
		bad("vectors/" + name + ".json: no entry of vectors/coverage.json names it")
	}
	if len(found) == 0 && len(named) > 0 {
		bad("vectors/: no vector found, and vectors/coverage.json names " + strconv.Itoa(len(named)))
	}
	return problems, open
}

// readIDs reads spec/ids.txt: one requirement id per line.
func readIDs(spec fs.FS) ([]string, error) {
	b, err := fs.ReadFile(spec, "ids.txt")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			ids = append(ids, l)
		}
	}
	return ids, nil
}

// readScenarios reads the scenario titles of spec/protocol.md under each requirement: the
// "#### Scenario: " headings between a "### <id> <title>" heading and the next heading above level 4.
func readScenarios(spec fs.FS) (map[string][]string, error) {
	b, err := fs.ReadFile(spec, "protocol.md")
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	id := ""
	for _, l := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(l, "#### Scenario: "):
			if id != "" {
				out[id] = append(out[id], strings.TrimSpace(strings.TrimPrefix(l, "#### Scenario: ")))
			}
		case strings.HasPrefix(l, "### "):
			id, _, _ = strings.Cut(strings.TrimPrefix(l, "### "), " ")
		case strings.HasPrefix(l, "# "), strings.HasPrefix(l, "## "):
			id = ""
		}
	}
	return out, nil
}
