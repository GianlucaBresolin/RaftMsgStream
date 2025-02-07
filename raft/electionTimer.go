package raft

import (
	"log"
	"math/rand"
	"time"
)

func (rn *RaftNode) startTimer() {
	rn.minimumTimer = time.NewTimer(time.Duration(MinElectionTimeout) * time.Millisecond)
	rn.electionTimer = time.NewTimer(time.Duration(MinElectionTimeout+rand.Intn(MaxElectionTimeout-MinElectionTimeout)) * time.Millisecond)
}

func (rn *RaftNode) resetTimer() {
	if !rn.minimumTimer.Stop() {
		select {
		case <-rn.minimumTimer.C: // try to drain from the channel
		default:
		}
	}

	if !rn.electionTimer.Stop() {
		select {
		case <-rn.electionTimer.C: // try to drain from the channel
		default:
		}
	}

	rn.minimumTimer.Reset(time.Duration(MinElectionTimeout) * time.Millisecond)
	rn.electionTimer.Reset(time.Duration(MinElectionTimeout+rand.Intn(MaxElectionTimeout-MinElectionTimeout)) * time.Millisecond)
}

func (rn *RaftNode) handleTimer() {
	for {
		select {
		case <-rn.electionTimer.C:
			// timer expired
			rn.mutex.Lock()
			if rn.unvotingServer { // do not start election if the server is unvoting
				// maybe we lost the connection to the leader, so we retry
				rn.disconnectAsUnvotingNode()
				rn.connectAsUnvotingNode()
				rn.mutex.Unlock()
				continue
			}
			log.Println(rn.id, ": Election timeout expired, starting election...")
			rn.startElection()
			rn.resetTimer()
			rn.mutex.Unlock()
		case <-rn.shutdownTimersCh:
			return
		}
	}
}
