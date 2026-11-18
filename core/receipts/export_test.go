package receipts

// ReleasesOf is releases over a whole list, for the tests that hold receipt-6's rules over
// statements no key could sign.
func ReleasesOf(statements []Statement) ([]Place, error) { return releases(statements, false) }
