package raft

import (
	"encoding/json"
	"log"
	"net/rpc"
	"time"
)

type newConfiguration struct {
	NewC map[ServerID]Port `json:"NewConfig"`
}

func (ns *nodeState) prepareCold_new(command []byte) []byte {
	newConfiguration := newConfiguration{}
	err := json.Unmarshal(command, &newConfiguration)
	if err != nil {
		log.Println("Error unmarshalling new configuration: restore the old configuration")
		oldConfig, _ := json.Marshal(ns.peers.NewConfig)
		return oldConfig
	}

	// construct Cold,new and store it in the state
	ns.peers = Configuration{
		OldConfig: ns.peers.NewConfig,    // the current configuration becomes the old configuration
		NewConfig: newConfiguration.NewC, // the new configuration is the one passed in the command
	}

	// update peers connection
	for peer, port := range ns.peers.NewConfig {
		_, okInOldC := ns.peers.OldConfig[peer]
		_, okInNewC := ns.peers.NewConfig[peer]

		if okInNewC && !okInOldC && peer != ns.id {
			// add the connection
			client, err := rpc.Dial("tcp", "localhost"+string(port))
			if err != nil {
				log.Printf("Failed to dial %s: %v", peer, err)
			} else {
				log.Printf("Node %s connected to %s", ns.id, peer)
			}
			ns.peersConnection[peer] = client

			// update the nextIndex
			ns.nextIndex[peer] = ns.log.lastIndex() + 1
		}
	}

	Cold_new, _ := json.Marshal(ns.peers)

	return Cold_new
}

func (ns *nodeState) prepareCnew() {
	ns.mutex.Lock()

	newConfiguration := Configuration{
		OldConfig: nil,
		NewConfig: ns.peers.NewConfig,
	}

	// notify other nodes by appending Cnew to the log
	index := ns.log.lastIndex() + 1
	command, _ := json.Marshal(newConfiguration)
	ns.USN++

	logEntry := LogEntry{
		Index:   index,
		Term:    ns.term,
		Command: command,
		Type:    ConfigurationEntry,
		Client:  string(ns.id),
		USN:     ns.USN,
	}

	ns.log.entries = append(ns.log.entries, logEntry)
	clientCh := make(chan bool)

	_, ok := ns.peers.NewConfig[ns.id]
	replicationCounter := 0
	if ok {
		replicationCounter = 1
	}
	ns.pendingCommit[index] = replicationState{
		replicationCounterOldC: 1, // leader already replicated
		replicationCounterNewC: uint(replicationCounter),
		committedOldC:          false,
		committedNewC:          false,
		clientCh:               clientCh,
	}

	ns.lastUncommitedRequestof[string(ns.id)] = ns.USN

	ns.logEntriesCh <- struct{}{} // trigger log replication
	ns.mutex.Unlock()

	<-clientCh
	// update our configuration
	ns.mutex.Lock()
	ns.peers = newConfiguration
	ns.mutex.Unlock()
	time.Sleep(time.Second) // wait to let the other nodes to know that this configuration is commited
	ns.mutex.Lock()
	ns.applyCommitedConfiguration(command)
	ns.mutex.Unlock()
}

type commandConfiguration struct {
	OldC map[ServerID]Port `json:"OldConfig"`
	NewC map[ServerID]Port `json:"NewConfig"`
}

func (ns *nodeState) applyConfiguration(command []byte) {
	newConfiguration := commandConfiguration{}
	err := json.Unmarshal(command, &newConfiguration)
	if err != nil {
		log.Println("Error unmarshalling new configuration: restore the old configuration")
		ns.peers.NewConfig = ns.peers.OldConfig
		return
	}

	ns.peers = Configuration{
		OldConfig: newConfiguration.OldC,
		NewConfig: newConfiguration.NewC,
	}

	// update peers connection
	if newConfiguration.OldC != nil { // it is a Cold,new configuration
		for peer, port := range ns.peers.NewConfig {
			_, okInOldC := newConfiguration.OldC[peer]
			_, okInNewC := newConfiguration.NewC[peer]

			if okInNewC && !okInOldC && peer != ns.id {
				// add the connection
				client, err := rpc.Dial("tcp", "localhost"+string(port))
				if err != nil {
					log.Printf("Failed to dial %s: %v", peer, err)
				} else {
					log.Printf("Node %s connected to %s", ns.id, peer)
				}

				ns.peersConnection[peer] = client
			}
		}
	}
}

func (ns *nodeState) applyCommitedConfiguration(command []byte) {
	newConfiguration := commandConfiguration{}
	err := json.Unmarshal(command, &newConfiguration)
	if err != nil {
		log.Println("Error unmarshalling the commited configutaiton")
		return
	}

	_, ok := newConfiguration.NewC[ns.id]
	if !ok && newConfiguration.OldC == nil { // it is a Cnew configuration
		// we need to shut down the node
		ns.shutdownCh <- struct{}{}
		return
	}

	// update peers connection
	if newConfiguration.OldC == nil { // it is a Cnew configuration
		for peer, connection := range ns.peersConnection {
			_, okInNewC := newConfiguration.NewC[peer]

			if !okInNewC {
				// remove the connection
				log.Println("Closing connection to", peer)
				connection.Close()
				delete(ns.peersConnection, peer)
			}
		}
	}
}
