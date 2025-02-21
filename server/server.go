package server

import (
	"RaftMsgStream/raft"
	"log"
	"net"
	"net/http"
	"net/rpc"
)

type Server struct {
	raftNode     *raft.RaftNode
	stateMachine *msgStreamStateMachine
}

func NewServer(id raft.ServerID, port raft.Port, peers map[raft.ServerID]raft.Port, unvoting bool) *Server {
	server := rpc.NewServer()

	raftNode := raft.NewRaftNode(id, port, server, peers, unvoting)
	s := &Server{
		raftNode:     raftNode,
		stateMachine: newMsgStreamStateMachine(string(id), raftNode.CommitCh, raftNode.SnapshotRequestCh, raftNode.SnapshotResponseCh, raftNode.ApplySnapshotCh, raftNode.ReadStateCh, raftNode.ReadStateResultCh),
	}

	// Register the server for the client RPC interractions
	err := server.Register(s)
	if err != nil {
		log.Fatalf("Failed to register server %s: %v", id, err)
	}

	mux := http.NewServeMux()
	mux.Handle(rpc.DefaultRPCPath, server)

	listener, err := net.Listen("tcp", string(port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", string(port), err)
	}
	log.Printf("Server %s is listening on %s\n", string(id), string(port))

	go http.Serve(listener, mux)

	return s
}

func (s *Server) PrepareConnectionsWithOtherServers() {
	s.raftNode.PrepareConnections()
}

func (s *Server) Run() {
	go s.raftNode.HandleRaftNode()
	go s.stateMachine.handleMsgStreamStateMachine()
}

func (s *Server) Close() {
	s.raftNode.ShutdownCh <- struct{}{}
	s.stateMachine.shutdownCh <- struct{}{}
}
