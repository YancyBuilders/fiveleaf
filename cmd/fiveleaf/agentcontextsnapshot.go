package main

// The total comes from the active ACP session. Categories may be estimated
// from visible content; unavailable adapter internals remain explicitly unknown.
type agentContextSnapshot struct {
	known                              bool
	used, size                         int
	model                              string
	compactionKnown, compactionEnabled bool
	compactAt                          int
	estimated                          bool
	categories                         []agentContextCategory
}

type agentContextCategory struct {
	kind   string
	tokens int
	known  bool
}
