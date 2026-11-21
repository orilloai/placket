package main

import (
	"os"
	"strconv"
	"strings"
)

// The rule outside-commits-signed-off (docs/rules/changes.md; docs/adr/0013-red-green-tests.md): no
// commit of the change is signed off by a bot, and on a pull request from outside the project every
// commit that is not a merge is signed off by its author.
const (
	signName = "outside-commits-signed-off"
	signWhy  = "every commit of an outside pull request is signed off by its author, and none by a bot (docs/rules/changes.md#outside-commits-signed-off)"
)

// openerVar names the environment variable that holds the login of whoever opened the pull request.
// Unset or empty, there is no opener: a push to main, or a run from a checkout.
const openerVar = "PLACKET_PR_OPENER"

// dependabot is the opener whose pull requests the rule exempts in full: Dependabot signs off its own
// commits in its own bot's name.
const dependabot = "dependabot[bot]"

// project lists the openers whose pull requests need no sign-off: the project's own Apps, by the login
// GitHub gives an App, and the maintainer's account. Each is matched in full.
var project = []string{"orillo-swarm[bot]", "orillo-loom[bot]", "jlindenger"}

// A change commit is one commit of the change's range, with what the rule reads of it.
type changeCommit struct {
	hash     string   // abbreviated
	author   string   // the author's email
	merge    bool     // more than one parent
	signOffs []string // each Signed-off-by value, "Name <email>"
}

// judgeSignOffs judges the commits from base to head, for the opener given, and returns the rule's
// lines, unprefixed; none when there is no opener and nothing to refuse.
func judgeSignOffs(g git, baseRev, headRev, opener string) ([]string, bool, error) {
	if opener == dependabot {
		return []string{"ok, opened by Dependabot"}, false, nil
	}
	base, err := g.commit(baseRev)
	if err != nil {
		return nil, false, err
	}
	head, err := g.commit(headRev)
	if err != nil {
		return nil, false, err
	}
	cs, err := g.changeCommits(base, head)
	if err != nil {
		return nil, false, err
	}
	inProject := false
	for _, p := range project {
		inProject = inProject || opener == p
	}
	outside := opener != "" && !inProject

	var lines []string
	signed := 0
	for _, c := range cs {
		bot, byAuthor := false, false
		for _, s := range c.signOffs {
			name, email := splitIdent(s)
			bot = bot || isBot(name) || isBot(localPart(email))
			byAuthor = byAuthor || (email != "" && strings.EqualFold(email, c.author))
		}
		switch {
		case bot:
			lines = append(lines, c.hash+": signed off by a bot")
		case !outside || c.merge:
			// No sign-off is needed: no opener, the project's pull request, or a merge.
		case byAuthor:
			signed++
		default:
			lines = append(lines, c.hash+": no sign-off by its author")
		}
	}
	switch {
	case len(lines) > 0:
		return append(lines, "refused: "+signWhy), true, nil
	case opener == "":
		return nil, false, nil
	case inProject:
		return []string{"ok, opened by the project"}, false, nil
	case signed == 1:
		return []string{"ok, 1 commit signed off by its author"}, false, nil
	}
	return []string{"ok, " + strconv.Itoa(signed) + " commits signed off by their authors"}, false, nil
}

// splitIdent splits "Name <email>" into its name and email; with no angle brackets, the whole value
// is the name.
func splitIdent(s string) (name, email string) {
	i := strings.LastIndex(s, "<")
	if i < 0 || !strings.HasSuffix(s, ">") {
		return strings.TrimSpace(s), ""
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1 : len(s)-1])
}

// localPart is the part of an email before its last @.
func localPart(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 {
		return email[:i]
	}
	return email
}

// isBot reports whether a name, or an email's local part, is a bot's: GitHub ends a bot's login in [bot].
func isBot(s string) bool { return strings.HasSuffix(strings.ToLower(s), "[bot]") }

// childEnv is the environment of every process the program starts: its own, less the opener, so that
// no test it runs passes or fails by who opened the pull request. It starts from os.Environ, as
// ci-stays-fast asks, so the marker of make ci stays.
func childEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); k != openerVar {
			env = append(env, kv)
		}
	}
	return env
}
