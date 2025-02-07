package client

import (
	"RaftMsgStream/models"
	"RaftMsgStream/raft"
	"encoding/json"
	"log"
	"time"
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
	Username         string
	Group            string
	LastMessageIndex uint
}

type GetStateResult struct {
	Messages   []models.Message
	Membership bool
}

func (c *Client) UpdateRPC(args UpdateARgs, reply *UpdateResult) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// if we have almost an unvoting server, we wants to update by it
	if len(c.UnvotingServers) > 0 {
		_, ok := c.UnvotingServers[args.Server]
		if !ok {
			reply.Success = false
			return nil
		}
	}

	// check if the update is stale
	if args.RequestId <= c.LastRequestID {
		reply.Success = true
		return nil
	}

	// build the command for the read
	getStateArgs := &GetStateArgs{
		Username:         c.Id,
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
		done := make(chan error, 1)
		timeout := time.NewTimer(20 * time.Millisecond)

		go func() {
			log.Println("the client is reading the state from the server", args.Server)
			done <- c.Connections[args.Server].Call("RaftNode.GetStateRPC",
				raft.ClientRequestArguments{
					Command: command,
					Type:    raft.NOOPEntry,
					Id:      c.Id,
					USN:     c.USN,
				}, getStateResonse)
		}()

		select {
		case err := <-done:
			if err != nil {
				log.Printf("Failed to call GetStateRPC: %v", err)
				reply.Success = false
				return nil
			}
			if !getStateResonse.Success {
				// the read failed, retry
				log.Printf("Failed to read from server %s, retrying with the new leader", args.Server)
				args.Server = string(getStateResonse.Leader)
				continue
			}
			failRead = true
		case <-timeout.C:
			log.Println("Timeout reading from server", args.Server, "retrying...")
		}
	}
	// prepare the message from the result of the getStateRPC
	var getStateResonseData GetStateResult
	err = json.Unmarshal(getStateResonse.Data, &getStateResonseData)
	if err != nil {
		log.Printf("Failed to unmarshal messages: %v", err)
		reply.Success = false
		return nil
	}

	if !getStateResonseData.Membership {
		// we left the group
		delete(c.groups, args.Group)
		c.LastRequestID = args.RequestId
		log.Printf("Left group %s", args.Group)
	} else {
		// add the messages to the group
		c.groups[args.Group] = append(c.groups[args.Group], getStateResonseData.Messages...)
		c.LastRequestID = args.RequestId
	}
	c.USN++

	// send the messages to the frontend
	for _, message := range getStateResonseData.Messages {
		c.MessageCh <- message
	}

	reply.Success = true
	return nil
}
