package raft

import (
	"log"
	"time"
)

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
		case <-ns.electionTimer.C: // try to drain from the channel
		default:
		}
	}
	ns.state = Leader
	ns.currentLeader = ns.id
	ns.nextIndex = make(map[ServerID]uint)

	// initialize nextIndex for all peers
	for peer := range ns.peers.OldConfig {
		if peer != ns.id {
			ns.nextIndex[peer] = ns.log.lastIndex() + 1
		}
	}
	for peer := range ns.peers.NewConfig {
		if peer != ns.id {
			ns.nextIndex[peer] = ns.log.lastIndex() + 1
		}
	}

	go ns.handleLeadership()
	ns.firstHeartbeatCh <- struct{}{}
	log.Println("Node", ns.id, "won the election for term", ns.term)
}

func (ns *nodeState) revertToFollower() {
	if ns.state == Leader {
		ns.leaderCh <- true // stop handling leadership
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
			// we need to ask for votes
			for _, peerConnection := range ns.peersConnection {
				//log.Println("Asking for votes", ns.id, "for term", ns.term)
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
							// stale term or we becomes leader -> stop asking to that node for a vote
							stopAskingVote = true
						}
						ns.mutex.Unlock()

						if !stopAskingVote {
							time.Sleep(CandidateTimeout * time.Millisecond) // avoid flooding the nodes
						}
					}
				}()
			}
		case <-ns.shutdownAskForVotesCh:
			return
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

				oldMajority := 0
				if ns.peers.OldConfig != nil {
					oldMajority = int(len(ns.peers.OldConfig)/2) + 1
				}
				newMajority := int(len(ns.peers.NewConfig)/2) + 1

				if ns.electionVotes >= oldMajority && ns.electionVotes >= newMajority && ns.currentLeader == "" {
					ns.winElection()
				}
			}
			ns.mutex.Unlock()
		case <-ns.shutdownHandleVotesCh:
			return
		}
	}
}
