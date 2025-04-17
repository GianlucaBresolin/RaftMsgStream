package raft

import (
	"RaftMsgStream/models"
	"log"
	"sync"
	"time"
)

type replicationState struct {
	replicationCounterOldC uint
	replicationCounterNewC uint
	committedOldC          bool
	committedNewC          bool
	term                   uint
	clientCh               chan bool
}

func (rn *RaftNode) ActionRequest(req models.ClientActionArguments, res *models.ClientActionResult) {
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
				res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
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
					res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
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
			res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
			return
		} else {
			// we already have the request in the log, discard it
			res.Success = true
			res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
			rn.mutex.Unlock()
			return
		}
	}

	// redirect to leader if not leader or stale request
	res.Success = false
	res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
	rn.mutex.Unlock()
}

func (rn *RaftNode) GetState(req models.ClientGetStateArguments, res *models.ClientGetStateResult) {
	rn.mutex.Lock()
	// check if the request is stale
	_, okC := rn.lastUSNof[req.Id]
	if !okC {
		rn.lastUSNof[req.Id] = -1
	}
	if rn.lastUSNof[req.Id] >= req.USN {
		res.Success = false
		res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
		rn.mutex.Unlock()
		return
	}

	// provide the read-only operation sacrificing linearizability if unvoting server, otherwise
	// if we are the leader, proceed with exchaging heartbeats with the majority of the cluster
	// to provide the most up-to-date state
	if !rn.unvotingServer {
		if rn.state != Leader {
			res.Success = false
			res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
			rn.mutex.Unlock()
			return
		}

		// check if we have the NOOP entry committed to provide that, as a leader, we have the latest information
		// on which entries are committed
		if !rn.committedNooP {
			res.Success = false
			res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
			rn.mutex.Unlock()
			return
		}

		// exchange heartbeats with the majority of the cluster to check if we were deposed or not
		autorizedReadCh := make(chan struct{})
		// check if we have the majority of the old cluster (if present) to respond
		heartbeatCounterOld := 0
		if rn.peers.OldConfig != nil {
			heartbeatCounterOld = 1 // leader already responded if it is in the old configuration
		}
		// check if we have the majority of the new cluster to respond
		heartbeatCounterNew := 0
		if _, ok := rn.peers.NewConfig[rn.id]; ok {
			heartbeatCounterNew = 1 // leader already responded if it is in the new configuration
		}
		autorizedRead := false
		autorizedReadLock := sync.Mutex{}
		authorizedReadTimeout := time.After(100 * time.Millisecond)

		for node, peerConnection := range rn.peersConnection {
			_, okOld := rn.peers.OldConfig[node]
			_, okNew := rn.peers.NewConfig[node]
			if node == rn.id || (!okOld && !okNew) {
				// skip the current node (leader) and the unvoting servers
				continue
			}
			go func() {
				success := rn.handleReplicationLog(node, peerConnection)
				autorizedReadLock.Lock()
				if success {
					if okOld {
						heartbeatCounterOld++
					}
					if okNew {
						heartbeatCounterNew++
					}
				}
				if (rn.peers.OldConfig == nil || heartbeatCounterOld > len(rn.peers.OldConfig)/2) && heartbeatCounterNew > len(rn.peers.NewConfig)/2 && !autorizedRead {
					autorizedRead = true // to avoid flooding the channel
					autorizedReadCh <- struct{}{}
				}
				autorizedReadLock.Unlock()
			}()
		}
		rn.mutex.Unlock() // unlock to enable the exchange of heartbeats

		select {
		case <-authorizedReadTimeout:
			// we did not get the majority of the cluster to respond
			res.Success = false
			res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
			log.Println("Node", rn.id, "did not get the majority of the cluster to respond or the timeout expired")
			rn.mutex.Unlock()
			return
		case <-autorizedReadCh:
			// we got the majority of the cluster to respond, proceed with the read
			// log.Println("Node", rn.id, "got the majority of the cluster to respond")
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
	res.Leader = string(rn.peers.NewConfig[rn.currentLeader])
}
