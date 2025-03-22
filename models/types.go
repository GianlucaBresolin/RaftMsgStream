package models

type Message struct {
	Username string
	Msg      string
}

type ClientActionArguments struct {
	Command []byte
	Type    uint
	Id      string
	USN     int // Unique Sequence Number
}

type ClientActionResult struct {
	Success bool
	Data    []byte
	Leader  string
}

type ClientGetStateArguments struct {
	Command []byte
	Id      string
	USN     int
}

type ClientGetStateResult struct {
	Success bool
	Data    []byte
	Leader  string
}

type GetStateArgs struct {
	Username         string
	Group            string
	LastMessageIndex uint
}

type GetStateResult struct {
	Messages   []Message
	Membership bool
}

type Event struct {
	Msg   Message
	Group string
	Users map[string]bool
}
