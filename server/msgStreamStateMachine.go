package server

import (
	"RaftMsgStream/models"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log"

	"RaftMsgStream/server/protoServer"

	"google.golang.org/protobuf/proto"
)

type command struct {
	Username      string `json:"user"`
	Group         string `json:"group"`
	Msg           string `json:"msg"`
	Partecipation string `json:"partecipation"`
}

type group struct {
	users    map[string]bool
	messages []models.Message
}

type msgStreamStateMachine struct {
	serverId           string
	eventCh            chan models.Event
	shutdownCh         chan struct{}
	commandCh          chan []byte
	readStateCh        chan []byte
	readStateResultCh  chan []byte
	snapshotRequestCh  chan struct{}
	snapshotResponseCh chan []byte
	applySnapshotCh    chan []byte
	groups             map[string]*group
}

func newMsgStreamStateMachine(serverId string, eventCh chan models.Event, commandCh chan []byte, snapshotRequestCh chan struct{}, snapshotResponseCh chan []byte, applySnapshotCh chan []byte, readStateCh chan []byte, readStateResultCh chan []byte) *msgStreamStateMachine {
	return &msgStreamStateMachine{
		serverId:           serverId,
		eventCh:            eventCh,
		shutdownCh:         make(chan struct{}),
		commandCh:          commandCh,
		applySnapshotCh:    applySnapshotCh,
		snapshotRequestCh:  snapshotRequestCh,
		snapshotResponseCh: snapshotResponseCh,
		readStateCh:        readStateCh,
		readStateResultCh:  readStateResultCh,
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
				users:    make(map[string]bool),
				messages: make([]models.Message, 0),
			}
		}

		// check if the user is already in the group, otherwise add it
		_, okU := m.groups[command.Group].users[command.Username]
		if !okU {
			m.groups[command.Group].users[command.Username] = true
		}

		// add the message (if present) to the group
		if command.Msg != "" {
			m.groups[command.Group].messages = append(m.groups[command.Group].messages, models.Message{
				Username: command.Username,
				Msg:      command.Msg})

			// create the event to notify the users in the group
			event := models.Event{
				Msg: models.Message{
					Username: command.Username,
					Msg:      command.Msg,
				},
				Group: command.Group,
				Users: m.groups[command.Group].users,
			}
			m.eventCh <- event
		}
	case "false":
		// remove the user from the group
		delete(m.groups[command.Group].users, command.Username)
		// notify the user that he has been removed
		event := models.Event{
			Users: map[string]bool{command.Username: true},
			Group: command.Group,
		}
		m.eventCh <- event
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
			if command != nil {
				go m.applyCommand(command)
			}
		case command := <-m.readStateCh:
			// read the state providing the info requested by the received command
			getStateArgs := &models.GetStateArgs{}
			err := json.Unmarshal(command, getStateArgs)
			if err != nil {
				log.Printf("Error unmarshalling GetStateArgs %v", err)
				continue
			}
			getStateResult := &models.GetStateResult{
				Messages:   make([]models.Message, 0),
				Membership: false,
			}

			// set the membership of the user in the group required
			_, okU := m.groups[getStateArgs.Group].users[getStateArgs.Username]
			getStateResult.Membership = okU

			// set the messages of the group required
			group, ok := m.groups[getStateArgs.Group]
			if ok {
				getStateResult.Messages = append(getStateResult.Messages, group.messages[getStateArgs.LastMessageIndex+1:]...)
			}

			data, err := json.Marshal(getStateResult)
			if err != nil {
				log.Printf("Error marshalling GetStateResult %v", err)
				continue
			}
			m.readStateResultCh <- data
		case <-m.snapshotRequestCh:
			// create the snapshot
			// generate the proto object of the state
			groups := &protoServer.MsgStreamStateMachine{
				Groups: make([]*protoServer.Group, 0),
			}

			for groupName, group := range m.groups {
				groupProto := &protoServer.Group{
					GroupName: groupName,
					Users:     make([]string, 0),
					Messages:  make([]*protoServer.Message, 0),
				}

				for username := range group.users {
					groupProto.Users = append(groupProto.Users, username)
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
			// reset the state
			m.groups = make(map[string]*group)
			// rebuild the state from the snapshot
			for _, groupProto := range groups.Groups {
				group := &group{
					users:    make(map[string]bool),
					messages: make([]models.Message, 0),
				}
				for _, user := range groupProto.Users {
					group.users[user] = true
				}
				for _, message := range groupProto.Messages {
					group.messages = append(group.messages, models.Message{
						Username: message.Username,
						Msg:      message.Msg,
					})
				}
				m.groups[groupProto.GroupName] = group
			}
		case <-m.shutdownCh:
			close(m.shutdownCh)
			return
		}
	}
}
