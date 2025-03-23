package raft

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
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

const snapshotThreshold = 1024

type ServerID string
type Address string

type Configuration struct {
	OldConfig map[ServerID]Address
	NewConfig map[ServerID]Address
}

type RaftNode struct {
	id              ServerID
	address         Address
	available       bool
	state           uint
	term            uint
	peers           Configuration
	peersConnection map[ServerID]*rpc.Client
	ShutdownCh      chan struct{}
	// elction logic
	electionTimer         *time.Timer
	minimumTimer          *time.Timer
	shutdownTimersCh      chan struct{}
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
	committedNooP    bool
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
	shutdownSnapshotCh chan struct{}
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
	address Address,
	server *rpc.Server,
	peers map[ServerID]Address,
	unvoting bool) *RaftNode {
	peers[id] = address // add self to the peers list
	configDummySnap := Configuration{
		OldConfig: nil,
		NewConfig: peers,
	}
	snapshotConfigurationCommand, ok := json.Marshal(configDummySnap)
	if ok != nil {
		log.Println("Error marshalling dummy snapshot configuration command")
	}
	raftNode := &RaftNode{
		id:                    id,
		address:               address,
		available:             false,
		term:                  0,
		state:                 Follower,
		peers:                 Configuration{OldConfig: nil, NewConfig: peers},
		peersConnection:       make(map[ServerID]*rpc.Client),
		ShutdownCh:            make(chan struct{}),
		shutdownTimersCh:      make(chan struct{}),
		shutdownHandleVotesCh: make(chan struct{}),
		voteResponseCh:        make(chan RequestVoteResultWithServerID, len(peers)),
		shutdownAskForVotesCh: make(chan struct{}),
		voteRequestCh:         make(chan RequestVoteArguments, 1),
		currentLeader:         "",
		leaderCh:              make(chan bool),
		firstHeartbeatCh:      make(chan struct{}),
		nextIndex:             nil,
		committedNooP:         false,
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
		shutdownSnapshotCh: make(chan struct{}),
		SnapshotRequestCh:  make(chan struct{}),
		SnapshotResponseCh: make(chan []byte),
		snapshot: &Snapshot{
			LastIndex:        0,
			LastTerm:         0,
			LastConfig:       snapshotConfigurationCommand,
			StateMachineSnap: nil,
		},
		unvotingServer:  unvoting,
		unvotingServers: make(map[ServerID]string),
	}

	err := server.Register(raftNode)
	if err != nil {
		log.Fatalf("Failed to register node %s: %v", id, err)
	}

	mux := http.NewServeMux()
	mux.Handle(rpc.DefaultRPCPath, server)

	listener, err := net.Listen("tcp", string(raftNode.address))
	if err != nil {
		log.Fatalf("Failed to listen on address %s: %v", string(raftNode.address), err)
	}
	log.Printf("Node %s is listening on %s\n", string(raftNode.id), string(address))

	go http.Serve(listener, mux)
	return raftNode
}

func (rn *RaftNode) closeChannels() {
	close(rn.ShutdownCh)
	close(rn.shutdownTimersCh)
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
	close(rn.shutdownSnapshotCh)
}

func (rn *RaftNode) handleUnvotingNode() bool {
	rn.connectAsUnvotingNode()

	for rn.unvotingServer {
		select {
		case <-rn.ShutdownCh:
			rn.mutex.Lock()
			rn.closeConnections()
			rn.disconnectAsUnvotingNode()
			rn.closeChannels()
			rn.mutex.Unlock()
			return false
		default:
			// do nothing
		}
	}
	return true
}

func (rn *RaftNode) HandleRaftNode() {
	rn.startTimer()
	rn.available = true

	if rn.unvotingServer {
		joinTheCluster := rn.handleUnvotingNode()
		if !joinTheCluster {
			// the unvoting node is shutting down
			return
		}
		// the unvoting node has joined the cluster
	}

	go rn.handleTimer()
	go rn.askForVotes()
	go rn.handleVotes()

	go rn.handleSnapshot()

	// wait for shutdown
	<-rn.ShutdownCh
	rn.mutex.Lock()
	if rn.state == Leader {
		rn.revertToFollower()
	}
	rn.shutdownAskForVotesCh <- struct{}{}
	rn.shutdownHandleVotesCh <- struct{}{}
	rn.shutdownTimersCh <- struct{}{}
	rn.shutdownSnapshotCh <- struct{}{}
	rn.closeChannels()
	rn.closeConnections()
	rn.mutex.Unlock()
	log.Println("Node", rn.id, "shutdown")
}
