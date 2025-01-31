package models

type Message struct {
	Username string
	Msg      string
}

// updateRPC types for the client
type UpdateARgs struct {
	Server string
	Group  string
}

type UpdateResult struct {
	Success bool
}

// getStateRPC types for the server
type GetStateArgs struct {
	Group            string
	LastMessageIndex uint
}

type GetStateResult struct {
	Messages []Message
}
