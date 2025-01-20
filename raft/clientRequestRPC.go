package raft

type ClientRequestArguments struct {
	Command string
	Port    string
}

type ClientRequestResult struct {
	Success bool
	Leader  ServerID
}

func (n *Node) ClientRequestRPC(req ClientRequestArguments, res *ClientRequestResult) error {
	n.state.mutex.Lock()

	if n.state.state == Leader {
		logEntry := LogEntry{
			Index:   n.state.log.lastIndex() + 1,
			Term:    n.state.term,
			Command: req.Command,
		}

		n.state.log.entries = append(n.state.log.entries, logEntry)
		n.state.pendingCommit[logEntry.Index] = replicationState{
			replicationCounter: 1, //leader already replicated
			replicationSuccess: false,
		}
		n.state.logEntriesCh <- struct{}{} // trigger log replication
		n.state.mutex.Unlock()

		// TODO: find a way to send response to client only when committed
		res.Success = true
		res.Leader = n.state.id
		return nil
	}

	//redirect to leader
	res.Success = false
	res.Leader = n.state.currentLeader
	n.state.mutex.Unlock()
	return nil
}
