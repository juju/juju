// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence methods for storage.
type State interface {
	Leases(context.Context, ...lease.Key) (map[lease.Key]lease.Info, error)
	ClaimLease(context.Context, uuid.UUID, lease.Key, lease.Request) error
	ExtendLease(context.Context, lease.Key, lease.Request) error
	RevokeLease(context.Context, lease.Key, string) error
	LeaseGroup(context.Context, string, string) (map[lease.Key]lease.Info, error)
	PinLease(context.Context, lease.Key, string) error
	UnpinLease(context.Context, lease.Key, string) error
	Pinned(context.Context) (map[lease.Key][]string, error)
	ExpireLeases(context.Context) error
}

// Service provides the API for working with external controllers.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// Leases (lease.Store) returns all leases in the database,
// optionally filtering using the input keys.
func (s *Service) Leases(ctx context.Context, keys ...lease.Key) (map[lease.Key]lease.Info, error) {
	// TODO (manadart 2022-11-30): We expect the variadic `keys` argument to be
	// length 0 or 1. It was a work-around for design constraints at the time.
	// Either filter the result here for len(keys) > 1, or fix the design.
	// As it is, there are no upstream usages for more than one key,
	// so we just lock in that behaviour.
	if len(keys) > 1 {
		return nil, errors.Errorf("filtering with more than one lease key %w", coreerrors.NotSupported)
	}

	return s.st.Leases(ctx, keys...)
}

// ClaimLease (lease.Store) claims the lease indicated by the input key,
// for the holder and duration indicated by the input request.
// The lease must not already be held, otherwise an error is returned.
func (s *Service) ClaimLease(ctx context.Context, key lease.Key, req lease.Request) error {
	if err := req.Validate(); err != nil {
		return errors.Capture(err)
	}
	uuid, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	return s.st.ClaimLease(ctx, uuid, key, req)
}

// ExtendLease (lease.Store) ensures the input lease will be held for at least
// the requested duration starting from now.
// If the input holder does not currently hold the lease, an error is returned.
func (s *Service) ExtendLease(ctx context.Context, key lease.Key, req lease.Request) error {
	if err := req.Validate(); err != nil {
		return errors.Capture(err)
	}

	return s.st.ExtendLease(ctx, key, req)
}

// RevokeLease (lease.Store) deletes the lease from the store,
// provided it exists and is held by the input holder.
// If either of these conditions is false, an error is returned.
func (s *Service) RevokeLease(ctx context.Context, key lease.Key, holder string) error {
	return s.st.RevokeLease(ctx, key, holder)
}

// LeaseGroup (lease.Store) returns all leases
// for the input namespace and model.
func (s *Service) LeaseGroup(ctx context.Context, namespace, modelUUID string) (map[lease.Key]lease.Info, error) {
	return s.st.LeaseGroup(ctx, namespace, modelUUID)
}

// PinLease (lease.Store) adds the input entity into the lease_pin table
// to indicate that the lease indicated by the input key must not expire,
// and that this entity requires such behaviour.
func (s *Service) PinLease(ctx context.Context, key lease.Key, entity string) error {
	return s.st.PinLease(ctx, key, entity)
}

// UnpinLease (lease.Store) removes the record indicated by the input
// key and entity from the lease pin table, indicating that the entity
// no longer requires the lease to be pinned.
// When there are no entities associated with a particular lease,
// it is determined not to be pinned, and can expire normally.
func (s *Service) UnpinLease(ctx context.Context, key lease.Key, entity string) error {
	return s.st.UnpinLease(ctx, key, entity)
}

// Pinned (lease.Store) returns all leases that are currently pinned,
// and the entities requiring such behaviour for them.
func (s *Service) Pinned(ctx context.Context) (map[lease.Key][]string, error) {
	return s.st.Pinned(ctx)
}

// ExpireLeases ensures that all leases that have expired are deleted from
// the store.
func (s *Service) ExpireLeases(ctx context.Context) error {
	return s.st.ExpireLeases(ctx)
}
