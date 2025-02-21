package raft

import (
	"RaftMsgStream/models"
)

type replicationState struct {
	replicationCounterOldC uint
	replicationCounterNewC uint
	committedOldC          bool
	committedNewC          bool
	term                   uint
	clientCh               chan bool
}

func (rn *RaftNode) ActionRequest(req models.ClientRequestArguments, res *models.ClientRequestResult) {
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
				res.Success = true
				res.Leader = string(rn.id)
				rn.mutex.Unlock()
				return
			}
		}

		if rn.lastUSNof[req.Id] < req.USN {
			var command []byte
			if req.Type == 1 {
				// check if we altready have a configuration change in place
				if rn.peers.OldConfig != nil {
					// discard the request
					res.Success = false
					res.Leader = string(rn.id)
					rn.mutex.Unlock()
					return
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
			rn.mutex.Unlock()
			rn.logEntriesCh <- struct{}{} // trigger log replication

			committed := <-clientCh

			res.Success = committed
			res.Leader = string(rn.id)
			return
		} else {
			// we already have the request in the log, discard it
			res.Success = true
			res.Leader = string(rn.id)
			rn.mutex.Unlock()
			return
		}
	}

	// redirect to leader if not leader or stale request
	res.Success = false
	res.Leader = string(rn.currentLeader)
	rn.mutex.Unlock()
}

func (rn *RaftNode) GetState(req models.ClientRequestArguments, res *models.ClientRequestResult) {
	rn.mutex.Lock()
	// check if the request is stale
	_, okC := rn.lastUSNof[req.Id]
	if !okC {
		rn.lastUSNof[req.Id] = -1
	}
	if rn.lastUSNof[req.Id] >= req.USN {
		res.Success = false
		res.Leader = string(rn.currentLeader)
		rn.mutex.Unlock()
		return
	}

	// provide the read-only operation sacrificing linearizability if unvoting server, otherwise
	// if we are the leader, proceed with the no-op entry to provide the most up-to-date state
	if !rn.unvotingServer {
		if rn.state != Leader {
			res.Success = false
			res.Leader = string(rn.currentLeader)
			rn.mutex.Unlock()
			return
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

		rn.mutex.Unlock()
		rn.logEntriesCh <- struct{}{} // trigger log replication

		committed := <-nodeCh
		close(nodeCh)
		if !committed {
			res.Success = false
			res.Leader = string(rn.id)
			return
		}
		rn.mutex.Lock()
	}

	// we can now read the state machine
	rn.USN++
	rn.ReadStateCh <- req.Command // trigger the command read to the state machine

	resultData := <-rn.ReadStateResultCh
	rn.mutex.Unlock()

	res.Success = true
	res.Data = resultData
	res.Leader = string(rn.currentLeader)
}
