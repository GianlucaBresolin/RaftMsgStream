package raft

import (
	"log"
	"time"
)

const MinElectionTimeout = 150
const MaxElectionTimeout = 300

const CandidateTimeout = 10

func (ns *nodeState) startElection() {
	ns.state = Candidate
	ns.electionVotes = 0
	ns.term++
	ns.myVote = ns.id
	ns.voteResponseCh <- RequestVoteResult{ns.term, true}
	ns.voteRequestCh <- RequestVoteArguments{ns.term, ns.id, ns.log.lastIndex(), ns.log.lastTerm()}
	log.Println("Starting election for term", ns.term)
}

func (ns *nodeState) winElection() {
	if !ns.electionTimer.Stop() {
		select {
		case <-ns.electionTimer.C: //try to drain from the channel
		default:
		}
	}
	ns.state = Leader
	ns.currentLeader = ns.id
	ns.nextIndex = make(map[ServerID]uint)
	for peer := range ns.peers {
		ns.nextIndex[peer] = ns.log.lastIndex() + 1
	}
	go ns.handleLeadership()
	log.Println("Node", ns.id, "won the election for term", ns.term)
}

func (ns *nodeState) revertToFollower() {
	if ns.state == Leader {
		ns.leaderCh <- true //stop handling leadership
		ns.nextIndex = nil
	}
	ns.state = Follower
	ns.currentLeader = ""
	ns.myVote = ""
	ns.resetTimer()
}

func (ns *nodeState) askForVotes() {
	for {
		select {
		case requestVoteArguments := <-ns.voteRequestCh:
			//we need to ask for votes
			for _, peerConnection := range ns.peersConnection {
				// log.Println("Asking for votes", ns.id, "for term", ns.term)
				go func() {
					voteResponse := &RequestVoteResult{}
					stopAskingVote := false
					for !stopAskingVote {
						err := peerConnection.Call(
							"Node.RequestVoteRPC",
							requestVoteArguments,
							voteResponse)

						if err != nil {
							log.Println("Error sending RequestVoteRPC to", ns.id, ":", err)
						} else {
							ns.voteResponseCh <- *voteResponse
							stopAskingVote = true
						}

						ns.mutex.Lock()
						if (ns.term > requestVoteArguments.Term || ns.currentLeader != "") && ns.state == Candidate {
							//stale term or we becomes leader -> stop asking to that node for a vote
							stopAskingVote = true
						}
						ns.mutex.Unlock()

						if !stopAskingVote {
							time.Sleep(CandidateTimeout * time.Millisecond) //avoid flooding the nodes
						}
					}
				}()
			}
		}
	}
}

func (ns *nodeState) handleVotes() {
	for {
		select {
		case resp := <-ns.voteResponseCh:
			ns.mutex.Lock()
			if resp.Term > ns.term {
				ns.term = resp.Term
				ns.revertToFollower()
			}

			if resp.Term == ns.term && resp.VoteGranted {
				ns.electionVotes++
				if ns.electionVotes > int(ns.numberNodes)/2 && ns.currentLeader == "" {
					ns.winElection()
				}
			}
			ns.mutex.Unlock()
		}
	}
}
