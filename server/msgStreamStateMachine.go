package server

import (
	"RaftMsgStream/models"
	"encoding/json"
	"log"
	"net/rpc"
)

type command struct {
	Username      string `json:"user"`
	Userport      string `json:"port"`
	Group         string `json:"group"`
	Msg           string `json:"msg"`
	Partecipation string `json:"partecipation"`
}

type group struct {
	users    map[string]*rpc.Client
	messages []models.Message
}

type msgStreamStateMachine struct {
	serverId  string
	commandCh chan []byte
	groups    map[string]*group
}

func newMsgStreamStateMachine(serverId string) *msgStreamStateMachine {
	return &msgStreamStateMachine{
		serverId:  serverId,
		commandCh: make(chan []byte),
		groups:    make(map[string]*group),
	}
}

func (m *msgStreamStateMachine) handleMsgStreamStateMachine() {
	for {
		select {
		case command := <-m.commandCh:
			m.applyCommand(command)
		}
	}
}

func (m *msgStreamStateMachine) applyCommand(c []byte) {
	command := command{}
	err := json.Unmarshal(c, &command)
	if err != nil {
		log.Printf("Error unmarshalling command %v", err)
		return
	}

	// apply the command
	switch command.Partecipation {
	case "true":
		// check if the group exists, otherwise create it
		_, okG := m.groups[command.Group]
		if !okG {
			m.groups[command.Group] = &group{
				users:    make(map[string]*rpc.Client),
				messages: make([]models.Message, 0),
			}
		}

		// check if the user is already in the group, otherwise add it
		_, okU := m.groups[command.Group].users[command.Username]
		if !okU {
			// establish a connection with the user
			client, err := rpc.Dial("tcp", "localhost"+string(command.Userport))
			if err != nil {
				log.Printf("Failed to dial: %v", err)
			}
			m.groups[command.Group].users[command.Username] = client
		}

		// add the message (if present) to the group
		if command.Msg != "" {
			m.groups[command.Group].messages = append(m.groups[command.Group].messages, models.Message{
				Username: command.Username,
				Msg:      command.Msg})
		}

		// notify all the users in the group
		for _, user := range m.groups[command.Group].users {
			success := false
			for !success {
				updateResult := &models.UpdateResult{}
				err := user.Call("Client.UpdateRPC",
					models.UpdateARgs{
						Server: m.serverId,
						Group:  command.Group,
					}, updateResult)
				if err != nil {
					log.Printf("Failed to call Update: %v", err)
				}
				log.Println(updateResult)
				success = updateResult.Success
			}
		}
	case "false":
		// remove the user from the group
		delete(m.groups[command.Group].users, command.Username)
	}
}
