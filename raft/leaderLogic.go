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
			var arg AppendEntriesArguments

			ns.mutex.Lock()
			if ns.state == Leader {
				// TODO
				arg = AppendEntriesArguments{
					Term:             ns.term,
					LeaderId:         ns.id,
					PreviousLogIndex: 0,
					PreviousLogTerm:  0,
					Entries:          []logEntry{},
					LeaderCommit:     0,
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
