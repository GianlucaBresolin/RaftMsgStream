package raft

import "log"

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

	if req.Term > n.state.term {
		n.state.term = req.Term
		n.state.revertToFollower()
		// for safety check, later term wins
		n.state.myVote = req.CandidateId
		res.Term = n.state.term
		res.VoteGranted = true
		return nil
	}

	if req.Term < n.state.term {
		res.Term = n.state.term
		res.VoteGranted = false
		return nil
	}

	// req.Term == n.state.term

	// for safety check, candidate is up to date if its lastLogIndex and
	// lastLogTerm are at least as up-to-date as the node's
	if (n.state.log.lastIndex() <= req.LastLogIndex) && n.state.myVote == "" {
		res.Term = n.state.term
		res.VoteGranted = true
		n.state.myVote = req.CandidateId
		return nil
	}

	res.Term = n.state.term
	res.VoteGranted = false
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
		log.Println("Node", n.state.id, "becomes follower of", arg.LeaderId)
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
		n.state.log.entries = append(n.state.log.entries, arg.Entries...)
		res.Success = true
		if len(arg.Entries) > 0 {
			log.Println("node log", n.state.id, ":", n.state.log.entries)
		}
		if arg.LeaderCommit > n.state.log.lastCommitedIndex {
			n.state.log.lastCommitedIndex = min(arg.LeaderCommit, n.state.log.lastIndex())
			// remove all our pending commit that are less than or equal to leaderCommit
			for index, replicationState := range n.state.pendingCommit {
				if index <= arg.LeaderCommit {
					replicationState.clientCh <- true // TODO: check if the committed entry is the same as the one requested by the client
					delete(n.state.pendingCommit, index)
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
