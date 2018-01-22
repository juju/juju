// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttest

import (
	"encoding/gob"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

// FSM is an implementation of raft.FSM, which simply appends
// the log data to a slice.
type FSM struct {
	mu   sync.Mutex
	logs [][]byte
}

// Logs returns the accumulated log data.
func (fsm *FSM) Logs() [][]byte {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	copied := make([][]byte, len(fsm.logs))
	copy(copied, fsm.logs)
	return copied
}

// Apply is part of the raft.FSM interface.
func (fsm *FSM) Apply(log *raft.Log) interface{} {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	fsm.logs = append(fsm.logs, log.Data)
	return len(fsm.logs)
}

// Snapshot is part of the raft.FSM interface.
func (fsm *FSM) Snapshot() (raft.FSMSnapshot, error) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	copied := make([][]byte, len(fsm.logs))
	copy(copied, fsm.logs)
	return &Snapshot{copied, len(copied)}, nil
}

// Restore is part of the raft.FSM interface.
func (fsm *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	var logs [][]byte
	if err := gob.NewDecoder(rc).Decode(&logs); err != nil {
		return err
	}
	fsm.mu.Lock()
	fsm.logs = logs
	fsm.mu.Unlock()
	return nil
}

// Snapshot is an implementation of raft.FSMSnapshot, returned
// by the FSM.Snapshot in this package.
type Snapshot struct {
	logs [][]byte
	n    int
}

// Persist is part of the raft.FSMSnapshot interface.
func (snap *Snapshot) Persist(sink raft.SnapshotSink) error {
	if err := gob.NewEncoder(sink).Encode(snap.logs[:snap.n]); err != nil {
		sink.Cancel()
		return err
	}
	sink.Close()
	return nil
}

// Release is part of the raft.FSMSnapshot interface.
func (*Snapshot) Release() {}
