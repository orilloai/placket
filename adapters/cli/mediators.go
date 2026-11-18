package cli

import "github.com/orilloai/placket/adapters/client"

func init() {
	commands["mediators"] = mediatorsCommand
}

// mediatorObject is a mediator of the labels config as mediators prints it: its label when it has
// one, its Accord id, its key file, its founding set, its socket and its state directory.
func mediatorObject(m client.Mediator) map[string]any {
	o := map[string]any{"accord": m.Accord, "key": m.Key, "founding": m.Founding, "socket": m.Socket, "state": m.State}
	if m.Label != "" {
		o["label"] = m.Label
	}
	return o
}

// mediatorsCommand is `mediators`: the labels config's entries, one JSON object per line, in the
// order the file lists them, each mediatorObject, a missing config being an empty list. A config
// that is refused is its client error, which names the two entries at fault by their index for a
// check across entries: error label-taken I J or error mediator-collision I J (client.DecodeConfig).
// Its object holds mediators, the list.
func mediatorsCommand(r *run, args []string) int {
	fs := flags("mediators")
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	cfg, err := r.config()
	if err != nil {
		return r.failed(err)
	}
	list := []any{}
	for _, m := range cfg.Mediators {
		o := mediatorObject(m)
		line(r.stdout, r.screenJSON(canonicalJSON(o)))
		list = append(list, o)
	}
	r.put("mediators", list)
	return ExitOK
}
