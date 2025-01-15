package raft

import (
	"log"
	"net"
	"net/rpc"
)

type Node struct {
	state *nodeState
	port  Port
}

func NewNode(id ServerID, port Port, peers map[ServerID]Port) *Node {
	return &Node{
		state: newNodeState(id, peers),
		port:  port,
	}
}

func (n *Node) RegisterNode() {
	server := rpc.NewServer()
	err := server.Register(n)
	if err != nil {
		log.Fatalf("Failed to register node %s: %v", n.state.id, err)
	}

	listener, err := net.Listen("tcp", string(n.port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", n.port, err)
	}
	log.Printf("Node %s is listening on %s\n", n.state.id, n.port)

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

func (n *Node) PrepareConnections() {
	for peer, port := range n.state.peers {
		client, err := rpc.Dial("tcp", "localhost"+string(port))
		if err != nil {
			log.Printf("Failed to dial %s: %v", peer, err)
		} else {
			log.Printf("Node %s connected to %s", n.state.id, peer)
		}
		n.state.peersConnection[peer] = client
	}
}

func (n *Node) Run() {
	n.state.resetTimer()

	go n.state.handleTimer()
	go n.state.handleMyElection()

	for {
		//do things
	}
}
