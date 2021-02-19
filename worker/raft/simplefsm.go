// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"encoding/gob"
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

// SimpleFSM is an implementation of raft.FSM, which simply appends
// the log data to a slice.
type SimpleFSM struct {
	mu   sync.Mutex
	logs [][]byte
}

// Logs returns the accumulated log data.
func (fsm *SimpleFSM) Logs() [][]byte {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	copied := make([][]byte, len(fsm.logs))
	copy(copied, fsm.logs)
	return copied
}

// Apply is part of the raft.FSM interface.
func (fsm *SimpleFSM) Apply(log *raft.Log) interface{} {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	fsm.logs = append(fsm.logs, log.Data)
	return len(fsm.logs)
}

// Snapshot is part of the raft.FSM interface.
func (fsm *SimpleFSM) Snapshot() (raft.FSMSnapshot, error) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	copied := make([][]byte, len(fsm.logs))
	copy(copied, fsm.logs)
	return &SimpleSnapshot{copied, len(copied)}, nil
}

// Restore is part of the raft.FSM interface.
func (fsm *SimpleFSM) Restore(rc io.ReadCloser) error {
	var logs [][]byte
	if err := gob.NewDecoder(rc).Decode(&logs); err != nil {
		_ = rc.Close()
		return err
	}
	fsm.mu.Lock()
	fsm.logs = logs
	fsm.mu.Unlock()
	return rc.Close()
}

// SimpleSnapshot is an implementation of raft.FSMSnapshot, returned
// by the SimpleFSM.Snapshot in this package.
type SimpleSnapshot struct {
	logs [][]byte
	n    int
}

// Persist is part of the raft.FSMSnapshot interface.
func (snap *SimpleSnapshot) Persist(sink raft.SnapshotSink) error {
	if err := gob.NewEncoder(sink).Encode(snap.logs[:snap.n]); err != nil {
		_ = sink.Cancel()
		return err
	}
	return sink.Close()
}

// Release is part of the raft.FSMSnapshot interface.
func (*SimpleSnapshot) Release() {}
