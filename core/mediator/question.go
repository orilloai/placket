package mediator

import (
	"fmt"

	"github.com/orilloai/placket/core/internal/shape"
)

// A Question is an open question as the mediator holds it (maybe-1, record 0006): its id, the
// asker, the receiver the part is for, the asked part's kind, the form of its body (text, raw or
// data), its deadline and the role labels that may answer it, and in Part the body's value as given,
// without its salt: a text body's bytes, a raw body's decoded bytes, or a data body's canonical bytes.
// It holds a body, so it prints as a fixed text under every format verb and never prints Part
// (secrets-never-print).
type Question struct {
	ID, Asker, Receiver, Kind, Form, Deadline string
	Roles                                     []string
	Part                                      []byte
}

// Format prints the Question as a fixed text, whatever the verb.
func (Question) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("mediator.Question")) }

// GoString prints the Question as a fixed text.
func (Question) GoString() string { return "mediator.Question" }

// String prints the Question as a fixed text.
func (Question) String() string { return "mediator.Question" }

// Question returns a copy of the question open under id, to the mediator's own reference and to the
// question's answerers only (maybe-1, maybe-6, record 0006). The mediator's reference reads any open
// question before membership is asked, as the door that journals every ask must. Otherwise its rows
// run in section 6's order: a caller that is not a member is rejected with not-a-member
// (setup-contract-6); an id that names no open question returns false and no error (maybe-7); and a
// member that is not one of its answerers is rejected with not-an-answerer (maybe-6).
func (m *Mediator) Question(caller, id string) (Question, bool, error) {
	if !m.founded() {
		return Question{}, false, failure(CodeUnfounded)
	}
	var q *question
	for _, x := range m.questions {
		if x.id == id {
			q = x
			break
		}
	}
	if q != nil && caller == m.key.Reference() {
		return q.read(), true, nil
	}
	if !m.accord.Member(caller) {
		return Question{}, false, &Rejection{Code: NotAMember}
	}
	if q == nil {
		return Question{}, false, nil
	}
	if !contains(m.answerers(q.roles), caller) {
		return Question{}, false, &Rejection{Code: NotAnAnswerer}
	}
	return q.read(), true, nil
}

// read copies the question out, its body as the digest reads it (crossing-3).
func (q *question) read() Question {
	out := Question{ID: q.id, Asker: q.asker, Receiver: q.receiver, Kind: q.subject.Name,
		Deadline: q.deadline, Roles: append([]string(nil), q.roles...)}
	for _, form := range []string{"text", "raw", "data"} {
		v, has := q.delivery[form]
		if !has {
			continue
		}
		out.Form = form
		switch form {
		case "text":
			s, _ := v.(string)
			out.Part = []byte(s)
		case "raw":
			s, _ := v.(string)
			out.Part, _ = shape.Base64(s)
		case "data":
			out.Part = shape.Encode(v)
		}
	}
	return out
}
