package main

import (
	"RaftMsgStream/raft"
)

func main() {

	node1 := raft.NewNode("node1", ":5001", map[raft.ServerID]raft.Port{
		"node2": ":5002",
		"node3": ":5003"})
	node2 := raft.NewNode("node2", ":5002", map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node3": ":5003"})
	node3 := raft.NewNode("node3", ":5003", map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node2": ":5002"})

	node1.RegisterNode()
	node2.RegisterNode()
	node3.RegisterNode()

	node1.PrepareConnections()
	node2.PrepareConnections()
	node3.PrepareConnections()

	go node1.Run()
	go node2.Run()
	go node3.Run()

	select {}
}
