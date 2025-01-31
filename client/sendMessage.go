package client

import (
	"RaftMsgStream/raft"
	"encoding/json"
	"log"
	"time"
)

func (c *Client) SendMessage(group string, msg string) {
	success := false
	command := map[string]string{
		"user":          c.Id,
		"port":          c.Port,
		"group":         group,
		"msg":           msg,
		"partecipation": "true",
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
		err := c.Connections[leader].Call("RaftNode.ClientRequestRPC", args, &reply)
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
		time.Sleep(1 * time.Second)
	}
}
