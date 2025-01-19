package raft

import (
	"log"
	"sync"
	"time"
)

const LeaderTimeout = 20

type replicationState struct {
	replicationMap     map[ServerID]bool
	replicationCounter uint
	replicationSuccess bool
	mutex              sync.Mutex
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
		case logEntryToReplicate := <-ns.logEntriesCh:
			// replicate log entry
			ns.mutex.Lock()

			// build info for consistency check
			previousLogIndex := logEntryToReplicate.Index - 1
			previousLogTerm := ns.log.entries[previousLogIndex].Term

			// build arguments for AppendEntriesRPC
			arg := AppendEntriesArguments{
				Term:             ns.term,
				LeaderId:         ns.id,
				PreviousLogIndex: previousLogIndex,
				PreviousLogTerm:  previousLogTerm,
				Entries:          []LogEntry{*logEntryToReplicate},
				LeaderCommit:     ns.log.lastCommitedIndex,
			}

			if ns.state == Leader {
				replicationState := replicationState{
					replicationMap:     make(map[ServerID]bool),
					replicationCounter: 1, // leader already replicated
					replicationSuccess: false,
				}

				for node := range ns.peers {
					replicationState.replicationMap[node] = false
				}
				ns.mutex.Unlock()

				for node, peerConnection := range ns.peersConnection {
					go func() {
						for !replicationState.replicationMap[node] {
							res := &AppendEntriesResult{}

							err := peerConnection.Call(
								"Node.AppendEntriesRPC",
								&arg,
								res)

							if err != nil {
								log.Println("Error sending AppendEntriesRPC to", ns.id, ":", err)
							}

							ns.mutex.Lock()
							if res.Term > ns.term {
								ns.revertToFollower()
								ns.term = res.Term
							}
							ns.mutex.Unlock()

							if res.Success {
								replicationState.replicationMap[node] = true
								replicationState.mutex.Lock()
								replicationState.replicationCounter++
								if replicationState.replicationCounter > ns.numberNodes/2 && !replicationState.replicationSuccess {
									// commit the log entry in the replicationState
									replicationState.replicationSuccess = true
									ns.pendingCommit[logEntryToReplicate.Index] <- true

									// commit log entry in the nodestate
									ns.mutex.Lock()
									entry := ns.log.entries[logEntryToReplicate.Index]
									entry.Committed = true
									ns.log.entries[logEntryToReplicate.Index] = entry
									if logEntryToReplicate.Index > ns.log.lastCommitedIndex {
										ns.log.lastCommitedIndex = logEntryToReplicate.Index
									}
									ns.mutex.Unlock()
								}
								replicationState.mutex.Unlock()
							} else {
								//TODO: manage failure logic of replication: inconsistency between leader and follower logs
							}
						}
					}()
				}
			} else {
				ns.mutex.Unlock()
			}

		case <-ticker.C:
			// heartbeat
			var arg AppendEntriesArguments

			ns.mutex.Lock()
			if ns.state == Leader {
				// TODO
				arg = AppendEntriesArguments{
					Term:             ns.term,
					LeaderId:         ns.id,
					PreviousLogIndex: 0,
					PreviousLogTerm:  0,
					Entries:          []LogEntry{},
					LeaderCommit:     ns.log.lastCommitedIndex,
				}
			} else {
				ns.mutex.Unlock()
				continue // not a leader anymore
			}
			ns.mutex.Unlock()

			for _, peerConnection := range ns.peersConnection {
				go func() {
					res := &AppendEntriesResult{}

					err := peerConnection.Call(
						"Node.AppendEntriesRPC",
						&arg,
						res)

					if err != nil {
						log.Println("Error sending AppendEntriesRPC to", ns.id, ":", err)
					}

					ns.mutex.Lock()
					if res.Term > ns.term {
						ns.revertToFollower()
						ns.term = res.Term
					}
					ns.mutex.Unlock()
				}()
			}
		}
	}
}
