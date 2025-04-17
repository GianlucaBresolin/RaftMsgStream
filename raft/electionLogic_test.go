package raft

import (
	"net"
	"net/rpc"
	"testing"
	"time"
)

func TestStartElection(t *testing.T) {
	rpcServer := rpc.NewServer()
	raftNode := NewRaftNode("id", ":1233", rpcServer, map[ServerID]Address{"2": "1234", "3": "1235"}, false)
	raftNode.term = 1

	raftNode.startElection()

	if raftNode.term != 2 {
		t.Errorf("Expected term to be 2, got %v", raftNode.term)
	}

	if raftNode.state != Candidate {
		t.Errorf("Expected state to be Candidate, got %v", raftNode.state)
	}
}

func TestWinElection(t *testing.T) {
	rpcServer := rpc.NewServer()
	raftNode := NewRaftNode("id", ":1233", rpcServer, map[ServerID]Address{"2": "1234", "3": "1235"}, false)
	raftNode.term = 1
	raftNode.state = Candidate
	raftNode.log.entries = []LogEntry{
		{Index: 0, Term: 1},
		{Index: 1, Term: 1},
		{Index: 2, Term: 1},
		{Index: 3, Term: 1},
	}
	raftNode.electionTimer = time.NewTimer(time.Second)
	raftNode.minimumTimer = time.NewTimer(time.Second)

	raftNode.winElection()

	if raftNode.state != Leader {
		t.Errorf("Expected state to be Leader, got %v", raftNode.state)
	}

	if raftNode.currentLeader != "id" {
		t.Errorf("Expected currentLeader to be id, got %v", raftNode.currentLeader)
	}

	if raftNode.nextIndex["2"] != 4 {
		t.Errorf("Expected nextIndex[2] to be 4, got %v", raftNode.nextIndex["2"])
	}

	time.Sleep(2 * time.Second) // ensure electionTimer has expired
	select {
	case <-raftNode.electionTimer.C:
		t.Errorf("Expected electionTimer to be stopped")
	default:
	}
}

func TestRevertToFollower(t *testing.T) {
	rpcServer := rpc.NewServer()
	raftNode := NewRaftNode("id", ":1233", rpcServer, map[ServerID]Address{"2": "1236", "3": "1235"}, false)
	raftNode.state = Leader
	raftNode.currentLeader = "id"
	raftNode.myVote = "id"
	raftNode.nextIndex = map[ServerID]uint{"2": 4, "3": 5}
	raftNode.electionTimer = time.NewTimer(time.Second)
	raftNode.minimumTimer = time.NewTimer(time.Second)
	raftNode.leaderCh = make(chan bool, 1)

	raftNode.revertToFollower()

	select {
	case flag := <-raftNode.leaderCh:
		if !flag {
			t.Errorf("Expected value to be sent to leaderCh")
		}
	default:
		t.Errorf("Expected value to be sent to leaderCh")
	}

	if raftNode.state != Follower {
		t.Errorf("Expected state to be Follower, got %v", raftNode.state)
	}

	if raftNode.currentLeader != "" {
		t.Errorf("Expected currentLeader to be empty, got %v", raftNode.currentLeader)
	}

	if raftNode.myVote != "" {
		t.Errorf("Expected myVote to be empty, got %v", raftNode.myVote)
	}
}

type MockServer struct {
	t               *testing.T
	expectedRequest RequestVoteArguments
	response        RequestVoteResult
}

func (m *MockServer) RequestVoteRPC(args *RequestVoteArguments, reply *RequestVoteResult) error {
	if *args != m.expectedRequest {
		m.t.Errorf("Expected request %+v, got %+v", m.expectedRequest, *args)
	}
	*reply = m.response
	return nil
}

func TestAskForVotes(t *testing.T) {
	mockResponse := RequestVoteResult{
		Term:        1,
		VoteGranted: true,
	}

	expectedRequest := RequestVoteArguments{1, "id", 0, 0}

	server1 := &MockServer{t: t, expectedRequest: expectedRequest, response: mockResponse}
	server2 := &MockServer{t: t, expectedRequest: expectedRequest, response: mockResponse}

	rpc.RegisterName("RaftNode", server1)
	rpc.RegisterName("RaftNode", server2)

	listener1, err := net.Listen("tcp", ":1236")
	if err != nil {
		t.Fatalf("Failed to start mock server 1: %v", err)
	}
	defer listener1.Close()

	listener2, err := net.Listen("tcp", ":1235")
	if err != nil {
		t.Fatalf("Failed to start mock server 2: %v", err)
	}
	defer listener2.Close()

	go rpc.Accept(listener1)
	go rpc.Accept(listener2)

	client1, err := rpc.Dial("tcp", ":1236")
	if err != nil {
		t.Fatalf("Failed to connect to mock server 1: %v", err)
	}
	defer client1.Close()

	client2, err := rpc.Dial("tcp", ":1235")
	if err != nil {
		t.Fatalf("Failed to connect to mock server 2: %v", err)
	}
	defer client2.Close()

	raftNode := &RaftNode{
		id: "id",
		peersConnection: map[ServerID]*rpc.Client{
			"2": client1,
			"3": client2,
		},
		voteRequestCh:  make(chan RequestVoteArguments, 1),
		voteResponseCh: make(chan RequestVoteResultWithServerID, 2),
		term:           1,
		state:          Candidate,
	}

	raftNode.voteRequestCh <- expectedRequest
	go raftNode.askForVotes()

	time.Sleep(1 * time.Second)

	responses := make([]RequestVoteResult, 0)
	for response := range raftNode.voteResponseCh {
		responses = append(responses, response.result)
		if len(responses) == 2 {
			break
		}
	}

	if len(responses) != 2 {
		t.Errorf("Expected 2 responses, got %d", len(responses))
	}

	for _, response := range responses {
		if !response.VoteGranted {
			t.Errorf("Expected VoteGranted to be true, got %v", response.VoteGranted)
		}
		if response.Term != 1 {
			t.Errorf("Expected Term to be 1, got %d", response.Term)
		}
	}
}

func TestAskForStaledVotes(t *testing.T) {
	mockResponse := RequestVoteResult{
		Term:        2,
		VoteGranted: false,
	}

	expectedRequest := RequestVoteArguments{1, "id", 0, 0}

	server1 := &MockServer{t: t, expectedRequest: expectedRequest, response: mockResponse}

	rpc.RegisterName("RaftNode", server1)

	listener1, err := net.Listen("tcp", ":1234")
	if err != nil {
		t.Fatalf("Failed to start mock server 1: %v", err)
	}
	defer listener1.Close()

	go rpc.Accept(listener1)

	client1, err := rpc.Dial("tcp", ":1234")
	if err != nil {
		t.Fatalf("Failed to connect to mock server 1: %v", err)
	}
	defer client1.Close()

	raftNode := &RaftNode{
		id: "id",
		peersConnection: map[ServerID]*rpc.Client{
			"2": client1,
		},
		voteRequestCh:  make(chan RequestVoteArguments, 1),
		voteResponseCh: make(chan RequestVoteResultWithServerID, 2),
		term:           2,
		state:          Candidate,
	}

	raftNode.voteRequestCh <- expectedRequest
	go raftNode.askForVotes()

	time.Sleep(1 * time.Second)

	responses := make([]RequestVoteResult, 0)
	for response := range raftNode.voteResponseCh {
		responses = append(responses, response.result)
		if len(responses) == 1 {
			break
		}
	}

	if len(responses) != 1 {
		t.Errorf("Expected 1 response, got %d", len(responses))
	}

	for _, response := range responses {
		if response.VoteGranted {
			t.Errorf("Expected VoteGranted to be false, got %v", response.VoteGranted)
		}
	}
}

func TestHandleVotesSuccess(t *testing.T) {
	raftNode := &RaftNode{
		id:                "id",
		term:              1,
		state:             Candidate,
		electionVotesNewC: 0,
		voteResponseCh:    make(chan RequestVoteResultWithServerID, 3),
		electionTimer:     time.NewTimer(time.Second),
		peers:             Configuration{OldConfig: nil, NewConfig: map[ServerID]Address{"1": "1233", "2": "1234", "3": "1235"}},
		snapshot: &Snapshot{
			LastIndex:        0,
			LastTerm:         0,
			LastConfig:       nil,
			StateMachineSnap: nil,
		},
		log: logStruct{
			entries: []LogEntry{
				// to start the log from index 1 we add a dummy entry
				{
					Index:   0,
					Term:    0,
					Command: nil,
					Type:    NOOPEntry,
					Client:  "",
					USN:     -1,
				},
			},
			lastCommitedIndex: 0,
		},
	}

	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"1", RequestVoteResult{1, true}}
	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"2", RequestVoteResult{1, true}}
	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"3", RequestVoteResult{1, false}}

	go raftNode.handleVotes()

	time.Sleep(1 * time.Second)

	if raftNode.electionVotesNewC != 2 {
		t.Errorf("Expected electionVotes to be 2, got %d", raftNode.electionVotesNewC)
	}

	if raftNode.state != Leader {
		t.Errorf("Expected state to be Leader, got %v", raftNode.state)
	}
}

func TestHandleVotesFailure(t *testing.T) {
	raftNode := &RaftNode{
		id:                "id",
		term:              1,
		state:             Candidate,
		electionVotesNewC: 0,
		voteResponseCh:    make(chan RequestVoteResultWithServerID, 3),
		electionTimer:     time.NewTimer(time.Second),
		minimumTimer:      time.NewTimer(time.Second),
		peers:             Configuration{OldConfig: nil, NewConfig: map[ServerID]Address{"1": "1233", "2": "1234", "3": "1235"}},
	}

	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"1", RequestVoteResult{1, true}}
	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"2", RequestVoteResult{1, false}}
	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"3", RequestVoteResult{1, false}}

	go raftNode.handleVotes()

	time.Sleep(1 * time.Second)

	if raftNode.electionVotesNewC != 1 {
		t.Errorf("Expected electionVotes to be 1, got %d", raftNode.electionVotesNewC)
	}

	if raftNode.state != Candidate {
		t.Errorf("Expected state to be Candidate, got %v", raftNode.state)
	}
}

func TestHandleVotesWithNewTerm(t *testing.T) {
	raftNode := &RaftNode{
		id:                "id",
		term:              1,
		state:             Candidate,
		electionVotesNewC: 0,
		voteResponseCh:    make(chan RequestVoteResultWithServerID, 3),
		electionTimer:     time.NewTimer(time.Second),
		minimumTimer:      time.NewTimer(time.Second),
		peers:             Configuration{OldConfig: nil, NewConfig: map[ServerID]Address{"2": "1234", "3": "1235"}},
	}

	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"1", RequestVoteResult{1, true}}
	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"2", RequestVoteResult{2, true}}
	raftNode.voteResponseCh <- RequestVoteResultWithServerID{"3", RequestVoteResult{1, false}}

	go raftNode.handleVotes()

	time.Sleep(1 * time.Second)

	if raftNode.electionVotesNewC != 1 {
		t.Errorf("Expected electionVotes to be 1, got %d", raftNode.electionVotesNewC)
	}

	if raftNode.state != Follower {
		t.Errorf("Expected state to be Follower, got %v", raftNode.state)
	}
}
