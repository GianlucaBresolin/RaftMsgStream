package server

import (
	"RaftMsgStream/models"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/rpc"

	"RaftMsgStream/server/protoServer"

	"google.golang.org/protobuf/proto"
)

type command struct {
	Username      string `json:"user"`
	Userport      string `json:"port"`
	Group         string `json:"group"`
	Msg           string `json:"msg"`
	Partecipation string `json:"partecipation"`
}

type userInfo struct {
	port       string
	connection *rpc.Client
}

type group struct {
	users    map[string]userInfo
	messages []models.Message
}

type msgStreamStateMachine struct {
	serverId           string
	commandCh          chan []byte
	snapshotRequestCh  chan struct{}
	snapshotResponseCh chan []byte
	applySnapshotCh    chan []byte
	groups             map[string]*group
}

func newMsgStreamStateMachine(serverId string, commandCh chan []byte, snapshotRequestCh chan struct{}, snapshotResponde chan []byte, applySnapshotCh chan []byte) *msgStreamStateMachine {
	return &msgStreamStateMachine{
		serverId:           serverId,
		commandCh:          commandCh,
		applySnapshotCh:    applySnapshotCh,
		snapshotRequestCh:  snapshotRequestCh,
		snapshotResponseCh: snapshotResponde,
		groups:             make(map[string]*group),
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
				users:    make(map[string]userInfo),
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
			m.groups[command.Group].users[command.Username] = userInfo{
				port:       command.Userport,
				connection: client,
			}
		}

		// add the message (if present) to the group
		if command.Msg != "" {
			m.groups[command.Group].messages = append(m.groups[command.Group].messages, models.Message{
				Username: command.Username,
				Msg:      command.Msg})
		}

		// notify all the users in the group
		// for _, user := range m.groups[command.Group].users {
		// 	success := false
		// 	for !success {
		// 		updateResult := &models.UpdateResult{}
		// 		err := user.Call("Client.UpdateRPC",
		// 			models.UpdateARgs{
		// 				Server: m.serverId,
		// 				Group:  command.Group,
		// 			}, updateResult)
		// 		if err != nil {
		// 			log.Printf("Failed to call Update: %v", err)
		// 		}
		// 		log.Println(updateResult)
		// 		success = updateResult.Success
		// 	}
		// }
	case "false":
		// remove the user from the group
		delete(m.groups[command.Group].users, command.Username)
	}
}

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	// write the data to the writer
	_, err := gzWriter.Write(data)
	if err != nil {
		return nil, err
	}

	// close the writer
	err = gzWriter.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func gzipDecompress(compressedData []byte) ([]byte, error) {
	buf := bytes.NewReader(compressedData)
	gzReader, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	// Legge tutti i dati decompressi
	decompressedData, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}

	return decompressedData, nil
}

func (m *msgStreamStateMachine) handleMsgStreamStateMachine() {
	for {
		select {
		case command := <-m.commandCh:
			m.applyCommand(command)
		case <-m.snapshotRequestCh:
			// create the snapshot
			// generate the proto object of the state
			groups := &protoServer.MsgStreamStateMachine{
				Groups: make([]*protoServer.Group, 0),
			}

			for groupName, group := range m.groups {
				groupProto := &protoServer.Group{
					GroupName: groupName,
					Users:     make([]*protoServer.User, 0),
					Messages:  make([]*protoServer.Message, 0),
				}

				for username, _ := range group.users {
					groupProto.Users = append(groupProto.Users, &protoServer.User{
						Username: username,
						Port:     group.users[username].port,
					})
				}

				for _, message := range group.messages {
					groupProto.Messages = append(groupProto.Messages, &protoServer.Message{
						Username: message.Username,
						Msg:      message.Msg,
					})
				}

				groups.Groups = append(groups.Groups, groupProto)
			}
			data, err := proto.Marshal(groups)
			if err != nil {
				log.Printf("Error marshalling snapshot %v", err)
				m.snapshotResponseCh <- nil
				continue
			}

			compressedSnapshot, err := gzipCompress(data)
			if err != nil {
				log.Printf("Error compressing snapshot %v", err)
				m.snapshotResponseCh <- nil
				continue
			}
			m.snapshotResponseCh <- compressedSnapshot
		case snapshot := <-m.applySnapshotCh:
			// apply the sended snapshot
			decompressedSnapshot, err := gzipDecompress(snapshot)
			if err != nil {
				log.Printf("Error decompressing snapshot %v", err)
				continue
			}
			groups := &protoServer.MsgStreamStateMachine{}
			err = proto.Unmarshal(decompressedSnapshot, groups)
			if err != nil {
				log.Printf("Error unmarshalling snapshot %v", err)
				continue
			}
			// close the current connections
			for _, group := range m.groups {
				for _, user := range group.users {
					user.connection.Close()
				}
			}
			// reset the state
			m.groups = make(map[string]*group)
			// rebuild the state from the snapshot
			for _, groupProto := range groups.Groups {
				group := &group{
					users:    make(map[string]userInfo),
					messages: make([]models.Message, 0),
				}
				for _, user := range groupProto.Users {
					// establish a connection with the user
					client, err := rpc.Dial("tcp", "localhost"+user.Port)
					if err != nil {
						log.Printf("Failed to dial: %v", err)
					}
					group.users[user.Username] = userInfo{
						port:       user.Port,
						connection: client,
					}
				}
				for _, message := range groupProto.Messages {
					group.messages = append(group.messages, models.Message{
						Username: message.Username,
						Msg:      message.Msg,
					})
				}
				m.groups[groupProto.GroupName] = group
			}
		}
	}
}
