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

func (rn *RaftNode) ClientRequestRPC(req ClientRequestArguments, res *ClientRequestResult) error {
	rn.mutex.Lock()

	if rn.state == Leader {
		// check if the request is stale
		// check if we already committed the request
		_, okC := rn.lastUSNof[req.Id]
		if !okC {
			rn.lastUSNof[req.Id] = -1
		}
		// check if we already have the request in the last requests
		_, okP := rn.lastUncommitedRequestof[req.Id]
		if !okP {
			rn.lastUncommitedRequestof[req.Id] = -1
		}

		if rn.lastUSNof[req.Id] < req.USN && rn.lastUncommitedRequestof[req.Id] < req.USN {
			var command []byte
			if req.Type == 1 {
				// prepare Cold,new
				command = rn.prepareCold_new(req.Command)
			} else {
				command = req.Command
			}

			logEntry := LogEntry{
				Index:   rn.log.lastIndex() + 1,
				Term:    rn.term,
				Command: command,
				Type:    req.Type,
				Client:  req.Id,
				USN:     req.USN,
			}

			rn.log.entries = append(rn.log.entries, logEntry)
			clientCh := make(chan bool)

			commitedOldC := false
			if rn.peers.OldConfig == nil {
				commitedOldC = true
			}

			_, ok := rn.peers.NewConfig[rn.id]
			replicationCounterNewC := 0
			if ok {
				replicationCounterNewC = 1 // leader already replicated
			}

			rn.pendingCommit[logEntry.Index] = replicationState{
				replicationCounterOldC: 1, // leader already replicated
				replicationCounterNewC: uint(replicationCounterNewC),
				committedOldC:          commitedOldC,
				committedNewC:          false,
				term:                   rn.term,
				clientCh:               clientCh,
			}

			rn.lastUncommitedRequestof[req.Id] = req.USN

			rn.logEntriesCh <- struct{}{} // trigger log replication
			rn.mutex.Unlock()

			committed := <-clientCh

			// if was a change configuration request, trigger the Cnew entry
			if req.Type == 1 && committed {
				go rn.prepareCnew()
			}

			res.Success = committed
			res.Leader = rn.id
			return nil
		}
	}

	// redirect to leader if not leader or stale request
	res.Success = false
	res.Leader = rn.currentLeader
	rn.mutex.Unlock()
	return nil
}
