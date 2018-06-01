// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"io"
	"sync"

	"github.com/hashicorp/raft"
)

type closableStore interface {
	raft.LogStore
	io.Closer
}

// syncLogStore is a raft.LogStore that ensures calls to the various
// interface methods (as well as Close) are goroutine safe. The log
// store is shared between the raft worker and the raft-backstop
// because a boltdb file can't be opened for writing from two places,
// and the backstop worker needs to be able to append an updated
// configuration to recover the raft cluster when there's no way to
// reach quorum.
type syncLogStore struct {
	mu    sync.Mutex
	store closableStore
}

// FirstIndex is part of raft.LogStore.
func (s *syncLogStore) FirstIndex() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.FirstIndex()
}

// LastIndex is part of raft.LogStore.
func (s *syncLogStore) LastIndex() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.LastIndex()
}

// GetLog is part of raft.LogStore.
func (s *syncLogStore) GetLog(index uint64, log *raft.Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.GetLog(index, log)
}

// StoreLog is part of raft.LogStore.
func (s *syncLogStore) StoreLog(log *raft.Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.StoreLog(log)
}

// StoreLogs is part of raft.LogStore.
func (s *syncLogStore) StoreLogs(logs []*raft.Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.StoreLogs(logs)
}

// DeleteRange is part of raft.LogStore.
func (s *syncLogStore) DeleteRange(min, max uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.DeleteRange(min, max)
}

// Close closes the underlying logstore.
func (s *syncLogStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.Close()
}
