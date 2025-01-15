package raft

type RequestVoteArguments struct {
	Term         int
	CandidateId  ServerID
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteResult struct {
	Term        int
	VoteGranted bool
}

func (n *Node) RequestVoteRPC(req RequestVoteArguments, res *RequestVoteResult) error {
	n.state.mutex.Lock()
	defer n.state.mutex.Unlock()

	if req.Term > n.state.term {
		n.state.term = req.Term
		n.state.revertToFollower()
		res.Term = n.state.term
		res.VoteGranted = false
		return nil
	}

	if req.Term < n.state.term {
		res.Term = n.state.term
		res.VoteGranted = false
		return nil
	}

	// req.Term == n.state.term
	if n.state.myVote == "" {
		res.Term = n.state.term
		res.VoteGranted = true
		n.state.myVote = req.CandidateId
		return nil
	}

	res.Term = n.state.term
	res.VoteGranted = false
	return nil
	//TODO: logic of safety check
}
