package raft

const (
	ActionEntry        = 0
	ConfigurationEntry = 1
	NOOPEntry          = 2
)

type LogEntry struct {
	Index   uint
	Term    uint
	Command []byte
	Type    uint
	// idempotent logic
	Client string
	USN    int
}

type logStruct struct {
	entries           []LogEntry
	lastCommitedIndex uint
}

func (l *logStruct) lastIndex() uint {
	return uint(len(l.entries) - 1)
}

func (rn *RaftNode) lastGlobalIndex() uint {
	return rn.log.lastIndex() + rn.snapshot.LastIndex
}

func (rn *RaftNode) lastGlobalCommitedIndex() uint {
	return rn.log.lastCommitedIndex + rn.snapshot.LastIndex
}

func (l *logStruct) lastTerm() uint {
	return l.entries[l.lastIndex()].Term
}
