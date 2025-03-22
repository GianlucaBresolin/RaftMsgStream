package server

import (
	"RaftMsgStream/models"
)

func (s *Server) actionRequest(req models.ClientActionArguments, res *models.ClientActionResult) error {
	s.raftNode.ActionRequest(req, res)
	return nil
}

func (s *Server) getState(req models.ClientGetStateArguments, res *models.ClientGetStateResult) error {
	s.raftNode.GetState(req, res)
	return nil
}
