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
