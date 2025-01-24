package raft

import (
	"net"
	"net/rpc"
	"testing"
	"time"
)

func TestStartElection(t *testing.T) {
	nodeState := newNodeState("id", map[ServerID]Port{"2": "1234", "3": "1235"})
	nodeState.term = 1

	nodeState.startElection()

	if nodeState.term != 2 {
		t.Errorf("Expected term to be 2, got %v", nodeState.term)
	}

	if nodeState.state != Candidate {
		t.Errorf("Expected state to be Candidate, got %v", nodeState.state)
	}
}

func TestWinElection(t *testing.T) {
	nodeState := newNodeState("id", map[ServerID]Port{"2": "1234", "3": "1235"})
	nodeState.term = 1
	nodeState.state = Candidate
	nodeState.log.entries = []LogEntry{
		{Index: 0, Term: 1},
		{Index: 1, Term: 1},
		{Index: 2, Term: 1},
		{Index: 3, Term: 1},
	}
	nodeState.electionTimer = time.NewTimer(time.Second)
	nodeState.minimumTimer = time.NewTimer(time.Second)

	nodeState.winElection()

	if nodeState.state != Leader {
		t.Errorf("Expected state to be Leader, got %v", nodeState.state)
	}

	if nodeState.currentLeader != "id" {
		t.Errorf("Expected currentLeader to be id, got %v", nodeState.currentLeader)
	}

	if nodeState.nextIndex["2"] != 4 {
		t.Errorf("Expected nextIndex[2] to be 4, got %v", nodeState.nextIndex["2"])
	}

	time.Sleep(2 * time.Second) // ensure electionTimer has expired
	select {
	case <-nodeState.electionTimer.C:
		t.Errorf("Expected electionTimer to be stopped")
	default:
	}
}

func TestRevertToFollower(t *testing.T) {
	nodeState := newNodeState("id", map[ServerID]Port{"2": "1234", "3": "1235"})
	nodeState.state = Leader
	nodeState.currentLeader = "id"
	nodeState.myVote = "id"
	nodeState.nextIndex = map[ServerID]uint{"2": 4, "3": 5}
	nodeState.electionTimer = time.NewTimer(time.Second)
	nodeState.minimumTimer = time.NewTimer(time.Second)
	nodeState.leaderCh = make(chan bool, 1)

	nodeState.revertToFollower()

	select {
	case flag := <-nodeState.leaderCh:
		if !flag {
			t.Errorf("Expected value to be sent to leaderCh")
		}
	default:
		t.Errorf("Expected value to be sent to leaderCh")
	}

	if nodeState.state != Follower {
		t.Errorf("Expected state to be Follower, got %v", nodeState.state)
	}

	if nodeState.currentLeader != "" {
		t.Errorf("Expected currentLeader to be empty, got %v", nodeState.currentLeader)
	}

	if nodeState.myVote != "" {
		t.Errorf("Expected myVote to be empty, got %v", nodeState.myVote)
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

	rpc.RegisterName("Node", server1)
	rpc.RegisterName("Node", server2)

	listener1, err := net.Listen("tcp", ":1234")
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

	client1, err := rpc.Dial("tcp", ":1234")
	if err != nil {
		t.Fatalf("Failed to connect to mock server 1: %v", err)
	}
	defer client1.Close()

	client2, err := rpc.Dial("tcp", ":1235")
	if err != nil {
		t.Fatalf("Failed to connect to mock server 2: %v", err)
	}
	defer client2.Close()

	nodeState := &nodeState{
		id: "id",
		peersConnection: map[ServerID]*rpc.Client{
			"2": client1,
			"3": client2,
		},
		voteRequestCh:  make(chan RequestVoteArguments, 1),
		voteResponseCh: make(chan RequestVoteResult, 2),
		term:           1,
		state:          Candidate,
	}

	nodeState.voteRequestCh <- expectedRequest
	go nodeState.askForVotes()

	time.Sleep(1 * time.Second)

	responses := make([]RequestVoteResult, 0)
	for response := range nodeState.voteResponseCh {
		responses = append(responses, response)
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

	rpc.RegisterName("Node", server1)

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

	nodeState := &nodeState{
		id: "id",
		peersConnection: map[ServerID]*rpc.Client{
			"2": client1,
		},
		voteRequestCh:  make(chan RequestVoteArguments, 1),
		voteResponseCh: make(chan RequestVoteResult, 2),
		term:           2,
		state:          Candidate,
	}

	nodeState.voteRequestCh <- expectedRequest
	go nodeState.askForVotes()

	time.Sleep(1 * time.Second)

	responses := make([]RequestVoteResult, 0)
	for response := range nodeState.voteResponseCh {
		responses = append(responses, response)
		if len(responses) == 1 {
			break
		}
	}

	if len(responses) != 1 {
		t.Errorf("Expected 1 response, got %d", len(responses))
	}

	for _, response := range responses {
		if !response.VoteGranted {
			t.Errorf("Expected VoteGranted to be false, got %v", response.VoteGranted)
		}
	}
}

func TestHandleVotesSuccess(t *testing.T) {
	nodeState := &nodeState{
		id:             "id",
		term:           1,
		state:          Candidate,
		electionVotes:  0,
		voteResponseCh: make(chan RequestVoteResult, 3),
		electionTimer:  time.NewTimer(time.Second),
		peers:          Configuration{OldConfig: nil, NewConfig: map[ServerID]Port{"2": "1234", "3": "1235"}},
	}

	nodeState.voteResponseCh <- RequestVoteResult{1, true}
	nodeState.voteResponseCh <- RequestVoteResult{1, true}
	nodeState.voteResponseCh <- RequestVoteResult{1, false}

	go nodeState.handleVotes()

	time.Sleep(1 * time.Second)

	if nodeState.electionVotes != 2 {
		t.Errorf("Expected electionVotes to be 1, got %d", nodeState.electionVotes)
	}

	if nodeState.state != Leader {
		t.Errorf("Expected state to be Leader, got %v", nodeState.state)
	}
}

func TestHandleVotesFailure(t *testing.T) {
	nodeState := &nodeState{
		id:             "id",
		term:           1,
		state:          Candidate,
		electionVotes:  0,
		voteResponseCh: make(chan RequestVoteResult, 3),
		electionTimer:  time.NewTimer(time.Second),
		minimumTimer:   time.NewTimer(time.Second),
		peers:          Configuration{OldConfig: nil, NewConfig: map[ServerID]Port{"2": "1234", "3": "1235"}},
	}

	nodeState.voteResponseCh <- RequestVoteResult{1, true}
	nodeState.voteResponseCh <- RequestVoteResult{1, false}
	nodeState.voteResponseCh <- RequestVoteResult{1, false}

	go nodeState.handleVotes()

	time.Sleep(1 * time.Second)

	if nodeState.electionVotes != 1 {
		t.Errorf("Expected electionVotes to be 1, got %d", nodeState.electionVotes)
	}

	if nodeState.state != Candidate {
		t.Errorf("Expected state to be Candidate, got %v", nodeState.state)
	}
}

func TestHandleVotesWithNewTerm(t *testing.T) {
	nodeState := &nodeState{
		id:             "id",
		term:           1,
		state:          Candidate,
		electionVotes:  0,
		voteResponseCh: make(chan RequestVoteResult, 3),
		electionTimer:  time.NewTimer(time.Second),
		minimumTimer:   time.NewTimer(time.Second),
		peers:          Configuration{OldConfig: nil, NewConfig: map[ServerID]Port{"2": "1234", "3": "1235"}},
	}

	nodeState.voteResponseCh <- RequestVoteResult{1, true}
	nodeState.voteResponseCh <- RequestVoteResult{2, false}
	nodeState.voteResponseCh <- RequestVoteResult{2, false}

	go nodeState.handleVotes()

	time.Sleep(1 * time.Second)

	if nodeState.electionVotes != 1 {
		t.Errorf("Expected electionVotes to be 1, got %d", nodeState.electionVotes)
	}

	if nodeState.state != Follower {
		t.Errorf("Expected state to be Follower, got %v", nodeState.state)
	}
}
