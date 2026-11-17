// Package jev is the judge adapter for Jev, a decision model a user calls with their own key, behind
// the provider-neutral judge.Judge (docs/adr/0023-judges.md).
//
// What it sends, in placket's terms: one HTTPS request, a POST to the config's host at path, that
// carries the config's model, the config's instruction, the part's kind and the part's body, with
// the key as the request's bearer credential; and the adapter's own fixed words for the two labels it
// asks Jev to choose between. Nothing else of the part, the message, the envelope or the Accord.
//
// What it reads back: the label Jev chose, pass or block, as the verdict; the confidence Jev gives
// that choice, from 0 to 1, as a score from 0 to 100; and no model text, since Jev returns none. Jev
// has no abstain label, so the adapter never answers abstain: a reply it cannot map is unparsable,
// and the runner reads it as none. A reply with no confidence has no score, and the threshold is then
// unused.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"unicode/utf8"

	"github.com/orilloai/placket/adapters/judge"
)

// Version is this adapter's version, the card's version for a Jev judge (setup-contract-4). It is
// raised when the adapter changes what it sends or how it reads a reply.
const Version = "0.1.0"

// KeyEnv is the environment variable the user's Jev key is read from, once, when the window or the
// eval starts. The key is held only in memory and written nowhere.
const KeyEnv = "PLACKET_JEV_KEY"

// path is the one path a request goes to, on the config's host.
const path = "/v1/systemone"

// question is the name of the one question a request asks, and its answer's name in the reply.
const question = "verdict"

// The adapter's own words for the two labels Jev chooses between.
const (
	criterionPass  = "The content meets the instruction."
	criterionBlock = "The content does not meet the instruction."
)

// New returns the Jev judge of cfg, under key, through rt. It takes cfg.Host as given: ReadConfig
// alone checks it. An empty key sends nothing and is no-key. A nil rt is a transport that uses no
// proxy from the environment and verifies TLS; the client never follows a redirect.
func New(cfg judge.Config, key string, rt http.RoundTripper) judge.Judge {
	if rt == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil
		rt = t
	}
	return &jev{cfg: cfg, key: key, client: &http.Client{Transport: rt,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// jev is a Jev judge. It holds the key, so it prints as a fixed text under every format verb
// (secrets-never-print).
type jev struct {
	cfg    judge.Config
	key    string
	client *http.Client
}

// Format prints the judge as a fixed text, whatever the verb.
func (jev) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("jev.judge")) }

// GoString prints the judge as a fixed text.
func (jev) GoString() string { return "jev.judge" }

// String prints the judge as a fixed text.
func (jev) String() string { return "jev.judge" }

// Judge asks Jev about r. With ctx done it returns ctx's error, and the runner says timeout.
func (j *jev) Judge(ctx context.Context, r judge.Request) (judge.Answer, error) {
	if j.key == "" {
		return judge.Answer{}, &judge.Error{Code: judge.CodeNoKey}
	}
	if !utf8.Valid(r.Body) {
		return judge.Answer{}, &judge.Error{Code: judge.CodeRawBody}
	}
	body, err := requestBody(j.cfg.Model, r)
	if err != nil {
		return judge.Answer{}, &judge.Error{Code: judge.CodeUnreachable}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+j.cfg.Host+path, bytes.NewReader(body))
	if err != nil {
		return judge.Answer{}, &judge.Error{Code: judge.CodeUnreachable}
	}
	req.Header.Set("Authorization", "Bearer "+j.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := j.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return judge.Answer{}, ctx.Err()
		}
		return judge.Answer{}, &judge.Error{Code: judge.CodeUnreachable}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return judge.Answer{}, &judge.Error{Code: judge.CodeRedirect}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return judge.Answer{}, &judge.Error{Code: judge.CodeStatus}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, judge.MaxOutput+1))
	if err != nil {
		if ctx.Err() != nil {
			return judge.Answer{}, ctx.Err()
		}
		return judge.Answer{}, &judge.Error{Code: judge.CodeUnreachable}
	}
	if len(b) > judge.MaxOutput {
		return judge.Answer{}, &judge.Error{Code: judge.CodeTooLarge}
	}
	a, ok := readReply(b)
	if !ok {
		return judge.Answer{}, &judge.Error{Code: judge.CodeUnparsable}
	}
	return a, nil
}

// requestBody is the request's JSON: the model, the one choice question with the instruction and the
// two labels, and the state, which holds the kind and the body and nothing else.
func requestBody(model string, r judge.Request) ([]byte, error) {
	type choice struct {
		Type         string            `json:"type"`
		Instructions string            `json:"instructions"`
		Criteria     map[string]string `json:"criteria"`
	}
	v := struct {
		Model     string            `json:"model"`
		State     map[string]string `json:"state"`
		Questions map[string]choice `json:"questions"`
	}{
		Model: model,
		State: map[string]string{"kind": r.Kind, "body": string(r.Body)},
		Questions: map[string]choice{question: {Type: "choice", Instructions: r.Instruction,
			Criteria: map[string]string{"pass": criterionPass, "block": criterionBlock}}},
	}
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}

// readReply reads Jev's reply strictly (parse-strict): one JSON object and nothing after it, with no
// name twice in any object, whose answers hold the question's answer, of type choice, whose choice is
// pass or block, and whose confidence, when it has one, is a number from 0 to 1. Members are matched
// by their exact names; members it does not read are left alone.
func readReply(b []byte) (judge.Answer, bool) {
	if !distinct(b) {
		return judge.Answer{}, false
	}
	top, ok := object(b)
	if !ok {
		return judge.Answer{}, false
	}
	answers, ok := object(top["answers"])
	if !ok {
		return judge.Answer{}, false
	}
	ans, ok := object(answers[question])
	if !ok {
		return judge.Answer{}, false
	}
	var typ, label string
	if json.Unmarshal(ans["type"], &typ) != nil || typ != "choice" || json.Unmarshal(ans["choice"], &label) != nil {
		return judge.Answer{}, false
	}
	if label != "pass" && label != "block" {
		return judge.Answer{}, false
	}
	a := judge.Answer{Verdict: label}
	if c, has := ans["confidence"]; has {
		var conf float64
		if json.Unmarshal(c, &conf) != nil || math.IsNaN(conf) || conf < 0 || conf > 1 {
			return judge.Answer{}, false
		}
		s := int(math.Round(conf * 100))
		a.Score = &s
	}
	return a, true
}

// object reads b as one JSON object, by its members' exact names; false for anything else, or for
// nothing.
func object(b json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(b) == 0 {
		return nil, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil || m == nil {
		return nil, false
	}
	return m, true
}

// distinct reports whether b is one JSON value and nothing after it, with no name twice in any of
// its objects.
func distinct(b []byte) bool {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if !value(d, 0) {
		return false
	}
	_, err := d.Token()
	return errors.Is(err, io.EOF)
}

// maxDepth is the deepest nesting distinct reads.
const maxDepth = 64

// value reads one value from d, checking each object's names are distinct.
func value(d *json.Decoder, depth int) bool {
	if depth > maxDepth {
		return false
	}
	t, err := d.Token()
	if err != nil {
		return false
	}
	switch t {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			n, err := d.Token()
			name, ok := n.(string)
			if err != nil || !ok || seen[name] {
				return false
			}
			seen[name] = true
			if !value(d, depth+1) {
				return false
			}
		}
		_, err := d.Token()
		return err == nil
	case json.Delim('['):
		for d.More() {
			if !value(d, depth+1) {
				return false
			}
		}
		_, err := d.Token()
		return err == nil
	}
	return true
}
