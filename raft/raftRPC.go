package raft

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

func (rn *RaftNode) RequestVoteRPC(req RequestVoteArguments, res *RequestVoteResult) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()

	if rn.unvotingServer {
		// discard vote request to avoid disruption from unvoting servers
		res.Term = req.Term
		res.VoteGranted = false
		return nil
	}

	// discard vote request to avoid disruption from removed servers
	select {
	case <-rn.minimumTimer.C:
	default:
		// minimumTimer is not expired
		res.Term = rn.term
		res.VoteGranted = false
		return nil
	}

	if req.Term < rn.term {
		// stale term -> reject vote
		res.Term = rn.term
		res.VoteGranted = false
		return nil
	}

	if req.Term > rn.term {
		rn.term = req.Term
		rn.revertToFollower()
	}

	// req.Term == rn.term

	// for safety check, candidate is up to date if its lastLogIndex and
	// lastLogTerm are at least as up-to-date as the node's
	if (rn.log.lastTerm() <= req.LastLogTerm) && (rn.log.lastIndex() <= req.LastLogIndex) && rn.myVote == "" {
		rn.myVote = req.CandidateId
		res.VoteGranted = true
	} else {
		res.VoteGranted = false
	}

	res.Term = rn.term
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

func (rn *RaftNode) AppendEntriesRPC(arg AppendEntriesArguments, res *AppendEntriesResult) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()

	if arg.Term < rn.term {
		res.Term = rn.term
		res.Success = false
		return nil
	}

	if arg.Term > rn.term {
		rn.revertToFollower()
		rn.currentLeader = arg.LeaderId
		// log.Println("Node", rn.id, "becomes follower of", arg.LeaderId)
		rn.term = arg.Term

		res.Term = rn.term
		res.Success = true
		return nil
	}

	// arg.Term == rn.term
	if rn.state == Candidate {
		rn.revertToFollower()
		rn.currentLeader = arg.LeaderId
		// log.Println("Node", rn.id, "becomes follower of", arg.LeaderId)
	} else if rn.currentLeader == "" {
		rn.currentLeader = arg.LeaderId
	}

	// consistency check
	exist := true
	var previousEntry LogEntry
	if arg.PreviousLogIndex > rn.log.lastIndex() {
		// we don't have the log entry at previousLogIndex or the term doesn't match
		exist = false
	} else {
		previousEntry = rn.log.entries[arg.PreviousLogIndex]
	}

	if exist && previousEntry.Term == arg.PreviousLogTerm {
		rn.log.entries = append(rn.log.entries[:arg.PreviousLogIndex+1], arg.Entries...)
		res.Success = true

		// checks for confgiuration changes
		var lastConfigurationEntry LogEntry
		for _, entry := range arg.Entries {
			if entry.Type == 1 {
				lastConfigurationEntry = entry
			}
		}
		if lastConfigurationEntry.Command != nil {
			rn.applyConfiguration(lastConfigurationEntry.Command)
		}

		if arg.LeaderCommit > rn.log.lastCommitedIndex {
			lastCommitedIndex := arg.LeaderCommit
			if arg.LeaderCommit > rn.log.lastIndex() {
				lastCommitedIndex = rn.log.lastIndex()
			}

			var lastConfigurationEntry LogEntry
			for _, entry := range rn.log.entries[rn.log.lastCommitedIndex : lastCommitedIndex+1] {
				// update the lastUSNof for all the committed requests
				if entry.Client != "" {
					//avoid NO-OP entry
					rn.lastUSNof[entry.Client] = entry.USN
				}
				// check if the committed entry was a configuration change
				if entry.Type == 1 {
					lastConfigurationEntry = entry
				}
			}
			// apply the last committed configuration change
			if lastConfigurationEntry.Command != nil {
				rn.applyCommitedConfiguration(lastConfigurationEntry.Command)
			}

			// update lastCommitedIndex
			rn.log.lastCommitedIndex = min(arg.LeaderCommit, rn.log.lastIndex())

			// apply the committed action entries to the state machine
			for _, entry := range rn.log.entries[rn.log.lastCommitedIndex : lastCommitedIndex+1] {
				if entry.Command != nil && entry.Type == 0 {
					rn.commitCh <- entry.Command
				}
			}

			// remove all our pending commit that are less than or equal to lastCommitedIndex
			for index, replicationState := range rn.pendingCommit {
				if index <= rn.log.lastCommitedIndex {
					if replicationState.term == rn.log.entries[index].Term {
						// the entry was committed
						replicationState.clientCh <- true
					} else {
						// the entry was not committed
						replicationState.clientCh <- false
					}
					delete(rn.pendingCommit, index)
				}
			}
			// update lastUncommitedRequestof
			rn.lastUncommitedRequestof = make(map[string]int) // reset the map
			for _, entry := range rn.log.entries[rn.log.lastCommitedIndex:] {
				if entry.Client != "" {
					//avoid NO-OP entry
					rn.lastUncommitedRequestof[entry.Client] = entry.USN
				}
			}
		}
	} else {
		if arg.PreviousLogIndex <= rn.log.lastIndex() {
			rn.log.entries = rn.log.entries[:arg.PreviousLogIndex] // delete all inconsistent entries after previousLogIndex
		}
		res.Success = false
	}

	res.Term = rn.term
	rn.resetTimer()
	return nil
}
