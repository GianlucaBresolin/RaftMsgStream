package raft

import (
	"log"
	"math/rand"
	"time"
)

func (ns *nodeState) startTimer() {
	ns.minimumTimer = time.NewTimer(time.Duration(MinElectionTimeout) * time.Millisecond)
	ns.electionTimer = time.NewTimer(time.Duration(MinElectionTimeout+rand.Intn(MaxElectionTimeout-MinElectionTimeout)) * time.Millisecond)
}

func (ns *nodeState) resetTimer() {
	if !ns.minimumTimer.Stop() {
		select {
		case <-ns.minimumTimer.C: // try to drain from the channel
		default:
		}
	}

	if !ns.electionTimer.Stop() {
		select {
		case <-ns.electionTimer.C: // try to drain from the channel
		default:
		}
	}

	ns.minimumTimer.Reset(time.Duration(MinElectionTimeout) * time.Millisecond)
	ns.electionTimer.Reset(time.Duration(MinElectionTimeout+rand.Intn(MaxElectionTimeout-MinElectionTimeout)) * time.Millisecond)
}

func (ns *nodeState) handleTimer() {
	for {
		select {
		case <-ns.electionTimer.C:
			// timer expired
			ns.mutex.Lock()
			if ns.unvotingServer { // do not start election if the server is unvoting
				ns.mutex.Unlock()
				continue
			}
			log.Println(ns.id, ": Election timeout expired, starting election...")
			ns.startElection()
			ns.resetTimer()
			ns.mutex.Unlock()
		case <-ns.shutdownTimers:
			return
		}
	}
}
