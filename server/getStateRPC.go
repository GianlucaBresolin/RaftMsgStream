package server

import (
	"RaftMsgStream/models"
)

func (s *Server) GetStateRPC(args models.GetStateArgs, reply *models.GetStateResult) error {
	_, ok := s.stateMachine.groups[args.Group]
	if !ok {
		reply.Messages = []models.Message{}
	} else {
		reply.Messages = s.stateMachine.groups[args.Group].messages[args.LastMessageIndex+1:]
	}
	return nil
}
