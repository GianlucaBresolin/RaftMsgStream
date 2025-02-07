package raft

import (
	"testing"
)

func TestUnvotingServerLogic(t *testing.T) {
	node1 := NewRaftNode("node1", ":5001", map[ServerID]Port{"node2": ":5002", "node3": ":5003"}, false)
	node2 := NewRaftNode("node2", ":5002", map[ServerID]Port{"node1": ":5001", "node3": ":5003"}, false)
	node3 := NewRaftNode("node3", ":5003", map[ServerID]Port{"node1": ":5001", "node2": ":5002"}, false)

	unvotingNode := NewRaftNode("node4", ":5005", map[ServerID]Port{"node1": ":5001", "node2": ":5002", "node3": ":5003"}, true)

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
