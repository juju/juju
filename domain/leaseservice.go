// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
)

type errToken struct {
	err error
}

func (e errToken) Check() error { return e.err }

// LeaseService creates a base service that offers lease capabilities.
type LeaseService struct {
	leaseManager lease.ModelLeaseManagerGetter
}

// NewLeaseService creates a new LeaseService.
func NewLeaseService(leaseManager lease.ModelLeaseManagerGetter) *LeaseService {
	return &LeaseService{
		leaseManager: leaseManager,
	}
}

// LeadershipCheck returns a token that can be used to check if the input unit
// is the leader of the input application.
func (s *LeaseService) LeadershipCheck(appName, unitName string) leadership.Token {
	leaseManager, err := s.leaseManager.GetLeaseManager()
	if err != nil {
		return errToken{err: err}
	}

	return leaseManager.Token(appName, unitName)
}

// WithLeader executes the closure function if the input unit is leader of the
// input application.
// As soon as that isn't the case, the context is cancelled and the function
// returns.
// The context must be passed to the closure function to ensure that the
// cancellation is propagated to the closure.
//
// Returns an error satisfying [corelease.ErrNotHeld] if the unit is not the leader.
func (s *LeaseService) WithLeader(
	ctx context.Context, appName, unitName string, fn func(context.Context) error,
) error {
	// Holding the lease is quite a complex operation, so we need to ensure that
	// the context is not cancelled before we start the operation.
	if err := ctx.Err(); err != nil {
		return errors.Errorf("leader pre-checking").Add(ctx.Err())
	}

	leaseManager, err := s.leaseManager.GetLeaseManager()
	if err != nil {
		return errors.Errorf("getting lease manager: %w", err)
	}

	// The leaseCtx will be cancelled when the lease is no longer held by the
	// leaseholder. This may or may not be the same as the holderName for the
	// lease. That check is done by the Token checker.
	leaseCtx, leaseCancel := context.WithCancel(ctx)
	defer leaseCancel()

	// Start will be closed when we start waiting for the lease to expire.
	// If the lease is not held, the function will return immediately and
	// the context will be cancelled.
	start := make(chan struct{})

	// WaitUntilExpired will be run against the leaseName. To ensure that after
	// we've waited that we still hold the lease, we need to check that the
	// lease is still held by the holder. Then we can guarantee that the lease
	// is held by the holder for the duration of the function. Although
	// convoluted this is necessary to ensure that the lease is held by the
	// holder for the duration of the function. The context will be cancelled
	// when the lease is no longer held by the leaseholder for the lease name.

	waitCtx, waitCancel := context.WithCancel(ctx)
	defer waitCancel()

	waitErr := make(chan error)
	go func() {
		// This guards against the case that the lease has changed state
		// before we run the function.
		err := leaseManager.WaitUntilExpired(waitCtx, appName, start)

		// Ensure that the lease context is cancelled when the wait has
		// completed. We do this as quick as possible to ensure that the
		// function is cancelled as soon as possible.
		leaseCancel()

		// The waitErr might not be read, so we need to provide another way
		// to collapse the goroutine. Using the waitCtx this goroutine will
		// be cancelled when the function is complete.
		select {
		case <-waitCtx.Done():
			return
		case waitErr <- errors.Errorf("waiting for leadership to expire: %w", err):
		}
	}()

	select {
	case <-leaseCtx.Done():
		// If the leaseCtx is cancelled, then the waiting for the lease to
		// expire finished unexpectedly. Return the context error.
		return errors.Errorf("waiting for leadership finished before execution").Add(leaseCtx.Err())
	case err := <-waitErr:
		if err == nil {
			// This shouldn't happen, but if it does, we need to return an
			// error. If we're attempting to wait whilst holding the lease,
			// before running the function and then wait return nil, we don't
			// know if the lease is held by the holder or what state we're in.
			return errors.Errorf("unable to wait for leadership to expire whilst holding lease")
		}
		if ctxErr := leaseCtx.Err(); ctxErr != nil {
			return errors.Errorf("waiting for leadership finished before execution").Add(ctxErr)
		}
		return err
	case <-start:
	}

	// Ensure that the lease is held by the holder before proceeding.
	// We're guaranteed that the lease is held by the holder, otherwise the
	// context will have been cancelled.
	if err := leaseManager.Token(appName, unitName).Check(); err != nil {
		return errors.Errorf("checking lease token: %w", err)
	}

	// The leaseCtx will be cancelled when the lease is no longer held. This
	// will ensure that the function is cancelled when the lease is no longer
	// held.
	if err := fn(leaseCtx); err != nil {
		return errors.Errorf("executing leadership func: %w", err)
	}
	return nil
}

// RevokeLeadership revokes the leadership for the input application and unit.
// If the unit is not the leader, it returns an error.
func (s *LeaseService) RevokeLeadership(appName string, unitName unit.Name) error {
	leaseManager, err := s.leaseManager.GetLeaseManager()
	if err != nil {
		return errors.Errorf("getting lease manager: %w", err)
	}

	if err := leaseManager.Revoke(appName, unitName.String()); errors.Is(err, lease.ErrNotHeld) {
		return errors.Errorf("revoking leadership: %w", err).Add(leadership.ErrClaimNotHeld)
	} else if err != nil {
		return errors.Errorf("revoking leadership: %w", err)
	}
	return nil
}
