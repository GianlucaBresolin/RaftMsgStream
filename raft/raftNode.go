package raft

import (
	"log"
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

const snapshotThreshold = 5

type ServerID string
type Port string

type Configuration struct {
	OldConfig map[ServerID]Port
	NewConfig map[ServerID]Port
}

type RaftNode struct {
	id              ServerID
	port            Port
	state           uint
	term            uint
	peers           Configuration
	peersConnection map[ServerID]*rpc.Client
	shutdownCh      chan struct{}
	// elction logic
	electionTimer         *time.Timer
	minimumTimer          *time.Timer
	shutdownTimers        chan struct{}
	electionVotesNewC     int
	electionVotesOldC     int
	voteResponseCh        chan RequestVoteResultWithServerID
	shutdownAskForVotesCh chan struct{}
	voteRequestCh         chan RequestVoteArguments
	shutdownHandleVotesCh chan struct{}
	myVote                ServerID
	currentLeader         ServerID
	// leader logic
	leaderCh         chan bool
	firstHeartbeatCh chan struct{}
	nextIndex        map[ServerID]uint
	USN              int
	// log logic
	log           logStruct
	logEntriesCh  chan struct{}
	pendingCommit map[uint]replicationState
	lastUSNof     map[string]int
	// state machine logic
	CommitCh          chan []byte
	ApplySnapshotCh   chan []byte
	ReadStateCh       chan []byte
	ReadStateResultCh chan []byte
	// snapshot logic
	takeSnapshotCh     chan struct{}
	SnapshotRequestCh  chan struct{}
	SnapshotResponseCh chan []byte
	snapshot           *Snapshot
	// unvoting logic
	unvotingServer  bool
	unvotingServers map[ServerID]string
	// mutex
	mutex sync.Mutex
}

func NewRaftNode(id ServerID,
	port Port,
	peers map[ServerID]Port,
	unvoting bool) *RaftNode {
	peers[id] = "" // add self to the peers list
	raftNode := &RaftNode{
		id:                    id,
		port:                  port,
		term:                  0,
		state:                 Follower,
		peers:                 Configuration{OldConfig: nil, NewConfig: peers},
		peersConnection:       make(map[ServerID]*rpc.Client),
		shutdownCh:            make(chan struct{}),
		shutdownTimers:        make(chan struct{}),
		shutdownHandleVotesCh: make(chan struct{}),
		voteResponseCh:        make(chan RequestVoteResultWithServerID, len(peers)),
		shutdownAskForVotesCh: make(chan struct{}),
		voteRequestCh:         make(chan RequestVoteArguments, 1),
		currentLeader:         "",
		leaderCh:              make(chan bool),
		firstHeartbeatCh:      make(chan struct{}),
		nextIndex:             nil,
		log: logStruct{
			entries: []LogEntry{
				// to start the log from index 1 we add a dummy entry
				{
					Index:   0,
					Term:    0,
					Command: nil,
					Type:    NOOPEntry,
					Client:  "",
					USN:     -1,
				},
			},
			lastCommitedIndex: 0,
		},
		logEntriesCh:       make(chan struct{}),
		pendingCommit:      make(map[uint]replicationState),
		lastUSNof:          make(map[string]int),
		CommitCh:           make(chan []byte),
		ApplySnapshotCh:    make(chan []byte),
		ReadStateCh:        make(chan []byte),
		ReadStateResultCh:  make(chan []byte),
		takeSnapshotCh:     make(chan struct{}),
		SnapshotRequestCh:  make(chan struct{}),
		SnapshotResponseCh: make(chan []byte),
		snapshot: &Snapshot{
			LastIndex:        0,
			LastTerm:         0,
			LastConfig:       nil,
			StateMachineSnap: nil,
		},
		unvotingServer:  unvoting,
		unvotingServers: make(map[ServerID]string),
	}
	raftNode.registerNode()
	return raftNode
}

func (rn *RaftNode) closeChannels() {
	close(rn.shutdownCh)
	close(rn.shutdownTimers)
	close(rn.shutdownAskForVotesCh)
	close(rn.shutdownHandleVotesCh)
	close(rn.voteResponseCh)
	close(rn.voteRequestCh)
	close(rn.firstHeartbeatCh)
	close(rn.leaderCh)
	close(rn.takeSnapshotCh)
	close(rn.logEntriesCh)
	close(rn.CommitCh)
	close(rn.ApplySnapshotCh)
	close(rn.ReadStateCh)
	close(rn.ReadStateResultCh)
	close(rn.SnapshotRequestCh)
	close(rn.SnapshotResponseCh)
}

func (rn *RaftNode) handleUnvotingNode() {
	rn.connectAsUnvotingNode()

	for rn.unvotingServer {
	}
}

func (rn *RaftNode) HandleRaftNode() {
	rn.startTimer()

	if rn.unvotingServer {
		rn.handleUnvotingNode()
	}

	go rn.handleTimer()
	go rn.askForVotes()
	go rn.handleVotes()

	go rn.handleSnapshot()

	for {
		select {
		case <-rn.shutdownCh:
			rn.mutex.Lock()
			if rn.state == Leader {
				rn.revertToFollower()
			}
			rn.shutdownAskForVotesCh <- struct{}{}
			rn.shutdownHandleVotesCh <- struct{}{}
			rn.shutdownTimers <- struct{}{}
			rn.closeChannels()
			rn.mutex.Unlock()
			log.Println("Node", rn.id, "shutdown")
			return
		}
	}
}
