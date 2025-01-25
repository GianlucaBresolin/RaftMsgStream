package raft

type ClientRequestArguments struct {
	Command []byte
	Type    uint
	Id      string
	USN     int // Unique Sequence Number
}

type ClientRequestResult struct {
	Success bool
	Leader  ServerID
}

type replicationState struct {
	replicationCounterOldC uint
	replicationCounterNewC uint
	committedOldC          bool
	committedNewC          bool
	term                   uint
	clientCh               chan bool
}

func (n *Node) ClientRequestRPC(req ClientRequestArguments, res *ClientRequestResult) error {
	n.state.mutex.Lock()

	if n.state.state == Leader {
		// check if the request is stale
		// check if we already committed the request
		_, okC := n.state.lastUSNof[req.Id]
		if !okC {
			n.state.lastUSNof[req.Id] = -1
		}
		// check if we already have the request in the last requests
		_, okP := n.state.lastUncommitedRequestof[req.Id]
		if !okP {
			n.state.lastUncommitedRequestof[req.Id] = -1
		}

		if n.state.lastUSNof[req.Id] < req.USN && n.state.lastUncommitedRequestof[req.Id] < req.USN {
			var command []byte
			if req.Type == 1 {
				// prepare Cold,new
				command = n.state.prepareCold_new(req.Command)
			} else {
				command = req.Command
			}

			logEntry := LogEntry{
				Index:   n.state.log.lastIndex() + 1,
				Term:    n.state.term,
				Command: command,
				Type:    req.Type,
				Client:  req.Id,
				USN:     req.USN,
			}

			n.state.log.entries = append(n.state.log.entries, logEntry)
			clientCh := make(chan bool)

			commitedOldC := false
			if n.state.peers.OldConfig == nil {
				commitedOldC = true
			}

			_, ok := n.state.peers.NewConfig[n.state.id]
			replicationCounterNewC := 0
			if ok {
				replicationCounterNewC = 1 // leader already replicated
			}

			n.state.pendingCommit[logEntry.Index] = replicationState{
				replicationCounterOldC: 1, // leader already replicated
				replicationCounterNewC: uint(replicationCounterNewC),
				committedOldC:          commitedOldC,
				committedNewC:          false,
				term:                   n.state.term,
				clientCh:               clientCh,
			}

			n.state.lastUncommitedRequestof[req.Id] = req.USN

			n.state.logEntriesCh <- struct{}{} // trigger log replication
			n.state.mutex.Unlock()

			committed := <-clientCh

			// if was a change configuration request, trigger the Cnew entry
			if req.Type == 1 && committed {
				go n.state.prepareCnew()
			}

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
