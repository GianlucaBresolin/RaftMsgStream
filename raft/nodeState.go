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

const MinElectionTimeout = 150
const MaxElectionTimeout = 300

const CandidateTimeout = 10

const LeaderTimeout = 20

type ServerID string
type Port string

type nodeState struct {
	id              ServerID
	state           uint
	term            uint
	numberNodes     uint
	peers           map[ServerID]Port
	peersConnection map[ServerID]*rpc.Client
	// elction logic
	electionTimer  *time.Timer
	electionVotes  int
	voteResponseCh chan RequestVoteResult
	voteRequestCh  chan RequestVoteArguments
	myVote         ServerID
	currentLeader  ServerID
	// leader logic
	leaderCh  chan bool
	nextIndex map[ServerID]uint
	// log logic
	log           logStruct
	logEntriesCh  chan struct{}
	pendingCommit map[uint]replicationState
	// mutex
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
		nextIndex:       nil,
		log: logStruct{
			entries: []LogEntry{
				// to start the log from index 1 we add a default entry
				{
					Index:   0,
					Term:    0,
					Command: "",
				},
			},
			lastCommitedIndex: 0,
		},
		logEntriesCh:  make(chan struct{}),
		pendingCommit: make(map[uint]replicationState),
	}
}

func (ns *nodeState) handleNodeState() {
	ns.startTimer()
	go ns.handleTimer()

	go ns.askForVotes()
	go ns.handleVotes()
}
