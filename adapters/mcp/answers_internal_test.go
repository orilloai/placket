package mcp

import (
	"strings"
	"testing"
)

// TestAnsweringIsReadFromTheAcceptedView is U17's reading of a view (setup-contract-14): a view with
// an answers row answers, and so does one with an asked crossing whose to names a role the key
// holds; an asked crossing whose to names only roles it does not hold, a crossing that crosses or is
// blocked, and a view with neither, do not; a view of another viewer or another Accord is not the
// key's own.
func TestAnsweringIsReadFromTheAcceptedView(t *testing.T) {
	t.Parallel()
	accord, me, other := "sha256:"+strings.Repeat("a", 64), "ed25519:"+strings.Repeat("1", 64), "ed25519:"+strings.Repeat("2", 64)
	view := func(viewer, crossings, answers string) []byte {
		return []byte(`{"accord":"` + accord + `","answers":[` + answers + `],"crossings":[` + crossings + `],"salt":"00","viewer":"` + viewer + `"}`)
	}
	asked := func(to string) string {
		return `{"counterpart":"` + other + `","dir":"give","kind":"k","outcome":"asked","roles":["peer"],"timeout":60,"to":["` + to + `"]}`
	}
	row := `{"asker":"` + other + `","kind":"k","receiver":"` + other + `","timeout":60}`
	for _, c := range []struct {
		name            string
		view            []byte
		roles           []string
		answering, read bool
	}{
		{"an answers row", view(me, "", row), []string{"peer"}, true, true},
		{"an asked row whose to names its role", view(me, asked("role:approver"), ""), []string{"peer", "approver"}, true, true},
		{"an asked row whose to names another role", view(me, asked("role:approver"), ""), []string{"peer"}, false, true},
		{"a crossing that crosses", view(me, strings.Replace(asked("role:peer"), `"asked"`, `"crosses"`, 1), ""), []string{"peer"}, false, true},
		{"a crossing blocked", view(me, strings.Replace(asked("role:peer"), `"asked"`, `"blocked"`, 1), ""), []string{"peer"}, false, true},
		{"nothing", view(me, "", ""), []string{"peer"}, false, true},
		{"another viewer's view", view(other, "", row), []string{"peer"}, false, false},
		{"another Accord's view", []byte(strings.Replace(string(view(me, "", row)), accord, "sha256:"+strings.Repeat("b", 64), 1)), []string{"peer"}, false, false},
	} {
		answering, read := answers(c.view, accord, me, c.roles)
		if answering != c.answering || read != c.read {
			t.Errorf("%s: answering %v, read %v; want %v, %v", c.name, answering, read, c.answering, c.read)
		}
	}
}
