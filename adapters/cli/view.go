package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
)

func init() {
	commands["view"] = viewCommand
	commands["accept"] = acceptCommand
}

// The lines of a view, in the product's words (setup-contract-14): each crossing the key is a side of,
// what the terms make of it, and each question it answers.
const (
	LineViewGive    = "give %s to %s (%s): %s"
	LineViewGet     = "get %s from %s (%s): %s"
	LineViewAsked   = "ask, answered by %s within %d seconds"
	LineViewJudges  = "; judges %s"
	LineViewAnswers = "answer %s from %s to %s within %d seconds"
	LineViewHash    = "view %s"
	LineAccepted    = "accepted view %s"
	LineFounded     = "the Accord is founded: every member accepted its view"
	LineNotYet      = "the Accord is not founded yet: it waits for every member's acceptance"
)

// A shownView is a view as the command reads it to print it.
type shownView struct {
	Crossings []struct {
		Counterpart string   `json:"counterpart"`
		Roles       []string `json:"roles"`
		Dir         string   `json:"dir"`
		Kind        string   `json:"kind"`
		Outcome     string   `json:"outcome"`
		To          []string `json:"to"`
		Timeout     int64    `json:"timeout"`
		Judges      []struct {
			Name string `json:"name"`
			Use  string `json:"use"`
		} `json:"judges"`
	} `json:"crossings"`
	Answers []struct {
		Asker    string `json:"asker"`
		Receiver string `json:"receiver"`
		Kind     string `json:"kind"`
		Timeout  int64  `json:"timeout"`
	} `json:"answers"`
}

// viewLines are a view's lines, in the product's words: pass for a crossing that crosses, block for
// one blocked, with no reason, and ask, with who answers, the timeout and the judges, for one asked;
// then each question the key answers. Every value is escaped as a field: the view came from the
// window.
func viewLines(view []byte) []string {
	var v shownView
	if json.Unmarshal(view, &v) != nil {
		return nil
	}
	var out []string
	for _, c := range v.Crossings {
		what := WordBlock
		switch c.Outcome {
		case founding.Crosses:
			what = WordPass
		case founding.Asked:
			to := make([]string, len(c.To))
			for i, r := range c.To {
				to[i] = field(r)
			}
			what = fmt.Sprintf(LineViewAsked, strings.Join(to, ","), c.Timeout)
			if len(c.Judges) > 0 {
				js := make([]string, len(c.Judges))
				for i, j := range c.Judges {
					js[i] = field(j.Name) + " " + field(j.Use)
				}
				what += fmt.Sprintf(LineViewJudges, strings.Join(js, ", "))
			}
		}
		roles := make([]string, len(c.Roles))
		for i, r := range c.Roles {
			roles[i] = field(r)
		}
		f := LineViewGive
		if c.Dir == "get" {
			f = LineViewGet
		}
		out = append(out, fmt.Sprintf(f, field(c.Kind), field(c.Counterpart), strings.Join(roles, ","), what))
	}
	for _, a := range v.Answers {
		out = append(out, fmt.Sprintf(LineViewAnswers, field(a.Kind), field(a.Asker), field(a.Receiver), a.Timeout))
	}
	return out
}

// viewCommand is `view`, with the socket flags (setup-contract-14): it asks the window for the key's
// own view of the Accord, keeps it as view.json in the key's state directory, replacing any view kept
// before, and prints the view on standard output and, on standard error, one line per crossing and
// per question it answers, in the product's words, then the view's typed hash. Its --json object
// holds view, the view, hash and lines.
func viewCommand(r *run, args []string) int {
	fs := newFlags("view")
	so := socketFlags(fs)
	if !r.parseSocket(fs, args, so) {
		return r.tool(codeUsage)
	}
	s, code := r.open(so)
	if s == nil {
		return code
	}
	rep, err := s.c.View(r.env.Context)
	if err != nil {
		return r.failed(err)
	}
	if code, refused := r.refusal(rep); refused {
		return code
	}
	h, _ := canonical.TypedHash(rep.View)
	line(r.stdout, r.screenJSON(rep.View))
	lines := viewLines(rep.View)
	for _, l := range lines {
		fmt.Fprintln(r.stderr, l)
	}
	fmt.Fprintf(r.stderr, LineViewHash+"\n", h)
	if lines == nil {
		lines = []string{}
	}
	r.put("view", json.RawMessage(rep.View))
	r.put("hash", h)
	r.put("lines", lines)
	return ExitOK
}

// acceptCommand is `accept`, with the socket flags (setup-contract-15): it signs the key's acceptance
// of the view its state directory keeps, as view kept it, and hands it to the window. When the
// window takes it, it keeps the acceptance as acceptance.json beside the view, prints it on standard
// output, and on standard error the accepted line and whether the Accord is now founded. With no view
// kept it is tool error view, and a window that refuses the acceptance names its founding code. Its
// --json object holds acceptance and founded.
func acceptCommand(r *run, args []string) int {
	fs := newFlags("accept")
	so := socketFlags(fs)
	if !r.parseSocket(fs, args, so) {
		return r.tool(codeUsage)
	}
	s, code := r.open(so)
	if s == nil {
		return code
	}
	rep, acc, err := s.c.Accept(r.env.Context)
	if err != nil {
		return r.failed(err)
	}
	if code, refused := r.refusal(rep); refused {
		return code
	}
	line(r.stdout, acc)
	fmt.Fprintf(r.stderr, LineAccepted+"\n", memberString(acc, "view"))
	if rep.Founded {
		fmt.Fprintln(r.stderr, LineFounded)
	} else {
		fmt.Fprintln(r.stderr, LineNotYet)
	}
	r.put("acceptance", json.RawMessage(acc))
	r.put(wordFounded, rep.Founded)
	return ExitOK
}
