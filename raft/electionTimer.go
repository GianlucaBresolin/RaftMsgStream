package raft

import (
	"log"
	"math/rand"
	"time"
)

func (ns *nodeState) startTimer() {
	ns.electionTimer = time.NewTimer(time.Duration(MinElectionTimeout+rand.Intn(MaxElectionTimeout-MinElectionTimeout)) * time.Millisecond)
}

func (ns *nodeState) resetTimer() {
	if ns.electionTimer != nil && !ns.electionTimer.Stop() {
		select {
		case <-ns.electionTimer.C: //try to drain from the channel
		default:
		}
	}

	ns.electionTimer.Reset(time.Duration(MinElectionTimeout+rand.Intn(MaxElectionTimeout-MinElectionTimeout)) * time.Millisecond)
}

func (ns *nodeState) handleTimer() {
	for {
		select {
		case <-ns.electionTimer.C:
			//timer expired
			ns.mutex.Lock()
			log.Println("Election timeout expired, starting election...")
			ns.startElection()
			ns.resetTimer()
			ns.mutex.Unlock()
		}
	}
}
