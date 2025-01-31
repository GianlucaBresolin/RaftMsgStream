package server

import (
	"RaftMsgStream/raft"
)

type Server struct {
	raftNode     *raft.RaftNode
	stateMachine *msgStreamStateMachine
}

func NewServer(id raft.ServerID, port raft.Port, peers map[raft.ServerID]raft.Port, unvoting bool) *Server {
	stateMachine := newMsgStreamStateMachine(string(id))
	return &Server{
		raftNode:     raft.NewRaftNode(id, port, peers, stateMachine.commandCh, unvoting),
		stateMachine: stateMachine,
	}
}

func (s *Server) PrepareConnectionsWithOtherServers() {
	s.raftNode.PrepareConnections()
}

func (s *Server) Run() {
	go s.raftNode.HandleRaftNode()
	go s.stateMachine.handleMsgStreamStateMachine()
}
