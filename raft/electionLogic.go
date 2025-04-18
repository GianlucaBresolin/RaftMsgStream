package raft

import (
	"encoding/json"
	"log"
	"net/rpc"
	"time"
)

func (rn *RaftNode) startElection() {
	rn.state = Candidate
	rn.electionVotesNewC = 0
	rn.electionVotesOldC = 0
	rn.term++
	rn.voteRequestCh <- RequestVoteArguments{rn.term, rn.id, rn.lastGlobalIndex(), rn.log.lastTerm()}
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
			rn.nextIndex[peer] = rn.lastGlobalIndex() + 1
		}
	}
	for peer := range rn.peers.NewConfig {
		if peer != rn.id {
			rn.nextIndex[peer] = rn.lastGlobalIndex() + 1
		}
	}
	for peer := range rn.unvotingServers {
		rn.nextIndex[peer] = rn.lastGlobalIndex() + 1
	}

	// checks if we need to finish the configuration change
	if rn.peers.OldConfig != nil {
		// if we have in the uncommitted entries Cnew the configuration change will be correctly committed
		cNew := false
		for _, entry := range rn.log.entries[rn.log.lastCommitedIndex+1:] {
			if entry.Type == ConfigurationEntry {
				// we have Cnew in the uncommitted entries
				cNew = true
				break
			}
		}

		if !cNew {
			// we need to create Cnew and commit it
			newConfiguration := Configuration{
				OldConfig: nil,
				NewConfig: rn.peers.NewConfig,
			}

			// notify other nodes by appending Cnew to the log
			index := rn.lastGlobalIndex() + 1
			command, _ := json.Marshal(newConfiguration)
			rn.USN++

			logEntry := LogEntry{
				Index:   index,
				Term:    rn.term,
				Command: command,
				Type:    ConfigurationEntry,
				Client:  string(rn.id),
				USN:     rn.USN,
			}

			rn.log.entries = append(rn.log.entries, logEntry)

			_, ok := rn.peers.NewConfig[rn.id]
			replicationCounter := 0
			if ok {
				replicationCounter = 1
			}
			rn.pendingCommit[index] = replicationState{
				replicationCounterOldC: 1, // leader already replicated
				replicationCounterNewC: uint(replicationCounter),
				committedOldC:          false,
				committedNewC:          false,
				clientCh:               nil, // no client to notify
			}
		}
	}

	// append a NOOP entry to the log to provide the most up-to-date state
	logEntry := LogEntry{
		Index:   rn.lastGlobalIndex() + 1,
		Term:    rn.term,
		Command: nil,
		Type:    NOOPEntry,
		Client:  string(rn.id),
		USN:     rn.USN,
	}
	rn.log.entries = append(rn.log.entries, logEntry)
	nodeCh := make(chan bool)

	commitedOldC := false
	if rn.peers.OldConfig == nil {
		commitedOldC = true
	}

	_, ok := rn.peers.NewConfig[rn.id]
	replicationCounterNewC := 0
	if ok {
		replicationCounterNewC = 1 // leader already replicated
	}

	rn.pendingCommit[logEntry.Index] = replicationState{
		replicationCounterOldC: 1, // leader already replicated
		replicationCounterNewC: uint(replicationCounterNewC),
		committedOldC:          commitedOldC,
		committedNewC:          false,
		term:                   rn.term,
		clientCh:               nodeCh,
	}

	go func() {
		committed := <-nodeCh
		rn.mutex.Lock()
		close(nodeCh)
		if !committed {
			log.Println("Error committing NOOP entry")
			rn.revertToFollower()
		} else {
			rn.committedNooP = true
		}
		rn.mutex.Unlock()
	}()

	go rn.handleLeadership()
	rn.firstHeartbeatCh <- struct{}{}
	log.Println("Node", rn.id, "won the election for term", rn.term)
}

func (rn *RaftNode) revertToFollower() {
	if rn.state == Leader {
		rn.leaderCh <- true // stop handling leadership
		rn.nextIndex = nil
		rn.committedNooP = false
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
				go func() {
					voteResponse := &RequestVoteResult{}
					stopAskingVote := false
					for !stopAskingVote {
						done := make(chan *rpc.Call, 1)
						timeout := time.NewTimer(20 * time.Millisecond)

						log.Println("Node ", rn.id, " is asking for votes to node ", peer, " for term ", requestVoteArguments.Term)

						peerConnection.Go(
							"RaftNode.RequestVoteRPC",
							requestVoteArguments,
							voteResponse,
							done)

						select {
						case call := <-done:
							if call.Error != nil {
								log.Println("Error sending RequestVoteRPC to", peer, ":", call.Error)
							} else {
								voteResponseWithServerID := RequestVoteResultWithServerID{serverID: peer, result: *voteResponse}
								rn.voteResponseCh <- voteResponseWithServerID
								stopAskingVote = true
							}

							rn.mutex.Lock()
							if rn.term > requestVoteArguments.Term || rn.currentLeader != "" {
								// stale term or we becomes leader -> stop asking to that node for a vote
								stopAskingVote = true
							}
							rn.mutex.Unlock()

							if !stopAskingVote {
								time.Sleep(CandidateTimeout * time.Millisecond) // avoid flooding the nodes
							}
						case <-timeout.C:
							log.Println("The node", peer, "doesn't responded in time to the RequestVoteRPC made by node", rn.id)
							// retry
						}
						close(done)
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
