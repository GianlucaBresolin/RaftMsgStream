package main

import (
	"RaftMsgStream/models"
	"RaftMsgStream/raft"
	"RaftMsgStream/server"
	"log"
	"os"
)

func main() {
	log.Println("Starting server in unvoting mode", os.Args[4])

	peers := make(map[raft.ServerID]models.Address)
	if len(os.Args) > 4 {
		for i := 5; i < len(os.Args); i += 3 {
			peers[raft.ServerID(os.Args[i])] = models.Address{
				Address:  os.Args[i+1],
				RaftPort: os.Args[i+2],
			}
		}
	}

	server := server.NewServer(
		raft.ServerID(os.Args[1]),
		os.Args[2],
		os.Args[3],
		peers,
		os.Args[4] == "true",
	)

	server.PrepareConnectionsWithOtherServers()

	server.Run()
}
