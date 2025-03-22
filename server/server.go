package server

import (
	"RaftMsgStream/models"
	"RaftMsgStream/raft"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

type Server struct {
	address      string
	clients      map[string]gin.ResponseWriter
	eventCh      chan models.Event
	raftNode     *raft.RaftNode
	stateMachine *msgStreamStateMachine
}

func NewServer(id raft.ServerID, address string, peers map[raft.ServerID]raft.Address, unvoting bool) *Server {
	server := rpc.NewServer()

	raftNodeAddress := raft.Address(address + ":" + os.Getenv("RAFT_PORT"))
	raftNode := raft.NewRaftNode(id, raftNodeAddress, server, peers, unvoting)
	eventCh := make(chan models.Event)
	return &Server{
		address:      address,
		clients:      make(map[string]gin.ResponseWriter),
		eventCh:      eventCh,
		raftNode:     raftNode,
		stateMachine: newMsgStreamStateMachine(string(id), eventCh, raftNode.CommitCh, raftNode.SnapshotRequestCh, raftNode.SnapshotResponseCh, raftNode.ApplySnapshotCh, raftNode.ReadStateCh, raftNode.ReadStateResultCh),
	}
}

func (s *Server) PrepareConnectionsWithOtherServers() {
	connected := false
	for !connected {
		err := s.raftNode.PrepareConnections()
		if err != nil {
			log.Println("Failed to connect to other nodes, retrying in 1 second")
			time.Sleep(1 * time.Second)
			continue
		}
		connected = true
	}
}

func (s *Server) Run() {
	go s.raftNode.HandleRaftNode()
	go s.stateMachine.handleMsgStreamStateMachine()

	// Start the Gin router
	router := gin.Default()
	router.LoadHTMLGlob("./templates/*")
	router.Static("./static", "./static")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// API to send a message to a group or join a group (If there are no messages to send)
	router.POST("/send", func(c *gin.Context) {
		var request struct {
			User    string `json:"user"`
			USN     int    `json:"USN"`
			Group   string `json:"group"`
			Message string `json:"message"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		command := map[string]string{
			"user":          request.User,
			"group":         request.Group,
			"msg":           request.Message,
			"partecipation": "true",
		}
		jsonCommand, _ := json.Marshal(command)
		clientRequest := models.ClientActionArguments{
			Command: jsonCommand,
			Type:    raft.ActionEntry,
			Id:      request.User,
			USN:     request.USN,
		}
		var clientRequestResult models.ClientActionResult

		if err := s.actionRequest(clientRequest, &clientRequestResult); err == nil {
			if clientRequestResult.Success {
				c.JSON(http.StatusOK, gin.H{"status": "message sent"})
			} else {
				c.JSON(http.StatusOK, gin.H{
					"status": "reditecting to leader",
					"leader": clientRequestResult.Leader, //TODO: rimuovo porta
				})
			}
		} else {
			c.JSON(http.StatusOK, gin.H{"status": "failed to send message"})
		}
	})

	// API to leave a group
	router.POST("/leave", func(c *gin.Context) {
		var request struct {
			User  string `json:"user"`
			USN   int    `json:"USN"`
			Group string `json:"group"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		command := map[string]string{
			"user":          request.User,
			"group":         request.Group,
			"partecipation": "false",
		}
		jsonCommand, _ := json.Marshal(command)
		clientRequest := models.ClientActionArguments{
			Command: jsonCommand,
			Type:    raft.ActionEntry,
			Id:      request.User,
			USN:     request.USN,
		}
		var clientRequestResult models.ClientActionResult

		if err := s.actionRequest(clientRequest, &clientRequestResult); err != nil {
			if clientRequestResult.Success {
				c.JSON(http.StatusOK, gin.H{"status": "left group"})
			} else {
				c.JSON(http.StatusOK, gin.H{
					"status": "reditecting to leader",
					"leader": clientRequestResult.Leader, //TODO: rimuovo porta
				})
			}
		} else {
			c.JSON(http.StatusOK, gin.H{"status": "failed to leave group"})
		}
	})

	// API to subscribe for notifications
	router.POST("/subscribe", func(c *gin.Context) {
		var request struct {
			User string `json:"user"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
		user := request.User
		s.clients[user] = c.Writer

		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/events?user=%s", user))

		c.JSON(http.StatusOK, gin.H{"status": "subscribed"})
	})

	// API to unsubscribe for notifications
	router.POST("/unsubscribe", func(c *gin.Context) {
		var request struct {
			User string `json:"user"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
		user := request.User
		<-s.clients[user].CloseNotify()

		c.JSON(http.StatusOK, gin.H{"status": "unsubscribed"})
	})

	// API to get notifications
	router.GET("/events", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		user := c.DefaultQuery("user", "")
		if _, ok := s.clients[user]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "User not subscribed"})
			return
		}

		for {
			select {
			case event := <-s.eventCh:
				if _, ok := event.Users[user]; ok {
					c.SSEvent("message", event.Msg)
				}
			case <-c.Writer.CloseNotify():
				delete(s.clients, user)
				return
			}
		}
	})

	// API to update the client's state
	router.POST("/update", func(c *gin.Context) {
		var request struct {
			User             string `json:"user"`
			USN              int    `json:"USN"`
			LastMessageIndex int    `json:"lastMessageIndex"`
			Group            string `json:"group"`
		}

		getStateArgs := models.GetStateArgs{
			Username:         request.User,
			Group:            request.Group,
			LastMessageIndex: uint(request.LastMessageIndex),
		}

		jsonCommand, _ := json.Marshal(getStateArgs)

		clientRequestArguments := models.ClientGetStateArguments{
			Command: jsonCommand,
			Id:      request.User,
			USN:     request.USN,
		}
		var clientRequestResult models.ClientGetStateResult

		if err := s.getState(clientRequestArguments, &clientRequestResult); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"status": "state updated",
				"state":  clientRequestResult.Data,
			})
		} else {
			if clientRequestResult.Leader != "" {
				c.JSON(http.StatusOK, gin.H{
					"status": "reditecting to leader",
					"leader": clientRequestResult.Leader,
				})
			} else {
				c.JSON(http.StatusOK, gin.H{"status": "failed to update state"})
			}
		}
	})

	go func() {
		if err := router.Run(s.address + ":" + os.Getenv("SERVER_PORT")); err != nil {
			log.Fatalf("Failed to start the Gin router: %v", err)
		}
	}()

	select {}
}

func (s *Server) Close() {
	s.raftNode.ShutdownCh <- struct{}{}
	s.stateMachine.shutdownCh <- struct{}{}
}
