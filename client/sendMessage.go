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
		done := make(chan error, 1)
		timeout := time.NewTimer(20 * time.Millisecond)

		go func() {
			done <- c.Connections[leader].Call("RaftNode.ActionRequestRPC", args, &reply)
		}()

		select {
		case err := <-done:
			if err != nil {
				log.Printf("Failed to call ActionRPC: %v", err)
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
