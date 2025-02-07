package benchmark

import (
	"RaftMsgStream/client"
	"RaftMsgStream/models"
	"RaftMsgStream/raft"
	"RaftMsgStream/server"
	"strconv"
	"sync"
	"testing"
)

const clusterSize = 20
const userSize = 1

var (
	cluster     map[raft.ServerID]*server.Server
	clientsNode []*client.Client
	once        sync.Once
	messageCh   chan models.Message
)

func setupCluster() {
	cluster = make(map[raft.ServerID]*server.Server)

	for i := 1; i <= clusterSize; i++ {
		otherNodes := make(map[raft.ServerID]raft.Port)
		for j := 1; j <= clusterSize; j++ {
			if i != j {
				otherNodes[raft.ServerID("node"+strconv.Itoa(j))] = raft.Port(":500" + strconv.Itoa(j))
			}
		}
		cluster[raft.ServerID("node"+strconv.Itoa(i))] = server.NewServer(
			raft.ServerID("node"+strconv.Itoa(i)),
			raft.Port(":500"+strconv.Itoa(i)),
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

	setupUsers(userSize)
}

func setupUsers(users int) {
	clientsNode = make([]*client.Client, 0, users)

	for i := 1; i <= users; i++ {
		messageCh = make(chan models.Message)
		clusterNode := make(map[string]string)
		for id := range cluster {
			clusterNode[string(id)] = ":500" + string(id)[4:]
		}

		client := client.NewClient("client"+strconv.Itoa(i), ":600"+strconv.Itoa(i), clusterNode, messageCh)
		clientsNode = append(clientsNode, client)
		client.PrepareConnections()
	}
}

func BenchmarkSingleUserAllVotingsClusterCommitTime(b *testing.B) {
	once.Do(setupCluster)

	go func() {
		for {
			<-clientsNode[0].MessageCh
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clientsNode[0].SendMessage("benchmark test", "this is a benchmark test")
	}
}

func BenchmarkSingleUserAllVotingsClusterWithReadingTime(b *testing.B) {
	once.Do(setupCluster)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clientsNode[0].SendMessage("benchmark test", "this is a benchmark test")
		<-clientsNode[0].MessageCh
	}
}
