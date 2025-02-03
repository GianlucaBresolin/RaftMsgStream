package raft

import (
	"log"
	"net"
	"net/rpc"
)

func (rn *RaftNode) registerNode() {
	server := rpc.NewServer()
	err := server.Register(rn)
	if err != nil {
		log.Fatalf("Failed to register node %s: %v", rn.id, err)
	}

	listener, err := net.Listen("tcp", string(rn.port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", rn.port, err)
	}
	log.Printf("Node %s is listening on %s\n", rn.id, rn.port)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Error accepting connection: %v", err)
				continue
			}
			go server.ServeConn(conn)
		}
	}()
}

func (rn *RaftNode) PrepareConnections() {
	for peer, port := range rn.peers.NewConfig {
		go func() {
			if rn.peers.OldConfig[peer] == port {
				// skip if the peer is already connected
				return
			}

			client, err := rpc.Dial("tcp", "localhost"+string(port))
			if err != nil {
				log.Fatalf("Failed to dial %s: %v", peer, err)
			} else {
				log.Printf("Node %s connected to %s", rn.id, peer)
			}
			rn.peersConnection[peer] = client
		}()
	}
}
