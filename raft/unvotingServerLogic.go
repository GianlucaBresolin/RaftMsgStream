package raft

import (
	"log"
	"net/rpc"
	"time"
)

type UnvotingServerArguments struct {
	ServerID ServerID
	Address  Address
}

type UnvotingServerResult struct {
	Success       bool
	CurrentLeader ServerID
}

// RPC offered by a node to add a server as unvoting server
func (rn *RaftNode) AddUnvotingServerRPC(req UnvotingServerArguments, res *UnvotingServerResult) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()

	if rn.state != Leader {
		res.Success = false
		res.CurrentLeader = rn.currentLeader
		return nil
	}

	// add the connection to the unvotingServer
	client, err := rpc.DialHTTP("tcp", string(req.Address))
	if err != nil {
		log.Printf("Failed to dial %s: %v", req.ServerID, err)
	} else {
		log.Printf("Node %s connected to the unvoting server %s", rn.id, req.ServerID)
	}
	rn.peersConnection[req.ServerID] = client

	// add the server to the unvoting servers
	rn.unvotingServers[req.ServerID] = string(req.Address)

	// update its nextIndex
	rn.nextIndex[req.ServerID] = rn.lastGlobalIndex() + 1

	res.Success = true
	res.CurrentLeader = rn.currentLeader
	return nil
}

// connectAsUnvotingServer connects the node as unvoting server
func (rn *RaftNode) connectAsUnvotingNode() {
	// request to be added as unvoting server
	args := UnvotingServerArguments{
		ServerID: rn.id,
		Address:  rn.address,
	}
	reply := UnvotingServerResult{
		Success:       false,
		CurrentLeader: "",
	}

	var leaderNode ServerID
	for serverID := range rn.peers.NewConfig {
		if serverID != rn.id {
			leaderNode = serverID // not sure it is the leader, but we try with a random one to get otherwise the real leader
			break
		}
	}

	for !reply.Success {
		done := make(chan *rpc.Call, 1)
		timeout := time.NewTimer(20 * time.Millisecond)

		rn.peersConnection[leaderNode].Go("RaftNode.AddUnvotingServerRPC", args, &reply, done)

		select {
		case call := <-done:
			if call.Error != nil {
				log.Printf("Failed to add unvoting server %s: %v", rn.id, call.Error)
				return
			}
			if reply.CurrentLeader != "" {
				leaderNode = reply.CurrentLeader // update the leader
			}
		case <-timeout.C:
			log.Println("The node doesn't responded in time")
			// try with another leader
			for serverID := range rn.peers.NewConfig {
				if serverID != rn.id && serverID != leaderNode {
					leaderNode = serverID
					break
				}
			}
		}
		close(done)
	}
}

// RPC offered by a node to remove a server as unvoting server
func (rn *RaftNode) RemoveUnvotingServerRPC(req UnvotingServerArguments, res *UnvotingServerResult) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()

	if rn.state != Leader {
		res.Success = false
		res.CurrentLeader = rn.currentLeader
		return nil
	}
	// check if the server is in the unvoting servers
	if _, ok := rn.unvotingServers[req.ServerID]; !ok {
		res.Success = true
		res.CurrentLeader = rn.currentLeader
		return nil
	}

	// remove the connection to the unvotingServer
	if connection, ok := rn.peersConnection[req.ServerID]; ok {
		connection.Close()
		delete(rn.peersConnection, req.ServerID)
	}

	// remove the server from the unvoting servers
	delete(rn.unvotingServers, req.ServerID)

	// update its nextIndex
	delete(rn.nextIndex, req.ServerID)

	res.Success = true
	res.CurrentLeader = rn.currentLeader
	return nil
}

// disconnectAsUnvotingServer disconnects the node as unvoting server
func (rn *RaftNode) disconnectAsUnvotingNode() {
	// request to be removed as unvoting server
	args := UnvotingServerArguments{
		ServerID: rn.id,
		Address:  rn.address,
	}
	reply := UnvotingServerResult{
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
		done := make(chan *rpc.Call, 1)
		timeout := time.NewTimer(20 * time.Millisecond)

		rn.peersConnection[leaderNode].Go("RaftNode.RemoveUnvotingServerRPC", args, &reply, done)

		select {
		case call := <-done:
			if call.Error != nil {
				log.Printf("Failed to remove unvoting server %s: %v", rn.id, call.Error)
				return
			}
			leaderNode = reply.CurrentLeader
		case <-timeout.C:
			log.Println("The node doesn't responded in time")
			// try with another leader
			for serverID := range rn.peers.NewConfig {
				if serverID != rn.id && serverID != leaderNode {
					leaderNode = serverID
					break
				}
			}
		}
		close(done)
	}
	log.Println("Node", rn.id, "disconnected as unvoting server")
}
