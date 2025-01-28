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

type ServerID string
type Port string

type Configuration struct {
	OldConfig map[ServerID]Port
	NewConfig map[ServerID]Port
}

type unvotingServer struct {
	port          Port
	acknogwledges bool
}

type nodeState struct {
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
	log                     logStruct
	logEntriesCh            chan struct{}
	pendingCommit           map[uint]replicationState
	lastUSNof               map[string]int
	lastUncommitedRequestof map[string]int
	// unvoting logic
	unvotingServer  bool
	unvotingServers map[ServerID]unvotingServer
	// mutex
	mutex sync.Mutex
}

func newNodeState(id ServerID, port Port, peers map[ServerID]Port, unvoting bool) *nodeState {
	peers[id] = "" // add self to the peers list
	return &nodeState{
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
				// to start the log from index 1 we add a default entry
				{
					Index:   0,
					Term:    0,
					Command: nil,
					Client:  "",
					USN:     -1,
				},
			},
			lastCommitedIndex: 0,
		},
		logEntriesCh:            make(chan struct{}),
		pendingCommit:           make(map[uint]replicationState),
		lastUSNof:               make(map[string]int),
		lastUncommitedRequestof: make(map[string]int),
		unvotingServer:          unvoting,
		unvotingServers:         make(map[ServerID]unvotingServer),
	}
}

func (ns *nodeState) closeChannels() {
	close(ns.shutdownCh)
	close(ns.shutdownTimers)
	close(ns.shutdownAskForVotesCh)
	close(ns.shutdownHandleVotesCh)
	close(ns.voteResponseCh)
	close(ns.voteRequestCh)
	close(ns.firstHeartbeatCh)
	close(ns.leaderCh)
}

func (ns *nodeState) handleUnvotingNode() {
	// request to be added as unvoting server
	args := AddUnvotingServerArguments{
		ServerID: ns.id,
		Port:     ns.port,
	}
	reply := AddUnvotingServerResult{
		Success: false,
	}

	var leaderNode ServerID
	for serverID := range ns.peers.NewConfig {
		if serverID != ns.id {
			leaderNode = serverID // not sure it is the leader, but we try with a random one to get otherwise the real leader
			break
		}
	}

	for !reply.Success {
		err := ns.peersConnection[leaderNode].Call("Node.AddUnvotingServerRPC", args, &reply)
		if err != nil {
			log.Printf("Failed to add unvoting server %s: %v", ns.id, err)
			return
		}
		leaderNode = reply.CurrentLeader
	}

	for ns.unvotingServer {
	}
}

func (ns *nodeState) handleNodeState() {
	ns.startTimer()

	if ns.unvotingServer {
		ns.handleUnvotingNode()
	}

	go ns.handleTimer()

	go ns.askForVotes()
	go ns.handleVotes()

	for {
		select {
		case <-ns.shutdownCh:
			ns.mutex.Lock()
			if ns.state == Leader {
				ns.revertToFollower()
			}
			ns.shutdownAskForVotesCh <- struct{}{}
			ns.shutdownHandleVotesCh <- struct{}{}
			ns.shutdownTimers <- struct{}{}
			ns.closeChannels()
			ns.mutex.Unlock()
			log.Println("Node", ns.id, "shutdown")
			return
		}
	}
}
