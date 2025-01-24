package raft

import (
	"log"
)

type RequestVoteArguments struct {
	Term         uint
	CandidateId  ServerID
	LastLogIndex uint
	LastLogTerm  uint
}

type RequestVoteResult struct {
	Term        uint
	VoteGranted bool
}

func (n *Node) RequestVoteRPC(req RequestVoteArguments, res *RequestVoteResult) error {
	n.state.mutex.Lock()
	defer n.state.mutex.Unlock()

	// discard vote request to avoid disruption from removed servers
	select {
	case <-n.state.minimumTimer.C:
	default:
		// minimumTimer is not expired
		res.Term = n.state.term
		res.VoteGranted = false
		return nil
	}

	if req.Term < n.state.term {
		// stale term -> reject vote
		res.Term = n.state.term
		res.VoteGranted = false
		return nil
	}

	if req.Term > n.state.term {
		n.state.term = req.Term
		n.state.revertToFollower()
	}

	// req.Term == n.state.term

	// for safety check, candidate is up to date if its lastLogIndex and
	// lastLogTerm are at least as up-to-date as the node's
	if (n.state.log.lastTerm() <= req.LastLogTerm) && (n.state.log.lastIndex() <= req.LastLogIndex) && n.state.myVote == "" {
		n.state.myVote = req.CandidateId
		res.VoteGranted = true
	} else {
		res.VoteGranted = false
	}

	res.Term = n.state.term
	return nil
}

type AppendEntriesArguments struct {
	Term             uint
	LeaderId         ServerID
	PreviousLogIndex uint
	PreviousLogTerm  uint
	Entries          []LogEntry
	LeaderCommit     uint
}

type AppendEntriesResult struct {
	Term    uint
	Success bool
}

func (n *Node) AppendEntriesRPC(arg AppendEntriesArguments, res *AppendEntriesResult) error {
	n.state.mutex.Lock()
	defer n.state.mutex.Unlock()

	if arg.Term < n.state.term {
		res.Term = n.state.term
		res.Success = false
		return nil
	}

	if arg.Term > n.state.term {
		n.state.revertToFollower()
		n.state.currentLeader = arg.LeaderId
		// log.Println("Node", n.state.id, "becomes follower of", arg.LeaderId)
		n.state.term = arg.Term

		res.Term = n.state.term
		res.Success = true
		return nil
	}

	// arg.Term == n.state.term
	if n.state.state == Candidate {
		n.state.revertToFollower()
		n.state.currentLeader = arg.LeaderId
		// log.Println("Node", n.state.id, "becomes follower of", arg.LeaderId)
	} else if n.state.currentLeader == "" {
		n.state.currentLeader = arg.LeaderId
	}

	// consistency check
	exist := true
	var previousEntry LogEntry
	if arg.PreviousLogIndex > n.state.log.lastIndex() {
		// we don't have the log entry at previousLogIndex or the term doesn't match
		exist = false
	} else {
		previousEntry = n.state.log.entries[arg.PreviousLogIndex]
	}

	if exist && previousEntry.Term == arg.PreviousLogTerm {
		n.state.log.entries = append(n.state.log.entries[:arg.PreviousLogIndex+1], arg.Entries...)
		res.Success = true

		// checks for confgiuration changes
		var lastConfigurationEntry LogEntry
		for _, entry := range arg.Entries {
			if entry.Type == 1 {
				lastConfigurationEntry = entry
			}
		}
		if lastConfigurationEntry.Command != nil {
			n.state.applyConfiguration(lastConfigurationEntry.Command)
			log.Println(n.state.id, n.state.peers)
		}

		if arg.LeaderCommit > n.state.log.lastCommitedIndex {
			// update the lastUSNof for all the committed requests
			for _, entry := range n.state.log.entries[n.state.log.lastCommitedIndex:] {
				if entry.Client != "" {
					//avoid NO-OP entry
					n.state.lastUSNof[entry.Client] = entry.USN
				}
			}

			n.state.log.lastCommitedIndex = min(arg.LeaderCommit, n.state.log.lastIndex())
			// remove all our pending commit that are less than or equal to lastCommitedIndex
			for index, replicationState := range n.state.pendingCommit {
				if index <= n.state.log.lastCommitedIndex {
					replicationState.clientCh <- true // TODO: check if the committed entry is the same as the one requested by the client, how to manage the removed pending commmit
					delete(n.state.pendingCommit, index)
				}
			}
			// update lastUncommitedRequestof
			n.state.lastUncommitedRequestof = make(map[string]int) // reset the map
			for _, entry := range n.state.log.entries[n.state.log.lastCommitedIndex:] {
				if entry.Client != "" {
					//avoid NO-OP entry
					n.state.lastUncommitedRequestof[entry.Client] = entry.USN
				}
			}
		}
	} else {
		if arg.PreviousLogIndex <= n.state.log.lastIndex() {
			n.state.log.entries = n.state.log.entries[:arg.PreviousLogIndex] // delete all inconsistent entries after previousLogIndex
		}
		res.Success = false
	}

	res.Term = n.state.term
	n.state.resetTimer()
	return nil
}
