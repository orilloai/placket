package receipts

// ReleasesOf is releases over a whole list, for the tests that hold receipt-6's rules over
// statements no key could sign.
func ReleasesOf(statements []Statement) ([]Place, error) {
	entries := make([]entry, len(statements))
	for i, s := range statements {
		entries[i] = entry{s: s}
	}
	return releases(entries, false)
}
