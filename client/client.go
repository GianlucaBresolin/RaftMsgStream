package client

import (
	"RaftMsgStream/models"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
)

type Client struct {
	Id              string
	Port            string
	USN             int
	LastRequestID   int
	Servers         map[string]string
	UnvotingServers map[string]string
	Connections     map[string]*rpc.Client
	groups          map[string][]models.Message
	MessageCh       chan models.Message
	mutex           sync.Mutex
}

func (c *Client) registerClient() {
	server := rpc.NewServer()
	err := server.Register(c)
	if err != nil {
		log.Fatalf("Failed to register client %s: %v", c.Id, err)
	}

	mux := http.NewServeMux()
	mux.Handle(rpc.DefaultRPCPath, server)

	listener, err := net.Listen("tcp", c.Port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", c.Port, err)
	}
	log.Printf("Client %s is listening on %s\n", c.Id, c.Port)

	go http.Serve(listener, mux)
}

func NewClient(id string, port string, servers map[string]string, messageCh chan models.Message) *Client {
	client := &Client{
		Id:              id,
		Port:            port,
		USN:             0,
		LastRequestID:   -1,
		Servers:         servers,
		UnvotingServers: make(map[string]string),
		Connections:     make(map[string]*rpc.Client),
		groups:          make(map[string][]models.Message),
		MessageCh:       messageCh,
		mutex:           sync.Mutex{},
	}
	client.registerClient()
	return client
}

func (c *Client) PrepareConnections() {
	for server, port := range c.Servers {
		connection, err := rpc.DialHTTP("tcp", "localhost"+port)
		if err != nil {
			log.Printf("Failed to dial %s: %v", server, err)
		} else {
			log.Printf("Client has connected to %s", server)
		}
		c.Connections[server] = connection
	}
}

func (c *Client) AddUnvotingServer(server string, port string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.UnvotingServers[server] = port
	// establish connection
	connection, err := rpc.DialHTTP("tcp", "localhost"+port)
	if err != nil {
		log.Printf("Failed to dial %s: %v", server, err)
	} else {
		c.Connections[server] = connection
	}
}

func (c *Client) RemoveUnvotingServer(server string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.UnvotingServers, server)
	delete(c.Connections, server)
}
