package server

import (
	"RaftMsgStream/models"
)

func (s *Server) actionRequest(req models.ClientActionArguments) models.ClientActionResult {
	return s.raftNode.ActionRequest(req)
}

func (s *Server) getState(req models.ClientGetStateArguments) models.ClientGetStateResult {
	return s.raftNode.GetState(req)
}
