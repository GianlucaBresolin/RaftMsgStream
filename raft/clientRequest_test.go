package raft

import (
	"testing"
)

func TestClientRequestArguments(t *testing.T) {
	var req ClientRequestArguments
	req.Command = []byte("test")
	req.Type = ActionEntry
	req.Id = "test"
	req.USN = 1

	var res ClientRequestResult

	leaderNode := NewRaftNode("Server1", ":5001", map[ServerID]Port{}, false)
	leaderNode.state = Leader
	leaderNode.lastUSNof = make(map[string]int)
	leaderNode.lastUSNof["test"] = 0

	go func() {
		<-leaderNode.logEntriesCh
		leaderNode.pendingCommit[1].clientCh <- true
	}()

	err := leaderNode.ActionRequestRPC(req, &res)
	if err != nil {
		t.Errorf("Failed to call ActionRPC: %v", err)
	}
	if !res.Success {
		t.Errorf("Expected success, got %v", res.Success)
	}
	if leaderNode.log.entries[1].Client != "test" {
		t.Errorf("Expected client test, got %v", leaderNode.log.entries[1].Client)
	}
}

func TestClientRequestStaleUSN(t *testing.T) {
	var req ClientRequestArguments
	req.Command = []byte("test")
	req.Type = ActionEntry
	req.Id = "test"
	req.USN = 1

	var res ClientRequestResult

	leaderNode := NewRaftNode("Server1", ":5001", map[ServerID]Port{}, false)
	leaderNode.state = Leader
	leaderNode.currentLeader = "Server1"
	leaderNode.lastUSNof = make(map[string]int)
	leaderNode.lastUSNof["test"] = 1

	err := leaderNode.ActionRequestRPC(req, &res)
	if err != nil {
		t.Errorf("Failed to call ActionRPC: %v", err)
	}
	if res.Success {
		t.Errorf("Expected failure, got %v", res.Success)
	}
	if res.Leader != "Server1" {
		t.Errorf("Expected leader Server1, got %v", res.Leader)
	}
}

func TestGetStateRPCVotingServer(t *testing.T) {
	var req ClientRequestArguments
	req.Command = []byte("test")
	req.Type = ActionEntry
	req.Id = "test"
	req.USN = 1

	var res ClientRequestResult

	leaderNode := NewRaftNode("Server1", ":5001", map[ServerID]Port{}, false)
	leaderNode.state = Leader
	leaderNode.lastUSNof = make(map[string]int)
	leaderNode.lastUSNof["test"] = 0

	go func() {
		i := 0
		for {
			<-leaderNode.logEntriesCh
			i++
			leaderNode.pendingCommit[uint(i)].clientCh <- true
		}
	}()

	go func() {
		<-leaderNode.ReadStateCh
		leaderNode.ReadStateResultCh <- []byte("test")
	}()

	err := leaderNode.GetStateRPC(req, &res)
	if err != nil {
		t.Errorf("Failed to call GetStateRPC: %v", err)
	}
	if !res.Success {
		t.Errorf("Expected success, got %v", res.Success)
	}
	if leaderNode.log.entries[1].Type != NOOPEntry {
		t.Errorf("Expected NOOP entry type, got %v", leaderNode.log.entries[1].Type)
	}
}

func TestGetStateRPCNonVotingServer(t *testing.T) {
	var req ClientRequestArguments
	req.Command = []byte("test")
	req.Type = ActionEntry
	req.Id = "test"
	req.USN = 1

	var res ClientRequestResult

	leaderNode := NewRaftNode("Server1", ":5001", map[ServerID]Port{}, false)
	leaderNode.state = Leader

	nonVotingNode := NewRaftNode("Server2", ":5002", map[ServerID]Port{"Server1": ":5001"}, true)
	nonVotingNode.currentLeader = "Server1"
	nonVotingNode.lastUSNof = make(map[string]int)
	nonVotingNode.lastUSNof["test"] = 0

	go func() {
		<-nonVotingNode.ReadStateCh
		nonVotingNode.ReadStateResultCh <- []byte("test")
	}()

	err := nonVotingNode.GetStateRPC(req, &res)
	if err != nil {
		t.Errorf("Failed to call GetStateRPC: %v", err)
	}
	if !res.Success {
		t.Errorf("Expected success, got %v", res.Success)
	}
	if res.Leader != "Server1" {
		t.Errorf("Expected leader Server1, got %v", res.Leader)
	}
}

func TestGetStateRPCStaleUSN(t *testing.T) {
	var req ClientRequestArguments
	req.Command = []byte("test")
	req.Type = ActionEntry
	req.Id = "test"
	req.USN = 1

	var res ClientRequestResult

	leaderNode := NewRaftNode("Server1", ":5001", map[ServerID]Port{}, false)
	leaderNode.state = Leader
	leaderNode.currentLeader = "Server1"
	leaderNode.lastUSNof = make(map[string]int)
	leaderNode.lastUSNof["test"] = 1

	err := leaderNode.GetStateRPC(req, &res)
	if err != nil {
		t.Errorf("Failed to call GetStateRPC: %v", err)
	}
	if res.Success {
		t.Errorf("Expected failure, got %v", res.Success)
	}
	if res.Leader != "Server1" {
		t.Errorf("Expected leader Server1, got %v", res.Leader)
	}
}

func TestGetStateRPCNotLeader(t *testing.T) {
	var req ClientRequestArguments
	req.Command = []byte("test")
	req.Type = ActionEntry
	req.Id = "test"
	req.USN = 1

	var res ClientRequestResult

	leaderNode := NewRaftNode("Server1", ":5001", map[ServerID]Port{}, false)
	leaderNode.state = Follower
	leaderNode.currentLeader = "Server2"

	err := leaderNode.GetStateRPC(req, &res)
	if err != nil {
		t.Errorf("Failed to call GetStateRPC: %v", err)
	}
	if res.Success {
		t.Errorf("Expected failure, got %v", res.Success)
	}
	if res.Leader != "Server2" {
		t.Errorf("Expected leader Server2, got %v", res.Leader)
	}
}
