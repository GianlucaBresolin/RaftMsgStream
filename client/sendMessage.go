package client

import (
	"RaftMsgStream/models"
	"RaftMsgStream/raft"
	"encoding/json"
	"log"
	"net/rpc"
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
	args := models.ClientRequestArguments{
		Command: jsonCommand,
		Type:    raft.ActionEntry,
		Id:      c.Id,
		USN:     c.USN,
	}
	var reply models.ClientRequestResult

	var leader string
	for server := range c.Servers {
		leader = server
		break
	}

	for !success {
		done := make(chan *rpc.Call, 1)
		timeout := time.NewTimer(20 * time.Millisecond)

		c.Connections[leader].Go("Server.ActionRequestRPC", args, &reply, done)

		select {
		case call := <-done:
			if call.Error != nil {
				log.Printf("Failed to call ActionRPC: %v", call.Error)
			}
			if reply.Success {
				success = true
			} else {
				if reply.Leader != "" {
					leader = string(reply.Leader)
				}
			}
		case <-timeout.C:
			log.Println("Timeout sending message to node", leader)
		}
	}
	c.USN++
}
