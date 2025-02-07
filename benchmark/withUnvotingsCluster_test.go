package benchmark

import (
	"RaftMsgStream/raft"
	"RaftMsgStream/server"
	"strconv"
	"testing"
)

const votingCLusterSize = 4
const unvotingClusterSize = 1
const userSizeWithUnvotings = 1

var (
	unvotingNodes map[raft.ServerID]*server.Server
)

func setupClusterWithUnvotingNodes() {
	cluster = make(map[raft.ServerID]*server.Server)

	for i := 1; i <= votingCLusterSize; i++ {
		otherNodes := make(map[raft.ServerID]raft.Port)
		for j := 1; j <= votingCLusterSize; j++ {
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

	unvotingNodes = make(map[raft.ServerID]*server.Server)

	for i := votingCLusterSize + 1; i <= (unvotingClusterSize + votingCLusterSize); i++ {
		otherNodes := make(map[raft.ServerID]raft.Port)
		for j := 1; j <= votingCLusterSize; j++ {
			if i != j {
				otherNodes[raft.ServerID("node"+strconv.Itoa(j))] = raft.Port(":500" + strconv.Itoa(j))
			}
		}
		unvotingNodes[raft.ServerID("node"+strconv.Itoa(i))] = server.NewServer(
			raft.ServerID("node"+strconv.Itoa(i)),
			raft.Port(":500"+strconv.Itoa(i)),
			otherNodes,
			true,
		)
	}

	for _, node := range unvotingNodes {
		node.PrepareConnectionsWithOtherServers()
	}

	setupUsers(userSizeWithUnvotings)

	for _, client := range clientsNode {
		for unvotingServer, _ := range unvotingNodes {
			client.AddUnvotingServer(string(unvotingServer), ":500"+string(unvotingServer)[4:])
		}
	}

	for _, node := range unvotingNodes {
		go node.Run()
	}
}

func BenchmarkSingleUserWithUnvotingNodesCommitTime(b *testing.B) {
	once.Do(setupClusterWithUnvotingNodes)

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

func BenchmarkSingleUserWithUnvotingNodesWithReadingTime(b *testing.B) {
	once.Do(setupClusterWithUnvotingNodes)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clientsNode[0].SendMessage("benchmark test", "this is a benchmark test")
		<-clientsNode[0].MessageCh
	}
}
