package server

import (
	"RaftMsgStream/raft"
)

type Server struct {
	raftNode     *raft.RaftNode
	stateMachine *msgStreamStateMachine
}

func NewServer(id raft.ServerID, port raft.Port, peers map[raft.ServerID]raft.Port, unvoting bool) *Server {
	raftNode := raft.NewRaftNode(id, port, peers, unvoting)
	return &Server{
		raftNode:     raftNode,
		stateMachine: newMsgStreamStateMachine(string(id), raftNode.CommitCh, raftNode.SnapshotRequestCh, raftNode.SnapshotResponseCh, raftNode.ApplySnapshotCh, raftNode.ReadStateCh, raftNode.ReadStateResultCh),
	}
}

func (s *Server) PrepareConnectionsWithOtherServers() {
	s.raftNode.PrepareConnections()
}

func (s *Server) Run() {
	go s.raftNode.HandleRaftNode()
	go s.stateMachine.handleMsgStreamStateMachine()
}
