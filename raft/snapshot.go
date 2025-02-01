package raft

import "log"

type Snapshot struct {
	LastIndex        uint
	LastTerm         uint
	LastConfig       Configuration
	StateMachineSnap []byte
}

func (rn *RaftNode) handleSnapshot() {
	for {
		<-rn.takeSnapshotCh
		// take snapshot
		rn.mutex.Lock()

		// request the snapshot to the state machine
		rn.snapshotRequestCh <- struct{}{}
		stateMachineSnap := <-rn.snapshotResponseCh

		// update the snapshot in the raft node
		rn.snapshot = &Snapshot{
			LastIndex:        rn.lastGlobalCommitedIndex(),
			LastTerm:         rn.log.entries[rn.log.lastCommitedIndex].Term,
			LastConfig:       rn.peers,
			StateMachineSnap: stateMachineSnap,
		}
		// clean the log and add the dummy entry
		rn.log.entries = append([]LogEntry{LogEntry{
			Index:   rn.snapshot.LastIndex,
			Term:    rn.snapshot.LastTerm,
			Command: nil,
			Client:  "",
			USN:     -1,
		}}, rn.log.entries[rn.log.lastCommitedIndex+1:]...)
		rn.log.lastCommitedIndex = 0

		log.Println("Snapshot taken", rn.id)
		rn.mutex.Unlock()
	}
}
