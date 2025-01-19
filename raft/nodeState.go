package raft

import (
	"net/rpc"
	"sync"
	"time"
)

const (
	Follower  = 0
	Candidate = 1
	Leader    = 2
)

type ServerID string
type Port string

type nodeState struct {
	id              ServerID
	state           uint
	term            uint
	numberNodes     uint
	peers           map[ServerID]Port
	peersConnection map[ServerID]*rpc.Client
	//elction logic
	electionTimer  *time.Timer
	electionVotes  int
	voteResponseCh chan RequestVoteResult
	voteRequestCh  chan RequestVoteArguments
	myVote         ServerID
	currentLeader  ServerID
	//leader logic
	leaderCh chan bool
	//log logic
	log           logStruct
	logEntriesCh  chan *LogEntry
	pendingCommit map[uint]chan bool
	//mutex
	mutex sync.Mutex
}

func newNodeState(id ServerID, peers map[ServerID]Port) *nodeState {
	return &nodeState{
		id:              id,
		term:            0,
		state:           Follower,
		numberNodes:     uint(len(peers) + 1),
		peers:           peers,
		peersConnection: make(map[ServerID]*rpc.Client),
		voteResponseCh:  make(chan RequestVoteResult, len(peers)),
		voteRequestCh:   make(chan RequestVoteArguments, 1),
		currentLeader:   "",
		leaderCh:        make(chan bool),
		log: logStruct{
			entries:           make(map[uint]LogEntry),
			lastCommitedIndex: 0,
		},
		logEntriesCh:  make(chan *LogEntry),
		pendingCommit: make(map[uint]chan bool),
	}
}

func (ns *nodeState) handleNodeState() {
	ns.startTimer()
	go ns.handleTimer()

	go ns.askForVotes()
	go ns.handleVotes()
}
