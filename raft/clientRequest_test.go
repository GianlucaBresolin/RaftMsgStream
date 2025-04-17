package raft

import (
	"RaftMsgStream/models"
	"net/rpc"
	"testing"
)

func TestClientRequestArguments(t *testing.T) {
	var req models.ClientActionArguments
	req.Command = []byte("test")
	req.Type = ActionEntry
	req.Id = "test"
	req.USN = 1

	var res models.ClientActionResult

	rpcServer := rpc.NewServer()
	node := NewRaftNode("Node1", "localhost:5001", rpcServer, map[ServerID]Address{}, false)
	node.state = Leader
	node.lastUSNof = make(map[string]int)
	node.lastUSNof["test"] = 0

	go func() {
		<-node.logEntriesCh
		node.pendingCommit[1].clientCh <- true
	}()

	node.ActionRequest(req, &res)
	if !res.Success {
		t.Errorf("Expected success, got %v", res.Success)
	}
	if node.log.entries[1].Client != "test" {
		t.Errorf("Expected client test, got %v", node.log.entries[1].Client)
	}
}

func TestClientRequestStaleUSN(t *testing.T) {
	var req models.ClientActionArguments
	req.Command = []byte("test")
	req.Type = ActionEntry
	req.Id = "test"
	req.USN = 1

	var res models.ClientActionResult

	rpcServer := rpc.NewServer()
	leaderNode := NewRaftNode("Server1", "localhost:5001", rpcServer, map[ServerID]Address{}, false)
	leaderNode.state = Leader
	leaderNode.currentLeader = "Server1"
	leaderNode.lastUSNof = make(map[string]int)
	leaderNode.lastUSNof["test"] = 1

	leaderNode.ActionRequest(req, &res)
	if res.Success {
		t.Errorf("Expected failure, got %v", res.Success)
	}
	if res.Leader != "Server1" {
		t.Errorf("Expected leader Server1, got %v", res.Leader)
	}
}

func TestGetStateRPCVotingServer(t *testing.T) {
	var req models.ClientGetStateArguments
	req.Command = []byte("test")
	req.Id = "test"
	req.USN = 1

	var res models.ClientGetStateResult

	rpcServer := rpc.NewServer()
	leaderNode := NewRaftNode("Server1", ":5001", rpcServer, map[ServerID]Address{}, false)
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

	leaderNode.GetState(req, &res)
	if !res.Success {
		t.Errorf("Expected success, got %v", res.Success)
	}
	if leaderNode.log.entries[1].Type != NOOPEntry {
		t.Errorf("Expected NOOP entry type, got %v", leaderNode.log.entries[1].Type)
	}
}

func TestGetStateRPCNonVotingServer(t *testing.T) {
	var req models.ClientGetStateArguments
	req.Command = []byte("test")
	req.Id = "test"
	req.USN = 1

	var res models.ClientGetStateResult

	rpcServer := rpc.NewServer()
	leaderNode := NewRaftNode("Server1", "localhost:5001", rpcServer, map[ServerID]Address{}, false)
	leaderNode.state = Leader

	nonVotingNode := NewRaftNode("Server2", "localhost:5002", rpcServer, map[ServerID]Address{"Server1": "localhost:5001"}, true)
	nonVotingNode.currentLeader = "Server1"
	nonVotingNode.lastUSNof = make(map[string]int)
	nonVotingNode.lastUSNof["test"] = 0

	go func() {
		<-nonVotingNode.ReadStateCh
		nonVotingNode.ReadStateResultCh <- []byte("test")
	}()

	nonVotingNode.GetState(req, &res)
	if !res.Success {
		t.Errorf("Expected success, got %v", res.Success)
	}
	if res.Leader != "Server1" {
		t.Errorf("Expected leader Server1, got %v", res.Leader)
	}
}

func TestGetStateRPCStaleUSN(t *testing.T) {
	var req models.ClientGetStateArguments
	req.Command = []byte("test")
	req.Id = "test"
	req.USN = 1

	var res models.ClientGetStateResult

	rpcServer := rpc.NewServer()
	leaderNode := NewRaftNode("Server1", ":5001", rpcServer, map[ServerID]Address{}, false)
	leaderNode.state = Leader
	leaderNode.currentLeader = "Server1"
	leaderNode.lastUSNof = make(map[string]int)
	leaderNode.lastUSNof["test"] = 1

	leaderNode.GetState(req, &res)
	if res.Success {
		t.Errorf("Expected failure, got %v", res.Success)
	}
	if res.Leader != "Server1" {
		t.Errorf("Expected leader Server1, got %v", res.Leader)
	}
}

func TestGetStateRPCNotLeader(t *testing.T) {
	var req models.ClientGetStateArguments
	req.Command = []byte("test")
	req.Id = "test"
	req.USN = 1

	var res models.ClientGetStateResult

	rpcServer := rpc.NewServer()
	leaderNode := NewRaftNode("Server1", "localhost:5001", rpcServer, map[ServerID]Address{}, false)
	leaderNode.state = Follower
	leaderNode.currentLeader = "Server2"

	leaderNode.GetState(req, &res)
	if res.Success {
		t.Errorf("Expected failure, got %v", res.Success)
	}
	if res.Leader != "Server2" {
		t.Errorf("Expected leader Server2, got %v", res.Leader)
	}
}
