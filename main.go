package main

import (
	"RaftMsgStream/raft"
	"log"
	"net/rpc"
	"time"
)

func clientConnection(node raft.ServerID, port raft.Port) *rpc.Client {
	client, err := rpc.Dial("tcp", "localhost"+string(port))
	if err != nil {
		log.Printf("Failed to dial %s: %v", node, err)
	} else {
		log.Printf("Client has connected to %s", node)
	}
	return client
}

func main() {

	//node1 := raft.NewNode("node1", ":5001", map[raft.ServerID]raft.Port{})
	node1 := raft.NewNode("node1", ":5001", map[raft.ServerID]raft.Port{
		"node2": ":5002",
		"node3": ":5003"})
	node2 := raft.NewNode("node2", ":5002", map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node3": ":5003"})
	node3 := raft.NewNode("node3", ":5003", map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node2": ":5002"})

	nodeMap := map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node2": ":5002",
		"node3": ":5003",
	}

	node1.RegisterNode()
	node2.RegisterNode()
	node3.RegisterNode()

	node1.PrepareConnections()
	node2.PrepareConnections()
	node3.PrepareConnections()

	go node1.Run()
	go node2.Run()
	go node3.Run()

	successRequests := false
	client := clientConnection("node1", ":5001")

	for !successRequests {
		args := raft.ClientRequestArguments{
			Command: "Hello",
		}
		var reply raft.ClientRequestResult
		err := client.Call("Node.ClientRequestRPC", args, &reply)

		if err != nil {
			log.Printf("Failed to call ClientRequestRPC: %v", err)
			time.Sleep(1 * time.Second)
		} else {
			if reply.Success {
				successRequests = true
				continue
			}
			time.Sleep(1 * time.Second)
			if reply.Leader == "" {
				continue
			}
			client.Close()
			client = clientConnection(reply.Leader, nodeMap[reply.Leader])
		}
	}

	for {
	}
}
