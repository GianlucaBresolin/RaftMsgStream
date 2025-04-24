package raft

import (
	"net/rpc"
	"testing"
	"time"
)

func TestRestetTimerWithDrain(t *testing.T) {
	node := RaftNode{
		electionTimer: time.NewTimer(time.Microsecond * 2),
		minimumTimer:  time.NewTimer(time.Microsecond),
	}

	time.Sleep(time.Microsecond * 3) // wait for timer to expire
	node.resetTimer()

	if node.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}
}

func TestRestetTimerWithoutDrain(t *testing.T) {
	node := RaftNode{
		electionTimer: time.NewTimer(time.Microsecond * 2),
		minimumTimer:  time.NewTimer(time.Microsecond),
	}

	node.resetTimer()

	if node.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}
}

func TestHandleTimer(t *testing.T) {
	rpcServer := rpc.NewServer()
	node := NewRaftNode("node1", ":5001", rpcServer, map[ServerID]Address{}, false)

	node.startTimer()
	go node.handleTimer()

	time.Sleep(time.Second * 2) // wait for election to start

	if node.electionTimer == nil {
		t.Error("electionTimer should not be nil")
	}

	if node.state != Candidate {
		t.Error("state should be candidate, got", node.state)
	}
}
