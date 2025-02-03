package raft

type ClientRequestArguments struct {
	Command []byte
	Type    uint
	Id      string
	USN     int // Unique Sequence Number
}

type ClientRequestResult struct {
	Success bool
	Data    []byte
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

func (rn *RaftNode) ActionRequestRPC(req ClientRequestArguments, res *ClientRequestResult) error {
	rn.mutex.Lock()

	if rn.state == Leader {
		// check if the request is stale
		// check if we already committed the request
		_, okC := rn.lastUSNof[req.Id]
		if !okC {
			rn.lastUSNof[req.Id] = -1
		}
		// check if we already have the request in the last requests still uncommited
		for _, entry := range rn.log.entries[rn.log.lastCommitedIndex+1:] {
			if entry.Client == req.Id {
				// we already have the request in the log, discard it
				res.Success = false
				res.Leader = rn.id
				rn.mutex.Unlock()
				return nil
			}
		}

		if rn.lastUSNof[req.Id] < req.USN {
			var command []byte
			if req.Type == 1 {
				// check if we altready have a configuration change in place
				if rn.peers.OldConfig != nil {
					// discard the request
					res.Success = false
					res.Leader = rn.id
					rn.mutex.Unlock()
					return nil
				}
				// prepare Cold,new
				command = rn.prepareCold_new(req.Command)
			} else {
				command = req.Command
			}

			logEntry := LogEntry{
				Index:   rn.lastGlobalIndex() + 1,
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

			rn.logEntriesCh <- struct{}{} // trigger log replication
			rn.mutex.Unlock()

			committed := <-clientCh

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

func (rn *RaftNode) GetStateRPC(req ClientRequestArguments, res *ClientRequestResult) error {
	rn.mutex.Lock()

	if rn.state != Leader {
		res.Success = false
		res.Leader = rn.currentLeader
		rn.mutex.Unlock()
		return nil
	}

	// append a NOOP entry to the log
	logEntry := LogEntry{
		Index:   rn.lastGlobalIndex() + 1,
		Term:    rn.term,
		Command: nil,
		Type:    NOOPEntry,
		Client:  string(rn.id),
		USN:     rn.USN,
	}
	rn.log.entries = append(rn.log.entries, logEntry)
	nodeCh := make(chan bool)

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
		clientCh:               nodeCh,
	}

	rn.logEntriesCh <- struct{}{} // trigger log replication
	rn.mutex.Unlock()

	committed := <-nodeCh
	close(nodeCh)
	if !committed {
		res.Success = false
		res.Leader = rn.id
		return nil
	}
	// we can now read the state machine
	rn.mutex.Lock()
	rn.USN++
	rn.ReadStateCh <- req.Command // trigger the command read to the state machine

	resultData := <-rn.ReadStateResultCh
	rn.mutex.Unlock()

	res.Success = true
	res.Data = resultData
	res.Leader = rn.id
	return nil
}
