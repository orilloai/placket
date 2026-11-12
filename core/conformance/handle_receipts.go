package conformance

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	vreceipts "github.com/orilloai/placket/core/receipts"
)

func init() {
	registered["verify"] = runVerify
	registered["reconcile"] = runReconcile
}

// receiptList reads a list of receipts, each given in one of its forms.
func receiptList(raw json.RawMessage) ([][]byte, bool) {
	var items []members
	if json.Unmarshal(raw, &items) != nil {
		return nil, false
	}
	out := make([][]byte, 0, len(items))
	for _, it := range items {
		b, ok := bytesOf(it, "receipt", "receiptText", "receiptHex")
		if !ok {
			return nil, false
		}
		out = append(out, b)
	}
	return out, true
}

// receiptsError writes a verifier's or reconciliation's failure as a vector states it, a kept
// party by its name.
func receiptsError(err error, names map[string]string) []byte {
	var re *vreceipts.Error
	if !errors.As(err, &re) {
		return jsonOf(map[string]any{"error": err.Error()})
	}
	o := map[string]any{"code": re.Code}
	if re.Code != vreceipts.CodeReference {
		o["index"] = re.Index
	}
	if re.Actor != "" {
		o["actor"] = names[re.Actor]
	}
	return jsonOf(o)
}

// runVerify verifies the receipts under reference and compares verified, or the code and index.
func runVerify(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	list, ok := receiptList(m["receipts"])
	if !ok {
		return bad("receipts", "a receipt does not decode")
	}
	statements, err := vreceipts.Verify(m.str("reference"), list)
	var got []byte
	if err == nil {
		got = jsonOf(map[string]any{"verified": len(statements)})
	} else {
		got = receiptsError(err, nil)
	}
	return []Part{compare("expect", got, m["expect"])}
}

// runReconcile reconciles the receipts with each party's kept records and compares the three lists,
// or the code.
func runReconcile(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	list, ok := receiptList(m["receipts"])
	if !ok {
		return bad("receipts", "a receipt does not decode")
	}
	var parties map[string]string
	var kept map[string]members
	if json.Unmarshal(m["parties"], &parties) != nil || json.Unmarshal(m["kept"], &kept) != nil {
		return bad("kept", "the parties or the kept records do not decode")
	}
	names := map[string]string{}
	for name, ref := range parties {
		names[ref] = name
	}
	var records []vreceipts.Kept
	for name, k := range kept {
		ref, ok := parties[name]
		if !ok {
			return bad("kept", "the party "+strconv.Quote(name)+" is not in parties")
		}
		var envs []struct {
			Seq  int64  `json:"seq"`
			Hash string `json:"hash"`
		}
		if json.Unmarshal(k["envelopes"], &envs) != nil {
			return bad("kept", "the envelopes of "+strconv.Quote(name)+" do not decode")
		}
		delivered, ok := receiptList(k["delivered"])
		if !ok {
			return bad("kept", "a delivered receipt of "+strconv.Quote(name)+" does not decode")
		}
		r := vreceipts.Kept{Actor: ref, Delivered: delivered}
		for _, e := range envs {
			r.Envelopes = append(r.Envelopes, vreceipts.Envelope{Sender: ref, Seq: e.Seq, Hash: e.Hash})
		}
		records = append(records, r)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Actor < records[j].Actor })
	rep, err := vreceipts.Reconcile(m.str("reference"), m.str("accord"), m.str("card"), list, records)
	var got []byte
	if err != nil {
		got = receiptsError(err, names)
	} else {
		items := func(l []vreceipts.Item) []any {
			out := []any{}
			for _, it := range l {
				out = append(out, map[string]any{"actor": names[it.Actor], "receiptId": it.ReceiptID})
			}
			return out
		}
		missing := []any{}
		for _, it := range rep.Missing {
			missing = append(missing, map[string]any{"actor": names[it.Actor], "seq": it.Seq})
		}
		got = jsonOf(map[string]any{"removed": items(rep.Removed), "unrecorded": items(rep.Unrecorded), "missing": missing})
	}
	return []Part{compare("expect", got, m["expect"])}
}
