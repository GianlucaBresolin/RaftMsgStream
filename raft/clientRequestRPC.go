package raft

type ClientRequestArguments struct {
	Command string
	Id      string
	USN     uint // Unique Sequence Number
}

type ClientRequestResult struct {
	Success bool
	Leader  ServerID
}

type replicationState struct {
	replicationCounter uint
	committed          bool
	clientCh           chan bool
}

func (n *Node) ClientRequestRPC(req ClientRequestArguments, res *ClientRequestResult) error {
	n.state.mutex.Lock()

	if n.state.state == Leader {
		// check if the request is stale
		lastUSN, ok := n.state.lastUSNof[req.Id]
		if !ok {
			n.state.lastUSNof[req.Id] = req.USN
		}

		if !ok || lastUSN < req.USN {
			logEntry := LogEntry{
				Index:   n.state.log.lastIndex() + 1,
				Term:    n.state.term,
				Command: req.Command,
				Client:  req.Id,
				USN:     req.USN,
			}

			n.state.log.entries = append(n.state.log.entries, logEntry)
			n.state.pendingCommit[logEntry.Index] = replicationState{
				replicationCounter: 1, // leader already replicated
				committed:          false,
				clientCh:           make(chan bool),
			}
			n.state.logEntriesCh <- struct{}{} // trigger log replication
			n.state.mutex.Unlock()

			committed := <-n.state.pendingCommit[logEntry.Index].clientCh
			res.Success = committed
			res.Leader = n.state.id
			return nil
		}
	}

	// redirect to leader if not leader or stale request
	res.Success = false
	res.Leader = n.state.currentLeader
	n.state.mutex.Unlock()
	return nil
}
