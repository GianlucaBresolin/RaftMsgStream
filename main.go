package main

import (
	"RaftMsgStream/client"
	"RaftMsgStream/raft"
	"RaftMsgStream/server"
)

func main() {
	// create a new server
	msgStreamServer1 := server.NewServer("Server1", ":5001", map[raft.ServerID]raft.Port{"Server2": ":5002", "Server3": ":5003"}, false)
	msgStreamServer2 := server.NewServer("Server2", ":5002", map[raft.ServerID]raft.Port{"Server1": ":5001", "Server3": ":5003"}, false)
	msgStreamServer3 := server.NewServer("Server3", ":5003", map[raft.ServerID]raft.Port{"Server1": ":5001", "Server2": ":5002"}, false)

	// prepare connections with other servers
	msgStreamServer1.PrepareConnectionsWithOtherServers()
	msgStreamServer2.PrepareConnectionsWithOtherServers()
	msgStreamServer3.PrepareConnectionsWithOtherServers()

	// run the server
	go msgStreamServer1.Run()
	go msgStreamServer2.Run()
	go msgStreamServer3.Run()

	client := client.NewClient("Client1", ":6001", map[string]string{"Server1": ":5001", "Server2": ":5002", "Server3": ":5003"})
	client.PrepareConnections()
	client.SendMessage("group1", "Hello World!")

	for {
	}
}
