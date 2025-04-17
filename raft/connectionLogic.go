package raft

import (
	"errors"
	"log"
	"net/rpc"
)

func (rn *RaftNode) PrepareConnections() error {
	e := error(nil)
	for peer, address := range rn.peers.NewConfig {
		if rn.peersConnection[peer] != nil {
			// skip if the peer is already connected
			continue
		}

		client, err := rpc.DialHTTP("tcp", string(address))
		if err != nil {
			log.Printf("Failed to dial %s: %v", peer, err)
			e = errors.New("failed to dial")
		} else {
			log.Printf("Node %s connected to %s", rn.id, peer)
			rn.peersConnection[peer] = client
		}
	}
	return e
}

func (rn *RaftNode) closeConnections() {
	for peer, connection := range rn.peersConnection {
		if peer == rn.id {
			continue
		}
		connection.Close()
	}
}
