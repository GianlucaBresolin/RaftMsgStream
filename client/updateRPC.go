package client

import (
	"RaftMsgStream/models"
	"RaftMsgStream/raft"
	"encoding/json"
	"log"
)

type UpdateARgs struct {
	Server    string
	Group     string
	RequestId int
}

type UpdateResult struct {
	Success bool
}

type GetStateArgs struct {
	Group            string
	LastMessageIndex uint
}

type GetStateResult struct {
	Messages []models.Message
}

func (c *Client) UpdateRPC(args UpdateARgs, reply *UpdateResult) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// check if the update is stale
	if args.RequestId <= c.LastRequestID {
		reply.Success = true
		return nil
	}

	// build the command for the read
	getStateArgs := &GetStateArgs{
		Group:            args.Group,
		LastMessageIndex: uint(len(c.groups[args.Group]) - 1),
	}
	command, err := json.Marshal(getStateArgs)
	if err != nil {
		log.Printf("Failed to marshal GetStateArgs: %v", err)
		reply.Success = false
		return nil
	}

	// call the getStateRPC on the server
	getStateResonse := &raft.ClientRequestResult{}
	failRead := false
	for !failRead {
		err := c.Connections[args.Server].Call("RaftNode.GetStateRPC",
			raft.ClientRequestArguments{
				Command: command,
				Type:    raft.NOOPEntry,
				Id:      c.Id,
				USN:     c.USN,
			}, getStateResonse)
		if err != nil {
			log.Printf("Failed to call GetStateRPC: %v", err)
			reply.Success = false
			return nil
		}
		if !getStateResonse.Success {
			// the read failed, retry
			log.Printf("Failed to read from server %s, retrying", args.Server)
			args.Server = string(getStateResonse.Leader)
			continue
		}
		failRead = true
	}
	// prepare the message from the result of the getStateRPC
	var getStateResonseData GetStateResult
	err = json.Unmarshal(getStateResonse.Data, &getStateResonseData)
	if err != nil {
		log.Printf("Failed to unmarshal messages: %v", err)
		reply.Success = false
		return nil
	}

	// add the messages to the group
	c.groups[args.Group] = append(c.groups[args.Group], getStateResonseData.Messages...)
	c.LastRequestID = args.RequestId
	c.USN++
	log.Println(c.groups[args.Group])

	// send the messages to the frontend
	for _, message := range getStateResonseData.Messages {
		c.messageCh <- message
	}

	reply.Success = true
	return nil
}
