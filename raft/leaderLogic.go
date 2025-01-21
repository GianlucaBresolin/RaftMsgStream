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
				for _, logEntryToReplicate := range logEntriesToReplicate {
					// the leader checks for commit just for log entries of the leader current term that are not committed
					repState, ok := ns.pendingCommit[logEntryToReplicate.Index]
					if logEntryToReplicate.Term == ns.term && ns.state == Leader && ok {
						committed := false
						if repState.replicationCounter+1 > ns.numberNodes/2 && !repState.committed {
							committed = true
							ns.log.lastCommitedIndex = logEntryToReplicate.Index
							repState.clientCh <- true
							log.Println("committed log entry in position", logEntryToReplicate.Index)
						}

						if committed {
							// remove pending commit
							delete(ns.pendingCommit, logEntryToReplicate.Index)
						} else {
							ns.pendingCommit[logEntryToReplicate.Index] = replicationState{
								replicationCounter: repState.replicationCounter + 1,
								committed:          committed,
							}
						}
					}
					ns.nextIndex[node] = logEntryToReplicate.Index + 1
				}
			} else {
				// inconsistent log entry in the follower
				log.Println("inconsistency founded")
				ns.nextIndex[node]--
				ns.handleReplicationLog(node, peerConnection)
			}
			ns.mutex.Unlock()
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
			for node, peerConnection := range ns.peersConnection {
				go ns.handleReplicationLog(node, peerConnection)
			}
		case <-ticker.C:
			// heartbeat
			for node, peerConnection := range ns.peersConnection {
				go ns.handleReplicationLog(node, peerConnection)
			}
		}
	}
}
