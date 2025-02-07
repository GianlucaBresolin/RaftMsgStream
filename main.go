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

	// create a new server
	msgStreamServer1 := server.NewServer("Server1", ":5001", map[raft.ServerID]raft.Port{"Server2": ":5002", "Server3": ":5003"}, false)
	msgStreamServer2 := server.NewServer("Server2", ":5002", map[raft.ServerID]raft.Port{"Server1": ":5001", "Server3": ":5003"}, false)
	msgStreamServer3 := server.NewServer("Server3", ":5003", map[raft.ServerID]raft.Port{"Server1": ":5001", "Server2": ":5002"}, false)

	// prepare connections with other servers
	msgStreamServer1.PrepareConnectionsWithOtherServers()
	msgStreamServer2.PrepareConnectionsWithOtherServers()
	msgStreamServer3.PrepareConnectionsWithOtherServers()

	// run the server
	go msgStreamServer1.Run()
	go msgStreamServer2.Run()
	go msgStreamServer3.Run()

	user := client.NewClient(username, ":6001", map[string]string{"Server1": ":5001", "Server2": ":5002", "Server3": ":5003"}, messageCh)
	user.PrepareConnections()

	fakeUser := client.NewClient("Other User", ":6002", map[string]string{"Server1": ":5001", "Server2": ":5002", "Server3": ":5003"}, fakeMessageCh)
	fakeUser.PrepareConnections()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
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

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")
	router.Static("/static", "./static")

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
