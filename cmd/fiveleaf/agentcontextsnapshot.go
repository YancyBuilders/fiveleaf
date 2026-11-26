package main

// The total comes from the active ACP session. Categories may be estimated
// from visible content; unavailable adapter internals remain explicitly unknown.
type agentContextSnapshot struct {
	known                              bool
	used, size                         int
	model                              string
	compactionKnown, compactionEnabled bool
	compactAt                          int
	compactionStatus                   agentCompactionStatus
	estimated                          bool
	categories                         []agentContextCategory
}

type agentCompactionStatus uint8

const (
	agentCompactionUnreported agentCompactionStatus = iota
	agentCompactionManaged
	agentCompactionRunning
	agentCompactionCompleted
	agentCompactionFailed
)

type agentContextCategory struct {
	kind   string
	tokens int
	known  bool
}
