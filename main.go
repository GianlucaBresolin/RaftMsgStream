package main

import (
	"RaftMsgStream/raft"
	"encoding/json"
	"log"
	"net"
	"net/rpc"
	"time"
)

func clientConnection(node raft.ServerID, port raft.Port) *rpc.Client {
	client, err := rpc.Dial("tcp", "localhost"+string(port))
	if err != nil {
		log.Printf("Failed to dial %s: %v", node, err)
	} else {
		log.Printf("Client has connected to %s", node)
	}
	return client
}

type ClientEndpoint struct {
	Id   string
	Port string
}

func (c *ClientEndpoint) RegisterClient() {
	server := rpc.NewServer()
	err := server.Register(c)
	if err != nil {
		log.Fatalf("Failed to register client %s: %v", c.Id, err)
	}

	listener, err := net.Listen("tcp", c.Port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", c.Port, err)
	}
	log.Printf("Client %s is listening on %s\n", c.Id, c.Port)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Error accepting connection: %v", err)
				continue
			}
			go server.ServeConn(conn)
		}
	}()
}

type GetResponseArgs struct {
	Success bool
}

type GetResponseResult struct {
}

func (c *ClientEndpoint) GetResponseRPC(res GetResponseArgs, reply *GetResponseResult) error {
	log.Println("Client received response with args: ", res)
	return nil
}

func main() {

	//node1 := raft.NewNode("node1", ":5001", map[raft.ServerID]raft.Port{})
	node1 := raft.NewNode("node1", ":5001", map[raft.ServerID]raft.Port{
		"node2": ":5002",
		"node3": ":5003"},
		false)
	node2 := raft.NewNode("node2", ":5002", map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node3": ":5003"},
		false)
	node3 := raft.NewNode("node3", ":5003", map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node2": ":5002"},
		false)

	nodeMap := map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node2": ":5002",
		"node3": ":5003",
	}

	node1.RegisterNode()
	node2.RegisterNode()
	node3.RegisterNode()

	node1.PrepareConnections()
	node2.PrepareConnections()
	node3.PrepareConnections()

	go node1.Run()
	go node2.Run()
	go node3.Run()

	successRequests := 0
	client := clientConnection("node1", ":5001")
	clientEndpoint := ClientEndpoint{"client1", ":5004"}
	clientEndpoint.RegisterClient()

	for successRequests != 2 {
		time.Sleep(1 * time.Second)
		command := map[string]string{"message": "hello"}
		data, _ := json.Marshal(command)
		args := raft.ClientRequestArguments{
			Command: data,
			Type:    raft.ActionEntry,
			Id:      "client1",
			USN:     successRequests,
		}
		var reply raft.ClientRequestResult
		err := client.Call("Node.ClientRequestRPC", args, &reply)

		if err != nil {
			log.Printf("Failed to call ClientRequestRPC: %v", err)
			time.Sleep(1 * time.Second)
		} else {
			if reply.Success {
				successRequests++
				continue
			}
			time.Sleep(1 * time.Second)
			if reply.Leader == "" {
				continue
			}
			client.Close()
			client = clientConnection(reply.Leader, nodeMap[reply.Leader])
		}
	}

	node4 := raft.NewNode("node4", ":5005", map[raft.ServerID]raft.Port{
		"node1": ":5001",
		"node2": ":5002",
		"node3": ":5003"},
		true)
	node4.RegisterNode()
	node4.PrepareConnections()
	go node4.Run()

	successRequests = 0
	for successRequests != 1 {
		time.Sleep(1 * time.Second)
		command := map[string]map[string]string{
			"newConfig": {
				"node1": ":5001",
				"node2": ":5002",
				"node3": ":5003",
				"node4": ":5005",
			}}
		data, _ := json.Marshal(command)
		args := raft.ClientRequestArguments{
			Command: data,
			Type:    raft.ConfigurationEntry,
			Id:      "client1",
			USN:     3,
		}
		var reply raft.ClientRequestResult
		err := client.Call("Node.ClientRequestRPC", args, &reply)

		if err != nil {
			log.Printf("Failed to call ClientRequestRPC: %v", err)
			time.Sleep(1 * time.Second)
		} else {
			if reply.Success {
				successRequests++
				continue
			}
			time.Sleep(1 * time.Second)
			if reply.Leader == "" {
				continue
			}
			client.Close()
			client = clientConnection(reply.Leader, nodeMap[reply.Leader])
		}
	}

	for {
	}
}
