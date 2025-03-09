package raft

import (
	"encoding/json"
	"log"
	"net/rpc"
)

type newConfiguration struct {
	NewC map[ServerID]Address `json:"NewConfig"`
}

func (rn *RaftNode) prepareCold_new(command []byte) []byte {
	newConfiguration := newConfiguration{}
	err := json.Unmarshal(command, &newConfiguration)
	if err != nil {
		log.Println("Error unmarshalling new configuration: restore the old configuration")
		oldConfig, _ := json.Marshal(rn.peers.NewConfig)
		return oldConfig
	}

	// construct Cold,new and store it in the state
	rn.peers = Configuration{
		OldConfig: rn.peers.NewConfig,    // the current configuration becomes the old configuration
		NewConfig: newConfiguration.NewC, // the new configuration is the one passed in the command
	}

	// update peers connection
	for peer, address := range rn.peers.NewConfig {
		_, okInOldC := rn.peers.OldConfig[peer]
		_, okInNewC := rn.peers.NewConfig[peer]

		if okInNewC && !okInOldC && peer != rn.id {
			_, okInUnv := rn.unvotingServers[peer]
			if !okInUnv {
				// (otherwise we already have a connection to this node in the unvoting servers)
				// add the connection
				client, err := rpc.DialHTTP("tcp", string(address))
				if err != nil {
					log.Printf("Failed to dial %s: %v", peer, err)
				} else {
					log.Printf("Node %s connected to %s", rn.id, peer)
				}
				rn.peersConnection[peer] = client

				// update the nextIndex
				rn.nextIndex[peer] = rn.lastGlobalIndex() + 1
			}
		}
	}

	Cold_new, _ := json.Marshal(rn.peers)

	return Cold_new
}

func (rn *RaftNode) prepareCnew() {
	newConfiguration := Configuration{
		OldConfig: nil,
		NewConfig: rn.peers.NewConfig,
	}

	// notify other nodes by appending Cnew to the log
	index := rn.lastGlobalIndex() + 1
	command, _ := json.Marshal(newConfiguration)
	rn.USN++

	logEntry := LogEntry{
		Index:   index,
		Term:    rn.term,
		Command: command,
		Type:    ConfigurationEntry,
		Client:  string(rn.id),
		USN:     rn.USN,
	}

	rn.log.entries = append(rn.log.entries, logEntry)

	_, ok := rn.peers.NewConfig[rn.id]
	replicationCounter := 0
	if ok {
		replicationCounter = 1
	}
	rn.pendingCommit[index] = replicationState{
		replicationCounterOldC: 1, // leader already replicated
		replicationCounterNewC: uint(replicationCounter),
		committedOldC:          false,
		committedNewC:          false,
		clientCh:               nil, // no client to notify
	}

	rn.USN++
	rn.logEntriesCh <- struct{}{} // trigger log replication
}

type commandConfiguration struct {
	OldC map[ServerID]Address `json:"OldConfig"`
	NewC map[ServerID]Address `json:"NewConfig"`
}

// applyConfiguration applies the configuration change to the state, even if it is not committed
func (rn *RaftNode) applyConfiguration(command []byte) {
	newConfiguration := commandConfiguration{}
	err := json.Unmarshal(command, &newConfiguration)
	if err != nil {
		log.Println("Error unmarshalling new configuration: restore the old configuration")
		rn.peers.NewConfig = rn.peers.OldConfig
		return
	}

	rn.peers = Configuration{
		OldConfig: newConfiguration.OldC,
		NewConfig: newConfiguration.NewC,
	}

	// if we are an unvoting node and we are in the new configuration, we need to become a voting node
	_, ok := newConfiguration.NewC[rn.id]
	if ok && rn.unvotingServer {
		rn.unvotingServer = false
		log.Println("Node", rn.id, "becomes a voting node")
	}

	// update peers connection
	if newConfiguration.OldC != nil { // it is a Cold,new configuration
		for peer, address := range rn.peers.NewConfig {
			_, okInOldC := newConfiguration.OldC[peer]
			_, okInNewC := newConfiguration.NewC[peer]

			if okInNewC && !okInOldC && peer != rn.id {
				_, okInUnv := rn.unvotingServers[peer]
				if !okInUnv {
					// (otherwise we already have a connection to this node in the unvoting servers)
					// add the connection
					client, err := rpc.DialHTTP("tcp", "localhost"+string(address))
					if err != nil {
						log.Printf("Failed to dial %s: %v", peer, err)
					} else {
						log.Printf("Node %s connected to %s", rn.id, peer)
					}
					rn.peersConnection[peer] = client
				}
			}
		}
	}
}

// applyCommitedConfiguration applies the configuration change to the state, only if it is committed
func (rn *RaftNode) applyCommitedConfiguration(command []byte) {
	newConfiguration := commandConfiguration{}
	err := json.Unmarshal(command, &newConfiguration)
	if err != nil {
		log.Println("Error unmarshalling the commited configutaiton")
		return
	}

	_, ok := newConfiguration.NewC[rn.id]
	if !ok && newConfiguration.OldC == nil && !rn.unvotingServer { // it is a Cnew configuration
		// we need to shut down the node
		rn.ShutdownCh <- struct{}{}
		return
	}

	// update peers connection
	if newConfiguration.OldC == nil { // it is a Cnew configuration
		for peer, connection := range rn.peersConnection {
			_, okInNewC := newConfiguration.NewC[peer]
			_, okInUnv := rn.unvotingServers[peer]

			if !okInNewC && !okInUnv && peer != rn.id {
				// remove the connection (it is not in the new configuration and neither in the unvoting servers)
				log.Println("Closing connection to", peer)
				connection.Close()
				delete(rn.peersConnection, peer)
			}
		}

	}
}
