package raft

import (
	"log"
	"net/rpc"
)

type AddUnvotingServerArguments struct {
	ServerID ServerID
	Port     Port
}

type AddUnvotingServerResult struct {
	Success       bool
	CurrentLeader ServerID
}

func (n *Node) AddUnvotingServerRPC(req AddUnvotingServerArguments, res *AddUnvotingServerResult) error {
	n.state.mutex.Lock()
	defer n.state.mutex.Unlock()

	if n.state.state != Leader {
		res.Success = false
		res.CurrentLeader = n.state.currentLeader
		return nil
	}

	// add the connection to the unvotingServer
	client, err := rpc.Dial("tcp", "localhost"+string(req.Port))
	if err != nil {
		log.Printf("Failed to dial %s: %v", req.ServerID, err)
	} else {
		log.Printf("Node %s connected to %s", n.state.id, req.ServerID)
	}
	n.state.peersConnection[req.ServerID] = client

	// add the server to the unvoting servers
	n.state.unvotingServers[req.ServerID] = unvotingServer{
		port:          req.Port,
		acknogwledges: false,
	}

	// update its nextIndex
	n.state.nextIndex[req.ServerID] = n.state.log.lastIndex() + 1

	res.Success = true
	res.CurrentLeader = n.state.currentLeader
	return nil
}
