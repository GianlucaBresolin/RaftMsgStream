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

// RPC offered by a node to add a server as unvoting server
func (rn *RaftNode) AddUnvotingServerRPC(req AddUnvotingServerArguments, res *AddUnvotingServerResult) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()

	if rn.state != Leader {
		res.Success = false
		res.CurrentLeader = rn.currentLeader
		return nil
	}

	// add the connection to the unvotingServer
	client, err := rpc.Dial("tcp", "localhost"+string(req.Port))
	if err != nil {
		log.Printf("Failed to dial %s: %v", req.ServerID, err)
	} else {
		log.Printf("Node %s connected to %s", rn.id, req.ServerID)
	}
	rn.peersConnection[req.ServerID] = client

	// add the server to the unvoting servers
	rn.unvotingServers[req.ServerID] = unvotingServer{
		port:          req.Port,
		acknogwledges: false,
	}

	// update its nextIndex
	rn.nextIndex[req.ServerID] = rn.log.lastIndex() + 1

	res.Success = true
	res.CurrentLeader = rn.currentLeader
	return nil
}

// connectAsUnvotingServer connects the node as unvoting server
func (rn *RaftNode) connectAsUnvotingNode() {
	// request to be added as unvoting server
	args := AddUnvotingServerArguments{
		ServerID: rn.id,
		Port:     rn.port,
	}
	reply := AddUnvotingServerResult{
		Success: false,
	}

	var leaderNode ServerID
	for serverID := range rn.peers.NewConfig {
		if serverID != rn.id {
			leaderNode = serverID // not sure it is the leader, but we try with a random one to get otherwise the real leader
			break
		}
	}

	for !reply.Success {
		err := rn.peersConnection[leaderNode].Call("Node.AddUnvotingServerRPC", args, &reply)
		if err != nil {
			log.Printf("Failed to add unvoting server %s: %v", rn.id, err)
			return
		}
		leaderNode = reply.CurrentLeader
	}
}
