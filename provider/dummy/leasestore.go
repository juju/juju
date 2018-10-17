// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
)

// leaseStore implements lease.Store as simply as possible for use in
// the dummy provider. Heavily cribbed from raftlease.FSM.
type leaseStore struct {
	mu       sync.Mutex
	clock    clock.Clock
	entries  map[lease.Key]*entry
	trapdoor raftlease.TrapdoorFunc
	target   raftlease.NotifyTarget
}

// entry holds the details of a lease.
type entry struct {
	// holder identifies the current holder of the lease.
	holder string

	// start is the global time at which the lease started.
	start time.Time

	// duration is the duration for which the lease is valid,
	// from the start time.
	duration time.Duration
}

func newLeaseStore(clock clock.Clock, target raftlease.NotifyTarget, trapdoor raftlease.TrapdoorFunc) *leaseStore {
	return &leaseStore{
		clock:    clock,
		entries:  make(map[lease.Key]*entry),
		target:   target,
		trapdoor: trapdoor,
	}
}

// ClaimLease is part of lease.Store.
func (s *leaseStore) ClaimLease(key lease.Key, req lease.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found := s.entries[key]; found {
		return lease.ErrInvalid
	}
	s.entries[key] = &entry{
		holder:   req.Holder,
		start:    s.clock.Now(),
		duration: req.Duration,
	}
	s.target.Claimed(key, req.Holder)
	return nil
}

// ExtendLease is part of lease.Store.
func (s *leaseStore) ExtendLease(key lease.Key, req lease.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, found := s.entries[key]
	if !found {
		return lease.ErrInvalid
	}
	if entry.holder != req.Holder {
		return lease.ErrInvalid
	}
	now := s.clock.Now()
	expiry := now.Add(req.Duration)
	if !expiry.After(entry.start.Add(entry.duration)) {
		// No extension needed - the lease already expires after the
		// new time.
		return nil
	}
	// entry is a pointer back into the f.entries map, so this update
	// isn't lost.
	entry.start = now
	entry.duration = req.Duration
	return nil
}

// Expire is part of lease.Store.
func (s *leaseStore) ExpireLease(key lease.Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, found := s.entries[key]
	if !found {
		return lease.ErrInvalid
	}
	expiry := entry.start.Add(entry.duration)
	if !s.clock.Now().After(expiry) {
		return lease.ErrInvalid
	}
	delete(s.entries, key)
	s.target.Expired(key)
	return nil
}

// Leases is part of lease.Store.
func (s *leaseStore) Leases() map[lease.Key]lease.Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	results := make(map[lease.Key]lease.Info)
	for key, entry := range s.entries {
		results[key] = lease.Info{
			Holder:   entry.holder,
			Expiry:   entry.start.Add(entry.duration),
			Trapdoor: s.trapdoor(key, entry.holder),
		}
	}
	return results
}

// Refresh is part of lease.Store.
func (s *leaseStore) Refresh() error {
	return nil
}

// PinLease is part of lease.Store.
func (s *leaseStore) PinLease(key lease.Key, entity names.Tag) error {
	return errors.NotImplementedf("lease pinning")
}

// UnpinLease is part of lease.Store.
func (s *leaseStore) UnpinLease(key lease.Key, entity names.Tag) error {
	return errors.NotImplementedf("lease unpinning")
}
