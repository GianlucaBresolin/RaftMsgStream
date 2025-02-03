package raft

import (
	"sync"
	"testing"
	"time"
)

func TestRestetTimerWithDrain(t *testing.T) {
	rn := RaftNode{
		electionTimer: time.NewTimer(time.Microsecond * 2),
		minimumTimer:  time.NewTimer(time.Microsecond),
	}

	time.Sleep(time.Microsecond * 2) // wait for timer to expire
	rn.resetTimer()

	if rn.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}
}

func TestRestetTimerWithoutDrain(t *testing.T) {
	rn := RaftNode{
		electionTimer: time.NewTimer(time.Microsecond * 2),
		minimumTimer:  time.NewTimer(time.Microsecond),
	}

	rn.resetTimer()

	if rn.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}
}

func TestHandleTimer(t *testing.T) {
	rn := RaftNode{
		electionTimer: time.NewTimer(time.Microsecond * 2),
		minimumTimer:  time.NewTimer(time.Microsecond),
		mutex:         sync.Mutex{},
	}

	go rn.handleTimer()

	if rn.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}

	time.Sleep(time.Second * 2) // wait for election to start
	if rn.state != Candidate {
		t.Error("state should be candidate, got", rn.state)
	}
}
