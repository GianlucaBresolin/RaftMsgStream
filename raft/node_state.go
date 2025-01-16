package raft

import (
	"log"
	"math/rand"
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
const CandidateTimeout = 20

type ServerID string
type Port string

type nodeState struct {
	id              ServerID
	state           int
	term            int
	numberNodes     int
	peers           map[ServerID]Port
	peersConnection map[ServerID]*rpc.Client
	electionTimer   *time.Timer
	electionVotes   int
	voteResponseCh  chan RequestVoteResult
	voteRequestCh   chan RequestVoteArguments
	myVote          ServerID
	currentLeader   ServerID
	mutex           sync.Mutex
}

func newNodeState(id ServerID, peers map[ServerID]Port) *nodeState {
	return &nodeState{
		term:            0,
		state:           Follower,
		id:              id,
		numberNodes:     int(len(peers) + 1),
		peers:           peers,
		peersConnection: make(map[ServerID]*rpc.Client),
		voteResponseCh:  make(chan RequestVoteResult, len(peers)),
		voteRequestCh:   make(chan RequestVoteArguments, 1),
		currentLeader:   "",
	}
}

func (ns *nodeState) revertToFollower() {
	ns.state = Follower
	ns.currentLeader = ""
	ns.myVote = ""
	ns.resetTimer()
}

func (ns *nodeState) startElection() {
	ns.state = Candidate
	ns.electionVotes = 0
	ns.term++
	ns.myVote = ns.id
	ns.voteResponseCh <- RequestVoteResult{ns.term, true}
	ns.voteRequestCh <- RequestVoteArguments{ns.term, ns.id, 0, 0}
	log.Println("Starting election for term", ns.term)
}

func (ns *nodeState) winElection() {
	ns.state = Leader
	ns.currentLeader = ns.id
	if !ns.electionTimer.Stop() {
		select {
		case <-ns.electionTimer.C: //try to drain from the channel
		default:
		}
	}
	log.Println("Node", ns.id, "won the election for term", ns.term)
}

func (ns *nodeState) handleMyElection() {
	//send vote requests
	go func() {
		for {
			select {
			case requestVoteArguments := <-ns.voteRequestCh:
				//we need to ask for votes
				for _, peersConnection := range ns.peersConnection {
					log.Println("Asking for votes")
					go func() {
						voteResponse := &RequestVoteResult{}
						peersConnection.Call(
							"Node.RequestVoteRPC",
							requestVoteArguments,
							voteResponse)
						ns.voteResponseCh <- *voteResponse
					}()
				}

				time.Sleep(CandidateTimeout * time.Millisecond)
				ns.mutex.Lock()
				if ns.state == Candidate {
					//continue asking for votes
					ns.voteRequestCh <- RequestVoteArguments{ns.term, ns.id, 0, 0}
				}
				ns.mutex.Unlock()
			}
		}
	}()

	//collect votes
	go func() {
		for {
			select {
			case resp := <-ns.voteResponseCh:
				//we got a vote response
				ns.mutex.Lock()
				if resp.Term > ns.term {
					ns.term = resp.Term
					ns.revertToFollower()
				}

				if resp.Term == ns.term && resp.VoteGranted {
					ns.electionVotes++
					if ns.electionVotes > ns.numberNodes/2 && ns.currentLeader == "" {
						ns.winElection()
					}
				}
				ns.mutex.Unlock()
			}
		}
	}()
}

func (ns *nodeState) startTimer() {
	ns.electionTimer = time.NewTimer(time.Duration(MinElectionTimeout+rand.Intn(MaxElectionTimeout-MinElectionTimeout)) * time.Millisecond)
}

func (ns *nodeState) resetTimer() {
	if ns.electionTimer != nil && !ns.electionTimer.Stop() {
		select {
		case <-ns.electionTimer.C: //try to drain from the channel
		default:
		}
	}

	ns.electionTimer.Reset(time.Duration(MinElectionTimeout+rand.Intn(MaxElectionTimeout-MinElectionTimeout)) * time.Millisecond)
}

func (ns *nodeState) handleTimer() {
	for {
		select {
		case <-ns.electionTimer.C:
			//timer expired
			ns.mutex.Lock()
			log.Println("Election timeout expired, starting election...")
			ns.startElection()
			ns.resetTimer()
			ns.mutex.Unlock()
		}
	}
}

// func (ns *nodeState) handleAppendEntriesRequest(req AppendEntriesRequest) AppendEntriesResponse {
// 	ns.mutex.Lock()
// 	defer ns.mutex.Unlock()

// 	// stale term -> discard request
// 	if req.Term < ns.term {
// 		return AppendEntriesResponse{
// 			Term:    ns.term,
// 			Success: false,
// 		}
// 	}

// 	// new term -> update term and become follower
// 	if req.Term > ns.term {
// 		ns.term = req.Term
// 		ns.state = Follower
// 		ns.resetTimer()
// 		// TO DO
// 	}

// 	// heartbeat
// 	if req.Term == ns.term {
// 		ns.resetTimer()
// 		return AppendEntriesResponse{
// 			Term:    ns.term,
// 			Success: true,
// 		}
// 	}

// 	// TO DO
// 	return AppendEntriesResponse{
// 		Term:    ns.term,
// 		Success: true,
// 	}
// }
