package client

import (
	"RaftMsgStream/raft"
	"encoding/json"
	"log"
)

func (c *Client) GetMembership(group string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	_, ok := c.groups[group]
	return ok
}

func (c *Client) LeaveGroup(group string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	success := false
	command := map[string]string{
		"user":          c.Id,
		"port":          c.Port,
		"group":         group,
		"partecipation": "false",
	}
	jsonCommand, _ := json.Marshal(command)
	args := raft.ClientRequestArguments{
		Command: jsonCommand,
		Type:    raft.ActionEntry,
		Id:      c.Id,
		USN:     c.USN,
	}
	var reply raft.ClientRequestResult

	var leader string
	for server, _ := range c.Servers {
		leader = server
		break
	}

	for !success {
		err := c.Connections[leader].Call("RaftNode.ActionRequestRPC", args, &reply)
		if err != nil {
			log.Printf("Failed to call ClientRequestRPC: %v", err)
		}
		if reply.Success {
			success = true
		} else {
			if reply.Leader != "" {
				leader = string(reply.Leader)
			}
		}
	}
	c.USN++
}
