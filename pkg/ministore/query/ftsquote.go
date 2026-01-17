package query

// QuoteFTSTerm quotes/escapes a user term so backend FTS MATCH parsers treat it as intended.
//
// TODO: implement with the same semantics as the Rust reference.
func QuoteFTSTerm(term string) string {
	return term
}
