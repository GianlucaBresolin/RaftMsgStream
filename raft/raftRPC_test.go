package raft

import (
	"testing"
	"time"
)

func TestRequestVoteRPCSuccess(t *testing.T) {
	nodeCandidate := NewRaftNode("Server1", ":5001", map[ServerID]Port{"Server2": ":5002"}, false)
	nodeCandidate.term = 1
	nodeCandidate.state = Candidate

	args := RequestVoteArguments{
		Term:         1,
		CandidateId:  "Server1",
		LastLogIndex: 1,
		LastLogTerm:  1,
	}

	var res RequestVoteResult

	otherNode := NewRaftNode("Server2", ":5002", map[ServerID]Port{"Server1": ":5001"}, false)
	otherNode.term = 0
	otherNode.state = Follower
	otherNode.myVote = ""

	nodeCandidate.PrepareConnections()
	otherNode.PrepareConnections()

	nodeCandidate.startTimer()
	otherNode.startTimer()

	time.Sleep(MinElectionTimeout * time.Millisecond)

	err := nodeCandidate.RequestVoteRPC(args, &res)
	if err != nil {
		t.Errorf("Error in RequestVoteRPC: %v", err)
	}

	if res.Term != 1 {
		t.Errorf("Expected term 1, got %v", res.Term)
	}

	if !res.VoteGranted {
		t.Errorf("Expected vote granted, got %v", res.VoteGranted)
	}
}

func TestRequestVoteRPCStaleTerm(t *testing.T) {
	nodeCandidate := NewRaftNode("Server1", ":5001", map[ServerID]Port{"Server2": ":5002"}, false)
	nodeCandidate.term = 1
	nodeCandidate.state = Candidate

	args := RequestVoteArguments{
		Term:         0,
		CandidateId:  "Server1",
		LastLogIndex: 1,
		LastLogTerm:  1,
	}

	var res RequestVoteResult

	otherNode := NewRaftNode("Server2", ":5002", map[ServerID]Port{"Server1": ":5001"}, false)
	otherNode.term = 0
	otherNode.state = Follower
	otherNode.myVote = ""

	nodeCandidate.PrepareConnections()
	otherNode.PrepareConnections()

	nodeCandidate.startTimer()
	otherNode.startTimer()

	time.Sleep(MinElectionTimeout * time.Millisecond)

	err := nodeCandidate.RequestVoteRPC(args, &res)
	if err != nil {
		t.Errorf("Error in RequestVoteRPC: %v", err)
	}

	if res.Term != 1 {
		t.Errorf("Expected term 1, got %v", res.Term)
	}

	if res.VoteGranted {
		t.Errorf("Expected vote not granted, got %v", res.VoteGranted)
	}
}

func TestRequestVoteRPCAlreadyVoted(t *testing.T) {
	nodeCandidate := NewRaftNode("Server1", ":5001", map[ServerID]Port{"Server2": ":5002"}, false)
	nodeCandidate.term = 1
	nodeCandidate.state = Candidate
	nodeCandidate.myVote = "Server2"

	args := RequestVoteArguments{
		Term:         1,
		CandidateId:  "Server1",
		LastLogIndex: 1,
		LastLogTerm:  1,
	}

	var res RequestVoteResult

	otherNode := NewRaftNode("Server2", ":5002", map[ServerID]Port{"Server1": ":5001"}, false)
	otherNode.term = 0
	otherNode.state = Follower
	otherNode.myVote = ""

	nodeCandidate.PrepareConnections()
	otherNode.PrepareConnections()

	nodeCandidate.startTimer()
	otherNode.startTimer()

	time.Sleep(MinElectionTimeout * time.Millisecond)

	err := nodeCandidate.RequestVoteRPC(args, &res)
	if err != nil {
		t.Errorf("Error in RequestVoteRPC: %v", err)
	}

	if res.Term != 1 {
		t.Errorf("Expected term 1, got %v", res.Term)
	}

	if res.VoteGranted {
		t.Errorf("Expected vote not granted, got %v", res.VoteGranted)
	}
}

func TestAppendEntriesRPCSuccess(t *testing.T) {
	nodeFollower := NewRaftNode("Server1", ":5001", map[ServerID]Port{"Server2": ":5002"}, false)
	nodeFollower.term = 1

	leaderNode := NewRaftNode("Server2", ":5002", map[ServerID]Port{"Server1": ":5001"}, false)
	leaderNode.term = 1
	leaderNode.state = Leader

	args := AppendEntriesArguments{
		Term:             1,
		LeaderId:         "Server2",
		PreviousLogIndex: uint(0),
		PreviousLogTerm:  uint(0),
		Entries: []LogEntry{{
			Term:    1,
			Index:   1,
			Command: []byte("test"),
			Type:    ActionEntry,
			Client:  "Server2",
			USN:     1,
		}, {
			Term:    1,
			Index:   2,
			Command: []byte("test2"),
			Type:    ActionEntry,
			Client:  "Server2",
			USN:     2,
		}},
		LeaderCommit: 1,
	}

	var res AppendEntriesResult

	nodeFollower.PrepareConnections()
	leaderNode.PrepareConnections()

	nodeFollower.startTimer()
	leaderNode.startTimer()

	go func() {
		<-nodeFollower.CommitCh
	}()

	err := leaderNode.peersConnection["Server1"].Call("RaftNode.AppendEntriesRPC", args, &res)

	if err != nil {
		t.Errorf("Error in AppendEntriesRPC: %v", err)
	}

	if res.Term != 1 {
		t.Errorf("Expected term 1, got %v", res.Term)
	}

	if !res.Success {
		t.Errorf("Expected success, got %v", res.Success)
	}

	if len(nodeFollower.log.entries) != 3 {
		t.Errorf("Expected log length 1, got %v", len(nodeFollower.log.entries))
	}

	if nodeFollower.log.entries[1].Index != 1 {
		t.Errorf("Expected log index 1, got %v", nodeFollower.log.entries[1].Index)
	}

	if nodeFollower.log.entries[1].Term != 1 {
		t.Errorf("Expected log term 1, got %v", nodeFollower.log.entries[1].Term)
	}

	if nodeFollower.log.lastCommitedIndex != 1 {
		t.Errorf("Expected last committed index 1, got %v", nodeFollower.log.lastCommitedIndex)
	}
}

func TestAppendEntriesRPCStaleTerm(t *testing.T) {
	nodeFollower := NewRaftNode("Server1", ":5001", map[ServerID]Port{"Server2": ":5002"}, false)
	nodeFollower.term = 1

	leaderNode := NewRaftNode("Server2", ":5002", map[ServerID]Port{"Server1": ":5001"}, false)
	leaderNode.term = 0
	leaderNode.state = Leader

	args := AppendEntriesArguments{
		Term:             0,
		LeaderId:         "Server2",
		PreviousLogIndex: uint(0),
		PreviousLogTerm:  uint(0),
		Entries: []LogEntry{{
			Term:    1,
			Index:   1,
			Command: []byte("test"),
			Type:    ActionEntry,
			Client:  "Server2",
			USN:     1,
		}},
		LeaderCommit: 0,
	}

	var res AppendEntriesResult

	nodeFollower.PrepareConnections()
	leaderNode.PrepareConnections()

	nodeFollower.startTimer()
	leaderNode.startTimer()

	err := leaderNode.peersConnection["Server1"].Call("RaftNode.AppendEntriesRPC", args, &res)
	if err != nil {
		t.Errorf("Error in AppendEntriesRPC: %v", err)
	}

	if res.Term != 1 {
		t.Errorf("Expected term 1, got %v", res.Term)
	}

	if res.Success {
		t.Errorf("Expected not success, got %v", res.Success)
	}
}

func TestInstallSnapshotRPCSuccess(t *testing.T) {
	nodeFollower := NewRaftNode("Server1", ":5001", map[ServerID]Port{"Server2": ":5002"}, false)
	nodeFollower.term = 1

	leaderNode := NewRaftNode("Server2", ":5002", map[ServerID]Port{"Server1": ":5001"}, false)
	leaderNode.term = 1
	leaderNode.state = Leader
	leaderNode.snapshot = &Snapshot{
		LastIndex:        snapshotThreshold,
		LastTerm:         1,
		LastConfig:       leaderNode.snapshot.LastConfig,
		StateMachineSnap: []byte("test"),
	}

	var res InstallSnapshotResult

	nodeFollower.PrepareConnections()
	leaderNode.PrepareConnections()

	nodeFollower.startTimer()
	leaderNode.startTimer()

	go func() {
		<-nodeFollower.ApplySnapshotCh
	}()

	err := leaderNode.peersConnection["Server1"].Call("RaftNode.InstallSnapshotRPC", &InstallSnapshotArguments{
		Term:              leaderNode.term,
		LeaderId:          leaderNode.id,
		LastIncludedIndex: leaderNode.snapshot.LastIndex,
		LastIncludedTerm:  leaderNode.snapshot.LastTerm,
		LastConfig:        leaderNode.snapshot.LastConfig,
		LastUSNof:         leaderNode.snapshot.LastUSNof,
		Offset:            0,
		Data:              leaderNode.snapshot.StateMachineSnap,
		Done:              true,
	}, &res)

	if err != nil {
		t.Errorf("Error in InstallSnapshotRPC: %v", err)
	}

	if res.Term != 1 {
		t.Errorf("Expected term 1, got %v", res.Term)
	}

	if !res.Success {
		t.Errorf("Expected success, got %v", res.Success)
	}

	if len(nodeFollower.log.entries) != 1 {
		t.Errorf("Expected log length 1, got %v", len(nodeFollower.log.entries))
	}

	if nodeFollower.lastGlobalIndex() != snapshotThreshold {
		t.Errorf("Expected log index %v, got %v", snapshotThreshold, nodeFollower.lastGlobalIndex())
	}
}

func TestInstallSnapshotRPCStaleTerm(t *testing.T) {
	nodeFollower := NewRaftNode("Server1", ":5001", map[ServerID]Port{"Server2": ":5002"}, false)
	nodeFollower.term = 1

	leaderNode := NewRaftNode("Server2", ":5002", map[ServerID]Port{"Server1": ":5001"}, false)
	leaderNode.term = 0
	leaderNode.state = Leader
	leaderNode.snapshot = &Snapshot{
		LastIndex:        snapshotThreshold,
		LastTerm:         1,
		LastConfig:       leaderNode.snapshot.LastConfig,
		StateMachineSnap: []byte("test"),
	}

	var res InstallSnapshotResult

	nodeFollower.PrepareConnections()
	leaderNode.PrepareConnections()

	nodeFollower.startTimer()
	leaderNode.startTimer()

	err := leaderNode.peersConnection["Server1"].Call("RaftNode.InstallSnapshotRPC", &InstallSnapshotArguments{
		Term:              leaderNode.term,
		LeaderId:          leaderNode.id,
		LastIncludedIndex: leaderNode.snapshot.LastIndex,
		LastIncludedTerm:  leaderNode.snapshot.LastTerm,
		LastConfig:        leaderNode.snapshot.LastConfig,
		LastUSNof:         leaderNode.snapshot.LastUSNof,
		Offset:            0,
		Data:              leaderNode.snapshot.StateMachineSnap,
		Done:              true,
	}, &res)

	if err != nil {
		t.Errorf("Error in InstallSnapshotRPC: %v", err)
	}

	if res.Term != 1 {
		t.Errorf("Expected term 1, got %v", res.Term)
	}

	if res.Success {
		t.Errorf("Expected not success, got %v", res.Success)
	}
}
