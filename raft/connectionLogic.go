package raft

import (
	"log"
	"net/rpc"
)

func (rn *RaftNode) registerNode(server *rpc.Server) {
	err := server.Register(rn)
	if err != nil {
		log.Fatalf("Failed to register node %s: %v", rn.id, err)
	}
}

func (rn *RaftNode) PrepareConnections() {
	for peer, address := range rn.peers.NewConfig {
		if rn.peers.OldConfig[peer] == address {
			// skip if the peer is already connected
			continue
		}

		client, err := rpc.DialHTTP("tcp", string(address))
		if err != nil {
			log.Fatalf("Failed to dial %s: %v", peer, err)
		} else {
			log.Printf("Node %s connected to %s", rn.id, peer)
		}
		rn.peersConnection[peer] = client
	}
}

func (rn *RaftNode) closeConnections() {
	for peer, connection := range rn.peersConnection {
		if peer == rn.id {
			continue
		}
		connection.Close()
	}
}
