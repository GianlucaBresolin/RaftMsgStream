package raft

import (
	"sync"
	"testing"
	"time"
)

func TestRestetTimerWithDrain(t *testing.T) {
	ns := nodeState{
		electionTimer: time.NewTimer(time.Microsecond * 2),
		minimumTimer:  time.NewTimer(time.Microsecond),
	}

	time.Sleep(time.Microsecond * 2) // wait for timer to expire
	ns.resetTimer()

	if ns.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}
}

func TestRestetTimerWithoutDrain(t *testing.T) {
	ns := nodeState{
		electionTimer: time.NewTimer(time.Microsecond * 2),
		minimumTimer:  time.NewTimer(time.Microsecond),
	}

	ns.resetTimer()

	if ns.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}
}

func TestHandleTimer(t *testing.T) {
	ns := nodeState{
		electionTimer: time.NewTimer(time.Microsecond * 2),
		minimumTimer:  time.NewTimer(time.Microsecond),
		mutex:         sync.Mutex{},
	}

	go ns.handleTimer()

	if ns.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}

	time.Sleep(time.Second * 2) // wait for election to start
	if ns.state != Candidate {
		t.Error("state should be candidate, got", ns.state)
	}
}
