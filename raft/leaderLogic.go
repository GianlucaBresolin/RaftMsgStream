package raft

import (
	"log"
	"net/rpc"
	"time"
)

const LeaderTimeout = 20

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
					updatedReplicationState := replicationState{
						replicationCounter: ns.pendingCommit[logEntryToReplicate.Index].replicationCounter + 1,
						replicationSuccess: ns.pendingCommit[logEntryToReplicate.Index].replicationSuccess,
					}
					if updatedReplicationState.replicationCounter > ns.numberNodes/2 && !updatedReplicationState.replicationSuccess {
						updatedReplicationState.replicationSuccess = true
						ns.log.lastCommitedIndex = logEntryToReplicate.Index
						delete(ns.pendingCommit, logEntryToReplicate.Index)
						log.Println("committed log entry in position", logEntryToReplicate.Index)
					}

					ns.pendingCommit[logEntryToReplicate.Index] = updatedReplicationState
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
