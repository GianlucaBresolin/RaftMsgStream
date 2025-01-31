package raft

import (
	"encoding/json"
	"log"
	"net/rpc"
	"time"
)

func (rn *RaftNode) handleReplicationLog(node ServerID, peerConnection *rpc.Client) {
	// replicate log entry
	rn.mutex.Lock()

	// build info for consistency check
	previousLogIndex := rn.nextIndex[node] - 1
	previousLogTerm := rn.log.entries[previousLogIndex].Term

	// build log entries to replicate
	var logEntriesToReplicate []LogEntry
	if rn.log.lastIndex() >= rn.nextIndex[node] {
		logEntriesToReplicate = rn.log.entries[rn.nextIndex[node]:]
	}

	// build arguments for AppendEntriesRPC
	arg := AppendEntriesArguments{
		Term:             rn.term,
		LeaderId:         rn.id,
		PreviousLogIndex: previousLogIndex,
		PreviousLogTerm:  previousLogTerm,
		Entries:          logEntriesToReplicate,
		LeaderCommit:     rn.log.lastCommitedIndex,
	}

	if rn.state == Leader {
		rn.mutex.Unlock()

		failedReplicationRequest := false
		for !failedReplicationRequest {
			res := &AppendEntriesResult{}

			err := peerConnection.Call(
				"RaftNode.AppendEntriesRPC",
				&arg,
				res)

			if err != nil {
				log.Println("Error sending AppendEntriesRPC to", rn.id, ":", err)
				log.Println("Retrying...")
				continue
			}
			failedReplicationRequest = true

			rn.mutex.Lock()
			if res.Term > rn.term {
				rn.revertToFollower()
				rn.term = res.Term
			}

			if res.Success {
				previousLastCommitedIndex := rn.log.lastCommitedIndex

				for _, logEntryToReplicate := range logEntriesToReplicate {
					// the leader checks for commit just for log entries of the leader current term that are not committed
					repState, ok := rn.pendingCommit[logEntryToReplicate.Index]
					_, okInOldC := rn.peers.OldConfig[node]
					_, okInNewC := rn.peers.NewConfig[node]

					if logEntryToReplicate.Term == rn.term && rn.state == Leader && ok {
						// update the replication state
						// joint-consensus: checks also for the old configuraiton
						if !repState.committedOldC && okInOldC {
							if repState.replicationCounterOldC+1 > uint(len(rn.peers.OldConfig)/2) {
								repState.committedOldC = true
							} else {
								repState.replicationCounterOldC++
							}
						}

						// normal consensus: checks only for the new configuration
						if !repState.committedNewC && okInNewC {
							if repState.replicationCounterNewC+1 > uint(len(rn.peers.NewConfig)/2) {
								repState.committedNewC = true
							} else {
								repState.replicationCounterNewC++
							}
						}

						if repState.committedOldC && repState.committedNewC {
							rn.log.lastCommitedIndex = logEntryToReplicate.Index
							repState.clientCh <- true
							log.Println("committed log entry in position", logEntryToReplicate.Index)
							delete(rn.pendingCommit, logEntryToReplicate.Index) // remove the entry from the pending commit
						} else {
							rn.pendingCommit[logEntryToReplicate.Index] = repState // update the replication state
						}
					}
					rn.nextIndex[node] = logEntryToReplicate.Index + 1
				}

				if previousLastCommitedIndex < rn.log.lastCommitedIndex {
					for _, entry := range rn.log.entries[previousLastCommitedIndex : rn.log.lastCommitedIndex+1] {
						// if we update the lastCommitedIndex, we have to apply to the state all the committed action entries
						if entry.Type == ActionEntry && entry.Command != nil {
							rn.commitCh <- entry.Command
						}

						// if we update the lastCommitedIndex, we have also to check if we committed a configuration change
						if entry.Type == ConfigurationEntry {
							// if we committed Cold,new prepare Cnew
							configuration := commandConfiguration{}
							err := json.Unmarshal(entry.Command, &configuration)
							if err != nil {
								log.Println("Error unmarshalling the configuration")
							}
							if configuration.OldC != nil { // we have a Cold,new
								rn.prepareCnew()
							} else {
								// we committed Cnew, we have to update the configuration (we have to wait in order to let
								//the other nodes to know that this configuration is commited)
								time.AfterFunc(1*time.Second, func() {
									rn.mutex.Lock()
									rn.peers = Configuration{
										OldConfig: nil,
										NewConfig: configuration.NewC,
									}
									rn.applyCommitedConfiguration(entry.Command)
									rn.mutex.Unlock()
								})
							}
						}

						// if we update the lastCommitedIndex, we have also to update rn.lastUSNof and rn.pendingRequestof
						if entry.Client != "" && entry.USN > rn.lastUSNof[entry.Client] {
							rn.lastUSNof[entry.Client] = entry.USN

							if rn.lastUncommitedRequestof[entry.Client] == entry.USN {
								// remove last request only if it is the same USN
								delete(rn.lastUncommitedRequestof, entry.Client)
							}
						}
					}
				}

				rn.mutex.Unlock()
			} else {
				// inconsistent log entry in the follower
				log.Println("inconsistency founded")
				rn.nextIndex[node]--
				rn.mutex.Unlock()
				rn.handleReplicationLog(node, peerConnection)
			}
		}
	}
}

func (rn *RaftNode) handleLeadership() {
	ticker := time.NewTicker(LeaderTimeout * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case stopLeadership := <-rn.leaderCh:
			if stopLeadership {
				log.Println("Node", rn.id, "lost leadership")
				return
			}
		case <-rn.logEntriesCh:
			rn.mutex.Lock()
			for node, peerConnection := range rn.peersConnection {
				go rn.handleReplicationLog(node, peerConnection)
			}
			rn.mutex.Unlock()
		case <-rn.firstHeartbeatCh:
			rn.mutex.Lock()
			// the first heartbeat is sent immediately
			for node, peerConnection := range rn.peersConnection {
				go rn.handleReplicationLog(node, peerConnection)
			}
			rn.mutex.Unlock()
		case <-ticker.C:
			// heartbeat
			rn.mutex.Lock()
			for node, peerConnection := range rn.peersConnection {
				go rn.handleReplicationLog(node, peerConnection)
			}
			rn.mutex.Unlock()
		}
	}
}
