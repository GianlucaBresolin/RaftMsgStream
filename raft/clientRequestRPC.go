package raft

import "log"

type ClientRequestArguments struct {
	Command string
}

type ClientRequestResult struct {
	Success bool
	Leader  ServerID
}

func (n *Node) ClientRequestRPC(req ClientRequestArguments, res *ClientRequestResult) error {
	n.state.mutex.Lock()
	defer n.state.mutex.Unlock()

	if n.state.state == Leader {
		log.Println("correctly appended the request")
		res.Success = true
		res.Leader = n.state.id
		return nil
	}

	//redirect to leader
	res.Success = false
	res.Leader = n.state.currentLeader
	return nil
}
