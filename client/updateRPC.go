package client

import (
	"RaftMsgStream/models"
	"log"
)

func (c *Client) UpdateRPC(args models.UpdateARgs, reply *models.UpdateResult) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	getStateResonse := &models.GetStateResult{}
	err := c.Connections[args.Server].Call("Server.GetStateRPC",
		models.GetStateArgs{
			Group:            args.Group,
			LastMessageIndex: uint(len(c.groups[args.Group])),
		}, getStateResonse)
	if err != nil {
		log.Printf("Failed to call GetStateRPC: %v", err)
		reply.Success = false
	} else {
		// add the messages to the group
		c.groups[args.Group] = append(c.groups[args.Group], getStateResonse.Messages...)
		log.Println(c.groups[args.Group])
		reply.Success = true
	}
	return nil
}
