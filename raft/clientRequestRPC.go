package raft

import "log"

type ClientRequestArguments struct {
	Command string
}

type ClientRequestResult struct {
	Success bool
	Leader  ServerID
}

func (n *Node) ClientRequestRPC(req ClientRequestArguments, res *ClientRequestResult) error {
	n.state.mutex.Lock()

	if n.state.state == Leader {
		logEntry := LogEntry{
			Index:     n.state.log.lastIndex() + 1,
			Term:      n.state.term,
			Command:   req.Command,
			Committed: false,
		}

		n.state.log.entries = append(n.state.log.entries, logEntry)
		n.state.logEntriesCh <- &logEntry

		responseCh := make(chan bool)
		n.state.pendingCommit[logEntry.Index] = responseCh
		n.state.mutex.Unlock()

		//wait for commit
		committed := <-responseCh
		delete(n.state.pendingCommit, logEntry.Index)
		close(responseCh)
		log.Println("Committed:", committed)
		res.Success = committed
		res.Leader = n.state.id
		return nil
	}

	//redirect to leader
	res.Success = false
	res.Leader = n.state.currentLeader
	n.state.mutex.Unlock()
	return nil
}
