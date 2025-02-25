package raft

import (
	"encoding/json"
	"net/rpc"
	"testing"
)

func TestPrepareCold_new(t *testing.T) {
	server := rpc.NewServer()
	node := NewRaftNode("node1", ":5001", server, map[ServerID]Port{"follower": ":5002"}, false)

	command := []byte(`{"NewConfig": {"follower": ":5002"}}`)
	result := node.prepareCold_new(command)

	if string(result) != `{"OldConfig":{"follower":":5002","node1":":5001"},"NewConfig":{"follower":":5002"}}` {
		t.Errorf("Expected result to be {\"OldConfig\":{\"follower\":\":5002\",\"node1\":\":5001\"},\"NewConfig\":{\"follower\":\":5002\"}}, got %s", result)
	}
}

func TestPrepareCnew(t *testing.T) {
	server := rpc.NewServer()
	node := NewRaftNode("node1", ":5001", server, map[ServerID]Port{"follower": ":5002"}, false)
	node.peers = Configuration{
		OldConfig: map[ServerID]Port{"follower": ":5002", "node1": ":5001"},
		NewConfig: map[ServerID]Port{"follower": ":5002"},
	}

	go func() {
		<-node.logEntriesCh
	}()

	node.prepareCnew()

	if string(node.log.entries[1].Command) != `{"OldConfig":null,"NewConfig":{"follower":":5002"}}` {
		t.Errorf("Expected command to be {\"OldConfig\":null,\"NewConfig\":{\"follower\":\":5002\"}}, got %s", node.log.entries[1].Command)
	}
}

func TestApplyConfiguration(t *testing.T) {
	server1 := rpc.NewServer()
	node1 := NewRaftNode("node1", ":5001", server1, map[ServerID]Port{"node2": ":5002"}, false)
	server2 := rpc.NewServer()
	node2 := NewRaftNode("node2", ":5002", server2, map[ServerID]Port{"node1": ":5001"}, false)

	node1.PrepareConnections()
	node2.PrepareConnections()

	server3 := rpc.NewServer()
	node3 := NewRaftNode("node3", ":5003", server3, map[ServerID]Port{"node1": ":5001", "node2": ":5002"}, true)
	node3.PrepareConnections()

	config := Configuration{
		OldConfig: map[ServerID]Port{"node2": ":5002", "node1": ":5001"},
		NewConfig: map[ServerID]Port{"node3": ":5003"},
	}
	c, _ := json.Marshal(config)

	node1.applyConfiguration(c)
	node3.applyConfiguration(c)

	if len(node1.peersConnection) != 3 {
		t.Errorf("Expected peersConnection to have 3 element, got %d", len(node1.peersConnection))
	}

	if node3.unvotingServer {
		t.Errorf("Expected unvotingServer to be false, got true")
	}
}

func TestApplyCommitedConfigurationShutDown(t *testing.T) {
	server1 := rpc.NewServer()
	node1 := NewRaftNode("node1", ":5001", server1, map[ServerID]Port{"node2": ":5002"}, false)
	server2 := rpc.NewServer()
	node2 := NewRaftNode("node2", ":5002", server2, map[ServerID]Port{"node1": ":5001"}, false)

	node1.PrepareConnections()
	node2.PrepareConnections()

	server3 := rpc.NewServer()
	node3 := NewRaftNode("node3", ":5003", server3, map[ServerID]Port{"node1": ":5001", "node2": ":5002"}, true)
	node3.PrepareConnections()

	config := Configuration{
		OldConfig: nil,
		NewConfig: map[ServerID]Port{"node3": ":5003"},
	}
	c, _ := json.Marshal(config)

	success := make(chan bool)

	go func() {
		<-node1.ShutdownCh
		success <- true
	}()

	node1.applyCommitedConfiguration(c)

	result := <-success
	if !result {
		t.Errorf("Expected all channels to be closed")
	}
}

func TestApplyCommitedConfigurationCloseConnection(t *testing.T) {
	server1 := rpc.NewServer()
	node1 := NewRaftNode("node1", ":5001", server1, map[ServerID]Port{"node2": ":5002"}, false)
	server2 := rpc.NewServer()
	node2 := NewRaftNode("node2", ":5002", server2, map[ServerID]Port{"node1": ":5001"}, false)

	node1.PrepareConnections()
	node2.PrepareConnections()

	config := Configuration{
		OldConfig: nil,
		NewConfig: map[ServerID]Port{"node1": ":5001"},
	}
	c, _ := json.Marshal(config)

	node1.applyCommitedConfiguration(c)

	if len(node1.peersConnection) != 1 {
		t.Errorf("Expected peersConnection to have 1 element, got %d", len(node1.peersConnection))
	}
}
