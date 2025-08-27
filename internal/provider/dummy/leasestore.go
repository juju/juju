// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
)

// leaseStore implements lease.Store as simply as possible for use in
// the dummy provider. Heavily cribbed from raftlease.FSM.
type leaseStore struct {
	mu      sync.Mutex
	clock   clock.Clock
	entries map[lease.Key]*entry
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

func newLeaseStore(clock clock.Clock) *leaseStore {
	return &leaseStore{
		clock:   clock,
		entries: make(map[lease.Key]*entry),
	}
}

// ClaimLease is part of lease.Store.
func (s *leaseStore) ClaimLease(_ context.Context, key lease.Key, req lease.Request) error {
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
	return nil
}

// ExtendLease is part of lease.Store.
func (s *leaseStore) ExtendLease(_ context.Context, key lease.Key, req lease.Request) error {
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

// RevokeLease is part of lease.Store.
func (s *leaseStore) RevokeLease(_ context.Context, key lease.Key, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, found := s.entries[key]
	if !found {
		return lease.ErrInvalid
	}
	delete(s.entries, key)
	return nil
}

// Leases is part of lease.Store.
func (s *leaseStore) Leases(_ context.Context, keys ...lease.Key) (map[lease.Key]lease.Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filter := make(map[lease.Key]bool)
	filtering := len(keys) > 0
	if filtering {
		for _, key := range keys {
			filter[key] = true
		}
	}

	results := make(map[lease.Key]lease.Info)
	for key, entry := range s.entries {
		if filtering && !filter[key] {
			continue
		}

		results[key] = lease.Info{
			Holder: entry.holder,
			Expiry: entry.start.Add(entry.duration),
		}
	}
	return results, nil
}

// LeaseGroup is part of lease.Store.
func (s *leaseStore) LeaseGroup(ctx context.Context, namespace, modelUUID string) (map[lease.Key]lease.Info, error) {
	leases, _ := s.Leases(ctx)
	if len(leases) == 0 {
		return leases, nil
	}
	results := make(map[lease.Key]lease.Info)
	for key, info := range leases {
		if key.Namespace == namespace && key.ModelUUID == modelUUID {
			results[key] = info
		}
	}
	return results, nil
}

// PinLease is part of lease.Store.
func (s *leaseStore) PinLease(context.Context, lease.Key, string) error {
	return errors.NotImplementedf("lease pinning")
}

// UnpinLease is part of lease.Store.
func (s *leaseStore) UnpinLease(context.Context, lease.Key, string) error {
	return errors.NotImplementedf("lease unpinning")
}

// Pinned is part of the Store interface.
func (s *leaseStore) Pinned(context.Context) (map[lease.Key][]string, error) {
	return nil, nil
}
