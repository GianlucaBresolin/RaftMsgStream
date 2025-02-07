package raft

import (
	"log"
)

type Snapshot struct {
	LastIndex        uint
	LastTerm         uint
	LastConfig       []byte
	LastUSNof        map[string]int
	StateMachineSnap []byte
}

func (rn *RaftNode) handleSnapshot() {
	for {
		select {
		case <-rn.shutdownSnapshotCh:
			return
		case <-rn.takeSnapshotCh:
			// take snapshot
			rn.mutex.Lock()

			// request the snapshot to the state machine
			rn.SnapshotRequestCh <- struct{}{}
			stateMachineSnap := <-rn.SnapshotResponseCh

			// create the last committed configuration in []byte
			var lastConfigCommand []byte
			for _, entry := range rn.log.entries[:rn.log.lastCommitedIndex+1] {
				if entry.Type == ConfigurationEntry {
					lastConfigCommand = entry.Command
				}
			}
			if lastConfigCommand == nil {
				// no configuration entry found, use the previous snapshot configuration
				lastConfigCommand = rn.snapshot.LastConfig
			}

			// update the snapshot in the raft node
			rn.snapshot = &Snapshot{
				LastIndex:        rn.lastGlobalCommitedIndex(),
				LastTerm:         rn.log.entries[rn.log.lastCommitedIndex].Term,
				LastConfig:       lastConfigCommand,
				LastUSNof:        rn.lastUSNof,
				StateMachineSnap: stateMachineSnap,
			}
			// clean the log and add the dummy entry
			dummyEntry := LogEntry{
				Index:   rn.snapshot.LastIndex,
				Term:    rn.snapshot.LastTerm,
				Command: nil,
				Client:  "",
				USN:     -1,
			}
			rn.log.entries = append([]LogEntry{dummyEntry}, rn.log.entries[rn.log.lastCommitedIndex+1:]...)
			rn.log.lastCommitedIndex = 0

			log.Println("Snapshot taken by ", rn.id)
			rn.mutex.Unlock()
		}
	}
}
