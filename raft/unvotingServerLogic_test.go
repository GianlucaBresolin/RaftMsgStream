package raft

import (
	"net/rpc"
	"testing"
)

func TestUnvotingServerLogic(t *testing.T) {
	server1 := rpc.NewServer()
	node1 := NewRaftNode("node1", ":5001", server1, map[ServerID]Port{"node2": ":5002", "node3": ":5003"}, false)
	server2 := rpc.NewServer()
	node2 := NewRaftNode("node2", ":5002", server2, map[ServerID]Port{"node1": ":5001", "node3": ":5003"}, false)
	server3 := rpc.NewServer()
	node3 := NewRaftNode("node3", ":5003", server3, map[ServerID]Port{"node1": ":5001", "node2": ":5002"}, false)

	server4 := rpc.NewServer()
	unvotingNode := NewRaftNode("node4", ":5005", server4, map[ServerID]Port{"node1": ":5001", "node2": ":5002", "node3": ":5003"}, true)

	node1.PrepareConnections()
	node2.PrepareConnections()
	node3.PrepareConnections()
	unvotingNode.PrepareConnections()

	go node1.HandleRaftNode()
	go node2.HandleRaftNode()
	go node3.HandleRaftNode()

	// add the unvoting node
	unvotingNode.connectAsUnvotingNode()

	var currentLeader *RaftNode
	switch node1.currentLeader {
	case "node1":
		currentLeader = node1
	case "node2":
		currentLeader = node2
	case "node3":
		currentLeader = node3
	}

	if len(currentLeader.peersConnection) != 3 {
		t.Errorf("Expected 3 peers, got %d", len(currentLeader.peersConnection))
	}

	unvotingNode.disconnectAsUnvotingNode()
	if len(currentLeader.peersConnection) != 2 {
		t.Errorf("Expected 2 peers, got %d", len(currentLeader.peersConnection))
	}
}
