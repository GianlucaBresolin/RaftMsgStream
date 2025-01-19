package raft

type LogEntry struct {
	Index     uint
	Term      uint
	Command   string
	Committed bool
}

type logStruct struct {
	entries           map[uint]LogEntry
	lastCommitedIndex uint
}

func (l *logStruct) lastIndex() uint {
	return uint(len(l.entries))
}

func (l *logStruct) lastTerm() uint {
	//TODO: implement
	return 1
}
