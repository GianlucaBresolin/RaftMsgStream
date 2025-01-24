package raft

import (
	"encoding/json"
	"log"
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

	//construct Cold,new and store it in the state
	ns.peers = Configuration{
		OldConfig: ns.peers.NewConfig,    // the current configuration becomes the old configuration
		NewConfig: newConfiguration.NewC, // the new configuration is the one passed in the command
	}

	Cold_new, _ := json.Marshal(ns.peers)

	return Cold_new
}

func (ns *nodeState) prepareCnew() {
	ns.mutex.Lock()

	// update our configuration
	ns.peers = Configuration{
		OldConfig: nil,
		NewConfig: ns.peers.NewConfig,
	}

	// notify other nodes by appending Cnew to the log
	index := ns.log.lastIndex() + 1
	command, _ := json.Marshal(ns.peers)
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
	ns.pendingCommit[index] = replicationState{
		replicationCounter: 1, // leader already replicated
		committed:          false,
		clientCh:           clientCh,
	}
	ns.lastUncommitedRequestof[string(ns.id)] = ns.USN

	ns.logEntriesCh <- struct{}{} // trigger log replication
	ns.mutex.Unlock()

	<-clientCh
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
}
