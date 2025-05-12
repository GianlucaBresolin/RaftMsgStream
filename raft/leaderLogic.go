package raft

import (
	"encoding/json"
	"log"
	"net/rpc"
	"time"
)

func (rn *RaftNode) handleReplicationLog(node ServerID, peerConnection *rpc.Client) bool {
	// replicate log entry
	rn.mutex.Lock()
	if rn.state == Leader {
		// build info for consistency check
		previousLogIndex := rn.nextIndex[node] - 1

		if previousLogIndex < rn.snapshot.LastIndex {
			// we don't have the log entry at previousLogIndex, we have to send the snapshot
			successInstallationSnapshot := false
			var reply InstallSnapshotResult
			snapshotArguments := &InstallSnapshotArguments{
				Term:              rn.term,
				LeaderId:          rn.id,
				LastIncludedIndex: rn.snapshot.LastIndex,
				LastIncludedTerm:  rn.snapshot.LastTerm,
				LastConfig:        rn.snapshot.LastConfig,
				LastUSNof:         rn.snapshot.LastUSNof,
				Offset:            0,
				Data:              rn.snapshot.StateMachineSnap,
				Done:              true,
			}
			rn.mutex.Unlock()

			for !successInstallationSnapshot {
				doneSnap := make(chan *rpc.Call, 1)
				timeoutSnap := time.NewTimer(20 * time.Millisecond)

				peerConnection.Go("RaftNode.InstallSnapshotRPC", snapshotArguments, &reply, doneSnap)

				select {
				case call := <-doneSnap:
					if call.Error != nil {
						log.Println("Error sending InstallSnapshotRPC to", rn.id, ":", call.Error)
					}
					if reply.Success {
						successInstallationSnapshot = true
					}
				case <-timeoutSnap.C:
					log.Println("The node", node, "doesn't responded in time to the InstallSnapshotRPC made by node", rn.id, "retrying...")
				}
			}
			log.Println("Snapshot installed on", node)
			rn.mutex.Lock()
			rn.nextIndex[node] = rn.snapshot.LastIndex + 1
			previousLogIndex = rn.snapshot.LastIndex
		}
		previousLogTerm := rn.log.entries[previousLogIndex-rn.snapshot.LastIndex].Term

		// build log entries to replicate
		var logEntriesToReplicate []LogEntry
		if rn.lastGlobalIndex() >= rn.nextIndex[node] {
			logEntriesToReplicate = rn.log.entries[rn.nextIndex[node]-rn.snapshot.LastIndex:]
		}

		// build arguments for AppendEntriesRPC
		arg := AppendEntriesArguments{
			Term:             rn.term,
			LeaderId:         rn.id,
			PreviousLogIndex: previousLogIndex,
			PreviousLogTerm:  previousLogTerm,
			Entries:          logEntriesToReplicate,
			LeaderCommit:     rn.lastGlobalCommitedIndex(),
		}

		rn.mutex.Unlock()

		failedReplicationRequest := false
		res := &AppendEntriesResult{}

		for !failedReplicationRequest {
			done := make(chan *rpc.Call, 1)
			timeout := time.NewTimer(20 * time.Millisecond)

			peerConnection.Go(
				"RaftNode.AppendEntriesRPC",
				&arg,
				res,
				done)

			select {
			case call := <-done:
				if call.Error != nil {
					log.Println("Error sending AppendEntriesRPC to", node, ":", call.Error, "retrying...")
					continue
				}
				failedReplicationRequest = true

				rn.mutex.Lock()
				if res.Term > rn.term {
					rn.revertToFollower()
					rn.term = res.Term
					rn.mutex.Unlock()
					return false
				}

				if res.Success {
					previousLastCommitedIndex := rn.log.lastCommitedIndex

					triggerSnapshot := false
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
								// update the last commited index
								rn.log.lastCommitedIndex = logEntryToReplicate.Index - rn.snapshot.LastIndex
								repState.clientCh <- true
								log.Println("commited log entry in position", logEntryToReplicate.Index)
								delete(rn.pendingCommit, logEntryToReplicate.Index) // remove the entry from the pending commit

								// check if we have to take a snapshot
								if (rn.log.lastCommitedIndex) >= snapshotThreshold {
									triggerSnapshot = true
								}
							} else {
								rn.pendingCommit[logEntryToReplicate.Index] = repState // update the replication state
							}
						}
						rn.nextIndex[node] = logEntryToReplicate.Index + 1
					}
					if triggerSnapshot {
						rn.takeSnapshotCh <- struct{}{} // trigger the snapshot process
					}
					if previousLastCommitedIndex < rn.log.lastCommitedIndex {
						for _, entry := range rn.log.entries[previousLastCommitedIndex+1 : rn.log.lastCommitedIndex+1] {
							// if we update the lastCommitedIndex, we have to apply to the state all the committed action entries
							if entry.Type == ActionEntry && entry.Command != nil {
								rn.CommitCh <- entry.Command
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
									// the other nodes to know that this configuration is commited)
									time.AfterFunc(100*time.Millisecond, func() {
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

							// if we update the lastCommitedIndex, we have also to update rn.lastUSNof
							if entry.Client != "" && entry.USN > rn.lastUSNof[entry.Client] {
								rn.lastUSNof[entry.Client] = entry.USN
							}
						}
					}

					rn.mutex.Unlock()
					return true
				} else {
					// inconsistent log entry in the follower, we have to decrement the nextIndex
					log.Println("inconsistency founded in " + node)
					rn.nextIndex[node] = previousLogIndex
					rn.mutex.Unlock()
					rn.handleReplicationLog(node, peerConnection)
					return false
				}
			case <-timeout.C:
				log.Println("The node", node, "doesn't responded in time to the AppendEntriesRPC made by node", rn.id, "retrying...")
			}
		}
	}
	rn.mutex.Unlock()
	return false
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
			rn.replicateLogToPeers()
		case <-rn.firstHeartbeatCh:
			rn.replicateLogToPeers()
		case <-ticker.C:
			rn.replicateLogToPeers()
		}
	}
}

func (rn *RaftNode) replicateLogToPeers() {
	rn.mutex.Lock()
	peers := make(map[ServerID]*rpc.Client)
	for node, peerConnection := range rn.peersConnection {
		if node != rn.id {
			peers[node] = peerConnection
		}
	}
	rn.mutex.Unlock()

	for node, peerConnection := range peers {
		go rn.handleReplicationLog(node, peerConnection)
	}
}
