package raft

import (
	"net/rpc"
	"testing"
	"time"
)

func TestHandleReplicationLog(t *testing.T) {
	rpcServerLeader := rpc.NewServer()
	leader := NewRaftNode("leader", ":5001", rpcServerLeader, map[ServerID]Address{"follower": ":5002"}, false)
	rpcServerFollower := rpc.NewServer()
	follower := NewRaftNode("follower", ":5002", rpcServerFollower, map[ServerID]Address{"leader": ":5001"}, false)

	leader.PrepareConnections()
	follower.PrepareConnections()

	time.Sleep(200 * time.Millisecond) // wait for connections to be established

	leader.state = Leader
	leader.available = true
	follower.available = true

	testEntry := LogEntry{
		Index:   1,
		Term:    0,
		Command: []byte("test"),
		Type:    ActionEntry,
		USN:     1,
	}

	leader.log.entries = append(leader.log.entries, testEntry)

	leader.nextIndex = make(map[ServerID]uint)
	leader.nextIndex["follower"] = 2
	leader.pendingCommit = make(map[uint]replicationState)
	responseCh := make(chan bool, 1)
	leader.pendingCommit[1] = replicationState{
		replicationCounterOldC: 1,
		replicationCounterNewC: 1,
		committedOldC:          true,
		committedNewC:          false,
		term:                   0,
		clientCh:               responseCh,
	}

	follower.startTimer()
	go follower.handleTimer()

	go leader.handleReplicationLog("follower", leader.peersConnection["follower"])
	time.Sleep(300 * time.Millisecond) // wait to let the replication happen

	if leader.nextIndex["follower"] != 2 {
		t.Errorf("Expected nextIndex to be 2, got %d", leader.nextIndex["follower"])
	}

	if follower.log.entries[1].Index != testEntry.Index && follower.log.entries[1].Term != testEntry.Term {
		t.Errorf("Expected follower log to contain the test entry, got %v", follower.log.entries[1])
	}

	if leader.log.lastCommitedIndex != 1 {
		t.Errorf("Expected lastCommitedIndex to be 1, got %d", leader.log.lastCommitedIndex)
	}

	commited := <-responseCh
	if !commited {
		t.Errorf("Expected response to be true, got false")
	}
}

func TestHandleReplicationLogDurignJointConsensus(t *testing.T) {
	// it needs to reach the majority of the old configuration and the new configuration
	rpcServerLeader := rpc.NewServer()
	leader := NewRaftNode("leader", ":5001", rpcServerLeader, map[ServerID]Address{"follower1": ":5002", "follower2": ":5003"}, false)
	rpcServerFollower1 := rpc.NewServer()
	follower1 := NewRaftNode("follower1", ":5002", rpcServerFollower1, map[ServerID]Address{"leader": ":5001"}, false)
	rpcServerFollower2 := rpc.NewServer()
	follower2 := NewRaftNode("follower2", ":5003", rpcServerFollower2, map[ServerID]Address{"leader": ":5001"}, false)

	leader.PrepareConnections()
	follower1.PrepareConnections()
	follower2.PrepareConnections()

	time.Sleep(200 * time.Millisecond) // wait for connections to be established

	leader.state = Leader
	leader.available = true
	follower1.available = true
	follower2.available = true

	leader.peers.OldConfig = make(map[ServerID]Address)
	leader.peers.OldConfig["follower2"] = ":5003"
	leader.peers.OldConfig["leader"] = ":5001"

	testEntry := LogEntry{
		Index:   1,
		Term:    0,
		Command: []byte("test"),
		Type:    ActionEntry,
		USN:     1,
	}

	leader.log.entries = append(leader.log.entries, testEntry)

	leader.nextIndex = make(map[ServerID]uint)
	leader.nextIndex["follower1"] = 1
	leader.nextIndex["follower2"] = 1

	leader.pendingCommit = make(map[uint]replicationState)
	responseCh := make(chan bool, 1)
	leader.pendingCommit[1] = replicationState{
		replicationCounterOldC: 1,
		replicationCounterNewC: 1,
		committedOldC:          false,
		committedNewC:          false,
		term:                   0,
		clientCh:               responseCh,
	}

	follower1.startTimer()
	follower2.startTimer()
	go follower1.handleTimer()
	go follower2.handleTimer()

	go leader.handleReplicationLog("follower1", leader.peersConnection["follower1"])
	go leader.handleReplicationLog("follower2", leader.peersConnection["follower2"])

	time.Sleep(100 * time.Millisecond) // wait to let the replication happen

	if leader.nextIndex["follower1"] != 2 {
		t.Errorf("Expected nextIndex to be 2, got %d", leader.nextIndex["follower"])
	}
	if leader.nextIndex["follower2"] != 2 {
		t.Errorf("Expected nextIndex to be 2, got %d", leader.nextIndex["follower"])
	}

	if follower1.log.entries[1].Index != testEntry.Index && follower1.log.entries[1].Term != testEntry.Term {
		t.Errorf("Expected follower log to contain the test entry, got %v", follower1.log.entries[1])
	}
	if follower2.log.entries[1].Index != testEntry.Index && follower2.log.entries[1].Term != testEntry.Term {
		t.Errorf("Expected follower log to contain the test entry, got %v", follower2.log.entries[1])
	}

	if leader.log.lastCommitedIndex != 1 {
		t.Errorf("Expected lastCommitedIndex to be 1, got %d", leader.log.lastCommitedIndex)
	}

	commited := <-responseCh
	if !commited {
		t.Errorf("Expected response to be true, got false")
	}
}

func TestHandleReplicationLogRevertToFollower(t *testing.T) {
	rpcServerLeader := rpc.NewServer()
	leader := NewRaftNode("node1", ":5001", rpcServerLeader, map[ServerID]Address{"follower": ":5002"}, false)
	rpcServerFollower := rpc.NewServer()
	follower := NewRaftNode("follower", ":5002", rpcServerFollower, map[ServerID]Address{"node1": ":5001"}, false)

	leader.PrepareConnections()
	follower.PrepareConnections()

	time.Sleep(200 * time.Millisecond) // wait for connections to be established

	follower.available = true
	leader.available = true

	leader.state = Leader
	leader.nextIndex = make(map[ServerID]uint)
	follower.term = 1

	testEntry := LogEntry{
		Index:   1,
		Term:    0,
		Command: []byte("test"),
		Type:    ActionEntry,
		USN:     1,
	}

	leader.log.entries = append(leader.log.entries, testEntry)

	leader.nextIndex = make(map[ServerID]uint)
	leader.nextIndex["follower"] = 2

	leader.pendingCommit = make(map[uint]replicationState)
	responseCh := make(chan bool, 1)
	leader.pendingCommit[1] = replicationState{
		replicationCounterOldC: 1,
		replicationCounterNewC: 1,
		committedOldC:          true,
		committedNewC:          false,
		term:                   1,
		clientCh:               responseCh,
	}

	leader.startTimer()
	go leader.handleTimer()

	follower.startTimer()
	go follower.handleTimer()

	go leader.handleReplicationLog("follower", leader.peersConnection["follower"])

	<-leader.leaderCh

	if len(follower.log.entries) >= 2 {
		t.Errorf("Expected follower log to doesn't contain the test entry, got its lenght of %v", len(follower.log.entries))
	}

	if leader.state != Follower {
		t.Errorf("Expected state to be Follower, got %v", leader.state)
	}
}
