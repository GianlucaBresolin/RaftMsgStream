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
		//TODO: logic of safety check
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
	if n.state.myVote == "" {
		//TODO: logic of safety check
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
	}

	// log.Println("Node", n.state.id, "received heartbeat from", arg.LeaderId)
	n.state.resetTimer()
	res.Term = n.state.term
	res.Success = true
	return nil
}
