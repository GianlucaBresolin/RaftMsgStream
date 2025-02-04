package raft

import (
	"log"
	"testing"
	"time"
)

func TestHandleReplicationLog(t *testing.T) {
	rn := NewRaftNode("node1", ":5001", map[ServerID]Port{"follower": ":5002"}, false)
	follower := NewRaftNode("follower", ":5002", map[ServerID]Port{"node1": ":5001"}, false)

	rn.PrepareConnections()
	follower.PrepareConnections()

	time.Sleep(100 * time.Millisecond) // wait for connections to be established

	rn.state = Leader

	testEntry := LogEntry{
		Index:   1,
		Term:    0,
		Command: []byte("test"),
		Type:    ActionEntry,
		USN:     1,
	}

	rn.log.entries = append(rn.log.entries, testEntry)

	rn.nextIndex = make(map[ServerID]uint)
	rn.nextIndex["follower"] = 2
	rn.pendingCommit = make(map[uint]replicationState)
	responseCh := make(chan bool, 1)
	rn.pendingCommit[1] = replicationState{
		replicationCounterOldC: 1,
		replicationCounterNewC: 1,
		committedOldC:          true,
		committedNewC:          false,
		term:                   1,
		clientCh:               responseCh,
	}

	follower.startTimer()
	go follower.handleTimer()
	go rn.handleReplicationLog("follower", rn.peersConnection["follower"])
	time.Sleep(100 * time.Millisecond) // wait to let the replication happen

	if rn.nextIndex["follower"] != 2 {
		t.Errorf("Expected nextIndex to be 2, got %d", rn.nextIndex["follower"])
	}

	if follower.log.entries[1].Index != testEntry.Index && follower.log.entries[1].Term != testEntry.Term {
		t.Errorf("Expected follower log to contain the test entry, got %v", follower.log.entries[1])
	}

	if rn.log.lastCommitedIndex != 1 {
		t.Errorf("Expected lastCommitedIndex to be 1, got %d", rn.log.lastCommitedIndex)
	}

	commited := <-responseCh
	if !commited {
		t.Errorf("Expected response to be true, got false")
	}
}

func TestHandleReplicationLogDurignJointConsensus(t *testing.T) {
	rn := NewRaftNode("node1", ":5001", map[ServerID]Port{"follower1": ":5002", "follower2": ":5003"}, false)
	follower1 := NewRaftNode("follower1", ":5002", map[ServerID]Port{"node1": ":5001"}, false)
	follower2 := NewRaftNode("follower2", ":5003", map[ServerID]Port{"node1": ":5001"}, false)

	rn.PrepareConnections()
	follower1.PrepareConnections()
	follower2.PrepareConnections()
	time.Sleep(200 * time.Millisecond) // wait for connections to be established

	rn.peers.OldConfig = make(map[ServerID]Port)
	rn.peers.OldConfig["follower2"] = ":5003"
	rn.peers.OldConfig["node1"] = ":5001"

	rn.state = Leader

	testEntry := LogEntry{
		Index:   1,
		Term:    0,
		Command: []byte("test"),
		Type:    ActionEntry,
		USN:     1,
	}

	rn.log.entries = append(rn.log.entries, testEntry)

	rn.nextIndex = make(map[ServerID]uint)
	rn.nextIndex["follower1"] = 2
	rn.nextIndex["follower2"] = 2

	rn.pendingCommit = make(map[uint]replicationState)
	responseCh := make(chan bool, 1)
	rn.pendingCommit[1] = replicationState{
		replicationCounterOldC: 1,
		replicationCounterNewC: 1,
		committedOldC:          false,
		committedNewC:          false,
		term:                   1,
		clientCh:               responseCh,
	}

	log.Println(rn.peersConnection)

	follower1.startTimer()
	follower2.startTimer()
	go follower1.handleTimer()
	go follower2.handleTimer()
	go rn.handleReplicationLog("follower1", rn.peersConnection["follower1"])
	go rn.handleReplicationLog("follower2", rn.peersConnection["follower2"])
	time.Sleep(100 * time.Millisecond) // wait to let the replication happen

	if rn.nextIndex["follower1"] != 2 {
		t.Errorf("Expected nextIndex to be 2, got %d", rn.nextIndex["follower"])
	}
	if rn.nextIndex["follower2"] != 2 {
		t.Errorf("Expected nextIndex to be 2, got %d", rn.nextIndex["follower"])
	}

	if follower1.log.entries[1].Index != testEntry.Index && follower1.log.entries[1].Term != testEntry.Term {
		t.Errorf("Expected follower log to contain the test entry, got %v", follower1.log.entries[1])
	}
	if follower2.log.entries[1].Index != testEntry.Index && follower2.log.entries[1].Term != testEntry.Term {
		t.Errorf("Expected follower log to contain the test entry, got %v", follower2.log.entries[1])
	}

	if rn.log.lastCommitedIndex != 1 {
		t.Errorf("Expected lastCommitedIndex to be 1, got %d", rn.log.lastCommitedIndex)
	}

	commited := <-responseCh
	if !commited {
		t.Errorf("Expected response to be true, got false")
	}
}

func TestHandleReplicationLogRevertToFollower(t *testing.T) {
	rn := NewRaftNode("node1", ":5001", map[ServerID]Port{"follower": ":5002"}, false)
	follower := NewRaftNode("follower", ":5002", map[ServerID]Port{"node1": ":5001"}, false)

	rn.PrepareConnections()
	follower.PrepareConnections()

	time.Sleep(100 * time.Millisecond) // wait for connections to be established

	rn.state = Leader
	rn.nextIndex = make(map[ServerID]uint)
	follower.term = 1

	testEntry := LogEntry{
		Index:   1,
		Term:    0,
		Command: []byte("test"),
		Type:    ActionEntry,
		USN:     1,
	}

	rn.log.entries = append(rn.log.entries, testEntry)

	rn.nextIndex = make(map[ServerID]uint)
	rn.nextIndex["follower"] = 2
	rn.pendingCommit = make(map[uint]replicationState)
	responseCh := make(chan bool, 1)
	rn.pendingCommit[1] = replicationState{
		replicationCounterOldC: 1,
		replicationCounterNewC: 1,
		committedOldC:          true,
		committedNewC:          false,
		term:                   1,
		clientCh:               responseCh,
	}

	follower.startTimer()
	rn.startTimer()
	go follower.handleTimer()
	go rn.handleTimer()
	go rn.handleReplicationLog("follower", rn.peersConnection["follower"])
	<-rn.leaderCh

	if len(follower.log.entries) != 1 {
		t.Errorf("Expected follower log to doesn't contain the test entry, got its lenght of %v", len(follower.log.entries))
	}

	if rn.state != Follower {
		t.Errorf("Expected state to be Follower, got %v", rn.state)
	}
}
