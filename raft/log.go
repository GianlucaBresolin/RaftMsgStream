package raft

type LogEntry struct {
	Index   uint
	Term    uint
	Command string
	// idempotent logic
	Client string
	USN    uint
}

type logStruct struct {
	entries           []LogEntry
	lastCommitedIndex uint
}

func (l *logStruct) lastIndex() uint {
	return uint(len(l.entries) - 1)
}

func (l *logStruct) lastTerm() uint {
	return l.entries[l.lastIndex()].Term
}
