package models

type Message struct {
	Username string
	Msg      string
}

type ClientRequestArguments struct {
	Command []byte
	Type    uint
	Id      string
	USN     int // Unique Sequence Number
}

type ClientRequestResult struct {
	Success bool
	Data    []byte
	Leader  string
}
