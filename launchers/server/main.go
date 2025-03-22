package main

import (
	"RaftMsgStream/raft"
	"RaftMsgStream/server"
	"log"
	"os"
)

func main() {
	log.Println("Starting server in unvoting mode", os.Args[3])

	peers := make(map[raft.ServerID]raft.Address)
	if len(os.Args) > 3 {
		for i := 4; i < len(os.Args); i += 2 {
			peers[raft.ServerID(os.Args[i])] = raft.Address(os.Args[i+1])
		}
	}

	server := server.NewServer(
		raft.ServerID(os.Args[1]),
		os.Args[2],
		peers,
		os.Args[3] == "true",
	)

	server.PrepareConnectionsWithOtherServers()

	server.Run()
}
