package raft

import (
	"log"
	"net/rpc"
	"time"
)

func (ns *nodeState) handleReplicationLog(node ServerID, peerConnection *rpc.Client) {
	// replicate log entry
	ns.mutex.Lock()

	// build info for consistency check
	previousLogIndex := ns.nextIndex[node] - 1
	previousLogTerm := ns.log.entries[previousLogIndex].Term

	// build log entries to replicate
	var logEntriesToReplicate []LogEntry
	if ns.log.lastIndex() >= ns.nextIndex[node] {
		logEntriesToReplicate = ns.log.entries[ns.nextIndex[node]:]
	}

	// build arguments for AppendEntriesRPC
	arg := AppendEntriesArguments{
		Term:             ns.term,
		LeaderId:         ns.id,
		PreviousLogIndex: previousLogIndex,
		PreviousLogTerm:  previousLogTerm,
		Entries:          logEntriesToReplicate,
		LeaderCommit:     ns.log.lastCommitedIndex,
	}

	if ns.state == Leader {
		ns.mutex.Unlock()

		failedReplicationRequest := false
		for !failedReplicationRequest {
			res := &AppendEntriesResult{}

			err := peerConnection.Call(
				"Node.AppendEntriesRPC",
				&arg,
				res)

			if err != nil {
				log.Println("Error sending AppendEntriesRPC to", ns.id, ":", err)
				log.Println("Retrying...")
				continue
			}
			failedReplicationRequest = true

			ns.mutex.Lock()
			if res.Term > ns.term {
				ns.revertToFollower()
				ns.term = res.Term
			}

			if res.Success {
				previousLastCommitedIndex := ns.log.lastCommitedIndex

				for _, logEntryToReplicate := range logEntriesToReplicate {
					// the leader checks for commit just for log entries of the leader current term that are not committed
					repState, ok := ns.pendingCommit[logEntryToReplicate.Index]
					_, okInOldC := ns.peers.OldConfig[node]
					_, okInNewC := ns.peers.NewConfig[node]

					if logEntryToReplicate.Term == ns.term && ns.state == Leader && ok {
						// update the replication state
						// joint-consensus: checks also for the old configuraiton
						if !repState.committedOldC && okInOldC {
							if repState.replicationCounterOldC+1 > uint(len(ns.peers.OldConfig)/2) {
								repState.committedOldC = true
							} else {
								repState.replicationCounterOldC++
							}
						}

						// normal consensus: checks only for the new configuration
						if !repState.committedNewC && okInNewC {
							if repState.replicationCounterNewC+1 > uint(len(ns.peers.NewConfig)/2) {
								repState.committedNewC = true
							} else {
								repState.replicationCounterNewC++
							}
						}

						if repState.committedOldC && repState.committedNewC {
							ns.log.lastCommitedIndex = logEntryToReplicate.Index
							repState.clientCh <- true
							log.Println("committed log entry in position", logEntryToReplicate.Index)
							delete(ns.pendingCommit, logEntryToReplicate.Index) // remove the entry from the pending commit
						} else {
							ns.pendingCommit[logEntryToReplicate.Index] = repState // update the replication state
						}
					}
					ns.nextIndex[node] = logEntryToReplicate.Index + 1
				}

				// if we update the lastCommitedIndex, we have also to update ns.lastUSNof and ns.pendingRequestof
				for _, entry := range ns.log.entries[previousLastCommitedIndex : ns.log.lastCommitedIndex+1] {
					if entry.Client != "" && entry.USN > ns.lastUSNof[entry.Client] {
						ns.lastUSNof[entry.Client] = entry.USN

						if ns.lastUncommitedRequestof[entry.Client] == entry.USN {
							// remove last request only if it is the same USN
							delete(ns.lastUncommitedRequestof, entry.Client)
						}
					}
				}
				ns.mutex.Unlock()
			} else {
				// inconsistent log entry in the follower
				log.Println("inconsistency founded")
				ns.nextIndex[node]--
				ns.mutex.Unlock()
				ns.handleReplicationLog(node, peerConnection)
			}
		}
	}
}

func (ns *nodeState) handleLeadership() {
	ticker := time.NewTicker(LeaderTimeout * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case stopLeadership := <-ns.leaderCh:
			if stopLeadership {
				log.Println("Node", ns.id, "lost leadership")
				return
			}
		case <-ns.logEntriesCh:
			ns.mutex.Lock()
			for node, peerConnection := range ns.peersConnection {
				go ns.handleReplicationLog(node, peerConnection)
			}
			ns.mutex.Unlock()
		case <-ns.firstHeartbeatCh:
			ns.mutex.Lock()
			// the first heartbeat is sent immediately
			for node, peerConnection := range ns.peersConnection {
				go ns.handleReplicationLog(node, peerConnection)
			}
			ns.mutex.Unlock()
		case <-ticker.C:
			// heartbeat
			ns.mutex.Lock()
			for node, peerConnection := range ns.peersConnection {
				go ns.handleReplicationLog(node, peerConnection)
			}
			ns.mutex.Unlock()
		}
	}
}
