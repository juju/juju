// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"

	"github.com/juju/juju/core/lease"
)

// LeaseChecker is an interface that checks if a lease is held by a holder.
type LeaseChecker interface {
	lease.Waiter
	lease.Checker
}

// LeaseService creates a base service that offers lease capabilities.
type LeaseService struct {
	leaseChecker func() LeaseChecker
}

// WithLease executes the closure function if the holder to the lease is
// held. As soon as that isn't the case, the context is cancelled and the
// function returns.
// The context must be passed to the closure function to ensure that the
// cancellation is propagated to the closure.
func (s *LeaseService) WithLease(ctx context.Context, leaseName, holderName string, fn func(context.Context) error) error {
	// Holding the lease is quite a complex operation, so we need to ensure that
	// the context is not cancelled before we start the operation.
	if err := ctx.Err(); err != nil {
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Late binding of the lease checker to ensure that the lease checker is
	// available when the function is executed.
	lease := s.leaseChecker()

	// Make the lease token check first, before performing any other operations.
	// There might be a high chance we don't even own the lease.
	if err := lease.Token(leaseName, holderName).Check(); err != nil {
		return err
	}

	// Start will be closed when we start waiting for the lease to expire.
	// If the lease is not held, the function will return immediately and
	// the context will be cancelled.
	start := make(chan struct{})

	waitErr := make(chan error)
	go func() {
		defer cancel()

		// This guards against the case that the lease has changed state
		// before we run the function.

		select {
		case <-ctx.Done():
			return
		case waitErr <- lease.WaitUntilExpired(ctx, leaseName, start):
		}
	}()

	// Ensure that the lease is held by the holder before proceeding.
	// We're guaranteed that the lease is held by the holder, otherwise the
	// context will have been cancelled.
	runErr := make(chan error)
	go func() {
		defer cancel()

		select {
		case <-ctx.Done():
			return
		case <-start:
		}

		if err := lease.Token(leaseName, holderName).Check(); err != nil {
			select {
			case <-ctx.Done():
			case runErr <- err:
			}
			return
		}

		select {
		case <-ctx.Done():
		case runErr <- fn(ctx):
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-waitErr:
		return err
	case err := <-runErr:
		return err
	}
}
