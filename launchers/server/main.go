package main

import (
	"RaftMsgStream/raft"
	"RaftMsgStream/server"
	"log"
	"os"
)

func main() {
	log.Println("Starting server in unvoting mode", os.Args[1])
	server := server.NewServer(
		raft.ServerID("node1"),
		raft.Address("localhost:5001"),
		map[raft.ServerID]raft.Address{},
		os.Args[1] == "true",
	)

	server.PrepareConnectionsWithOtherServers()

	go server.Run()
}
