package raft

import (
	"log"
	"time"
)

const LeaderTimeout = 20

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
		case <-ticker.C:
			ns.mutex.Lock()
			arg := AppendEntriesArguments{
				Term:             ns.term,
				LeaderId:         ns.id,
				PreviousLogIndex: 0,
				PreviousLogTerm:  0,
				Entries:          []LogEntry{},
				LeaderCommit:     0,
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
