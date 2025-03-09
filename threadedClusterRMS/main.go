package main

import (
	"RaftMsgStream/client"
	"RaftMsgStream/models"
	"RaftMsgStream/raft"
	"RaftMsgStream/server"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Error upgrading to WebSocket:", err)
		return
	}
	defer conn.Close()
	log.Println("WebSocket connection established")

	for {
		select {
		case message := <-messageCh:
			jsonMessage, err := json.Marshal(message)
			if err != nil {
				log.Println("Error marshalling message:", err)
				return
			}
			errW := conn.WriteMessage(websocket.TextMessage, []byte(jsonMessage))
			if errW != nil {
				log.Println("Error writing to WebSocket:", err)
				return
			}
		}
	}
}

var messageCh = make(chan models.Message)
var fakeMessageCh = make(chan models.Message)

func main() {
	// request the client name to use
	fmt.Println("Enter username:")
	var username string
	fmt.Scanln(&username)

	// ask cluster size
	fmt.Println("Enter the size of the voting cluster:")
	var votingCLusterSize int
	fmt.Scanln(&votingCLusterSize)

	// ask unvoting nodes size
	fmt.Println("Enter the size of the unvoting nodes in the cluster:")
	var unvotingClusterSize int
	fmt.Scanln(&unvotingClusterSize)

	cluster := make(map[raft.ServerID]*server.Server)

	for i := 1; i <= votingCLusterSize; i++ {
		otherNodes := make(map[raft.ServerID]raft.Address)
		for j := 1; j <= votingCLusterSize; j++ {
			if i != j {
				otherNodes[raft.ServerID("node"+strconv.Itoa(j))] = raft.Address("localhost:500" + strconv.Itoa(j))
			}
		}
		cluster[raft.ServerID("node"+strconv.Itoa(i))] = server.NewServer(
			raft.ServerID("node"+strconv.Itoa(i)),
			raft.Address("localhost:500"+strconv.Itoa(i)),
			otherNodes,
			false,
		)
	}

	for _, node := range cluster {
		node.PrepareConnectionsWithOtherServers()
	}

	for _, node := range cluster {
		go node.Run()
	}

	unvotingNodes := make(map[raft.ServerID]*server.Server)

	for i := votingCLusterSize + 1; i <= (unvotingClusterSize + votingCLusterSize); i++ {
		otherNodes := make(map[raft.ServerID]raft.Address)
		for j := 1; j <= votingCLusterSize; j++ {
			if i != j {
				otherNodes[raft.ServerID("node"+strconv.Itoa(j))] = raft.Address("localhost:500" + strconv.Itoa(j))
			}
		}
		unvotingNodes[raft.ServerID("node"+strconv.Itoa(i))] = server.NewServer(
			raft.ServerID("node"+strconv.Itoa(i)),
			raft.Address("localhost:500"+strconv.Itoa(i)),
			otherNodes,
			true,
		)
	}

	for _, node := range unvotingNodes {
		node.PrepareConnectionsWithOtherServers()
	}

	serversMap := make(map[string]string)
	for id := range cluster {
		serversMap[string(id)] = ":500" + string(id)[4:]
	}

	// create the user
	user := client.NewClient(username, ":6001", serversMap, messageCh)
	user.PrepareConnections()

	// create the fake user
	fakeUser := client.NewClient("Other User", ":6002", serversMap, fakeMessageCh)
	fakeUser.PrepareConnections()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				fakeUser.SendMessage("RaftMsgStream", "RaftMsgStream")
			case <-fakeMessageCh:
				// drain the channel
			}
		}
	}()

	// start the Gin router
	router := gin.Default()
	router.LoadHTMLGlob("../templates/*")
	router.Static("../static", "./static")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// websocket endpoint to handle received messages
	router.GET("/ws", handleWebSocket)

	// API to get the username
	router.GET("/get-username", func(c *gin.Context) {
		c.JSON(200, gin.H{"value": username})
	})

	// API to get the membership in a group of a user
	router.POST("/get-membership", func(c *gin.Context) {
		var req struct {
			Group string `form:"group"`
		}
		if err := c.ShouldBind(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		membership := user.GetMembership(req.Group)
		c.JSON(http.StatusOK, gin.H{"membership": membership})
	})

	// API to leave a group
	router.POST("/leave", func(c *gin.Context) {
		var req struct {
			Group string `json:"group"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
		user.LeaveGroup(req.Group)
		c.JSON(http.StatusOK, gin.H{"status": "left group"})
	})

	// API to send a message to a group or join a group (if there are no messages to send)
	router.POST("/send", func(c *gin.Context) {
		var req struct {
			Group   string `json:"group"`
			Message string `json:"message"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		user.SendMessage(req.Group, req.Message)
		c.JSON(http.StatusOK, gin.H{"status": "message sent"})
	})

	go func() {
		if err := router.Run(":8080"); err != nil {
			log.Fatalf("Failed to start Gin router: %v", err)
		}
	}()

	for {
	}
}
