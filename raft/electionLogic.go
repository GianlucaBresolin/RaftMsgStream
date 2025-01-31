package raft

import (
	"log"
	"time"
)

func (rn *RaftNode) startElection() {
	rn.state = Candidate
	rn.electionVotesNewC = 0
	rn.electionVotesOldC = 0
	rn.term++
	rn.myVote = rn.id
	rn.voteResponseCh <- RequestVoteResultWithServerID{rn.id, RequestVoteResult{rn.term, true}}
	rn.voteRequestCh <- RequestVoteArguments{rn.term, rn.id, rn.log.lastIndex(), rn.log.lastTerm()}
	log.Println("Starting election for term", rn.term)
}

func (rn *RaftNode) winElection() {
	if !rn.electionTimer.Stop() {
		select {
		case <-rn.electionTimer.C: // try to drain from the channel
		default:
		}
	}
	rn.state = Leader
	rn.currentLeader = rn.id
	rn.nextIndex = make(map[ServerID]uint)

	// initialize nextIndex for all peers (voting and unvoting nodes)
	for peer := range rn.peers.OldConfig {
		if peer != rn.id {
			rn.nextIndex[peer] = rn.log.lastIndex() + 1
		}
	}
	for peer := range rn.peers.NewConfig {
		if peer != rn.id {
			rn.nextIndex[peer] = rn.log.lastIndex() + 1
		}
	}
	for peer := range rn.unvotingServers {
		rn.nextIndex[peer] = rn.log.lastIndex() + 1
	}

	go rn.handleLeadership()
	rn.firstHeartbeatCh <- struct{}{}
	log.Println("Node", rn.id, "won the election for term", rn.term)
}

func (rn *RaftNode) revertToFollower() {
	if rn.state == Leader {
		rn.leaderCh <- true // stop handling leadership
		rn.nextIndex = nil
	}
	rn.state = Follower
	rn.currentLeader = ""
	rn.myVote = ""
	rn.resetTimer()
}

type RequestVoteResultWithServerID struct {
	serverID ServerID
	result   RequestVoteResult
}

func (rn *RaftNode) askForVotes() {
	for {
		select {
		case requestVoteArguments := <-rn.voteRequestCh:
			// we need to ask for votes
			for peer, peerConnection := range rn.peersConnection {
				//log.Println("Asking for votes", rn.id, "for term", rn.term)
				go func() {
					voteResponse := &RequestVoteResult{}
					stopAskingVote := false
					for !stopAskingVote {
						err := peerConnection.Call(
							"RaftNode.RequestVoteRPC",
							requestVoteArguments,
							voteResponse)

						if err != nil {
							log.Println("Error sending RequestVoteRPC to", rn.id, ":", err)
						} else {
							voteResponseWithServerID := RequestVoteResultWithServerID{serverID: peer, result: *voteResponse}
							rn.voteResponseCh <- voteResponseWithServerID
							stopAskingVote = true
						}

						rn.mutex.Lock()
						if (rn.term > requestVoteArguments.Term || rn.currentLeader != "") && rn.state == Candidate {
							// stale term or we becomes leader -> stop asking to that node for a vote
							stopAskingVote = true
						}
						rn.mutex.Unlock()

						if !stopAskingVote {
							time.Sleep(CandidateTimeout * time.Millisecond) // avoid flooding the nodes
						}
					}
				}()
			}
		case <-rn.shutdownAskForVotesCh:
			return
		}
	}
}

func (rn *RaftNode) handleVotes() {
	for {
		select {
		case resp := <-rn.voteResponseCh:
			rn.mutex.Lock()
			if resp.result.Term > rn.term {
				rn.term = resp.result.Term
				rn.revertToFollower()
			}

			okOldC := false
			if rn.peers.OldConfig != nil { // we are in a configuration change
				_, okOldC = rn.peers.OldConfig[resp.serverID]
			}
			_, okNewC := rn.peers.NewConfig[resp.serverID]

			if resp.result.Term == rn.term && resp.result.VoteGranted {
				if okOldC {
					rn.electionVotesOldC++
				}
				if okNewC {
					rn.electionVotesNewC++
				}

				oldMajority := 0
				if rn.peers.OldConfig != nil {
					oldMajority = int(len(rn.peers.OldConfig)/2) + 1
				}
				newMajority := int(len(rn.peers.NewConfig)/2) + 1

				if rn.electionVotesOldC >= oldMajority && rn.electionVotesNewC >= newMajority && rn.currentLeader == "" {
					rn.winElection()
				}
			}
			rn.mutex.Unlock()
		case <-rn.shutdownHandleVotesCh:
			return
		}
	}
}
