package server

import (
	"RaftMsgStream/models"
)

func (s *Server) ActionRequestRPC(req models.ClientRequestArguments, res *models.ClientRequestResult) error {
	s.raftNode.ActionRequest(req, res)
	return nil
}

func (s *Server) GetStateRPC(req models.ClientRequestArguments, res *models.ClientRequestResult) error {
	s.raftNode.GetState(req, res)
	return nil
}
