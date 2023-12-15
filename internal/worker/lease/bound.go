// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"context"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
)

// broker describes methods for manipulating and checking leases.
type broker interface {
	lease.Checker
	lease.Claimer
	lease.Revoker
	lease.Pinner
	lease.Reader
}

// boundManager implements the broker interface.
// It represents a lease manager for a specific namespace and model.
type boundManager struct {
	manager   *Manager
	secretary Secretary
	namespace string
	modelUUID string
}

// Claim is part of the lease.Claimer interface.
func (b *boundManager) Claim(leaseName, holderName string, duration time.Duration) error {
	key := b.leaseKey(leaseName)
	if err := b.secretary.CheckLease(key); err != nil {
		return errors.Annotatef(err, "cannot claim lease %q", leaseName)
	}
	if err := b.secretary.CheckHolder(holderName); err != nil {
		return errors.Annotatef(err, "cannot claim lease for holder %q", holderName)
	}
	if err := b.secretary.CheckDuration(duration); err != nil {
		return errors.Annotatef(err, "cannot claim lease for %s", duration)
	}

	return claim{
		leaseKey:   key,
		holderName: holderName,
		duration:   duration,
		response:   make(chan error),
		stop:       b.manager.tomb.Dying(),
	}.invoke(b.manager.claims)
}

// Revoke is part of the lease.Revoker interface.
func (b *boundManager) Revoke(leaseName, holderName string) error {
	key := b.leaseKey(leaseName)
	if err := b.secretary.CheckLease(key); err != nil {
		return errors.Annotatef(err, "cannot revoke lease %q", leaseName)
	}
	if err := b.secretary.CheckHolder(holderName); err != nil {
		return errors.Annotatef(err, "cannot revoke lease for holder %q", holderName)
	}

	return revoke{
		leaseKey:   key,
		holderName: holderName,
		response:   make(chan error),
		stop:       b.manager.tomb.Dying(),
	}.invoke(b.manager.revokes)
}

// WaitUntilExpired is part of the lease.Claimer interface.
func (b *boundManager) WaitUntilExpired(leaseName string, cancel <-chan struct{}) error {
	key := b.leaseKey(leaseName)
	if err := b.secretary.CheckLease(key); err != nil {
		return errors.Annotatef(err, "cannot wait for lease %q expiry", leaseName)
	}

	return block{
		leaseKey: key,
		unblock:  make(chan struct{}),
		stop:     b.manager.tomb.Dying(),
		cancel:   cancel,
	}.invoke(b.manager.blocks)
}

// Token is part of the lease.Checker interface.
func (b *boundManager) Token(leaseName, holderName string) lease.Token {
	return token{
		leaseKey:   b.leaseKey(leaseName),
		holderName: holderName,
		secretary:  b.secretary,
		checks:     b.manager.checks,
		stop:       b.manager.tomb.Dying(),
	}
}

// Leases (lease.Reader) returns all leases and holders
// in the bound namespace/model.
func (b *boundManager) Leases() (map[string]string, error) {
	ctx, cancel := b.tombContextWithCancel()
	defer cancel()

	leases, err := b.manager.leases(ctx, b.namespace, b.modelUUID)
	return leases, errors.Trace(err)
}

// Pinned (lease.Pinner) returns applications and the entities requiring their
// pinned behaviour, for pinned leases in the bound namespace/model.
func (b *boundManager) Pinned() (map[string][]string, error) {
	ctx, cancel := b.tombContextWithCancel()
	defer cancel()

	pinned, err := b.manager.pinned(ctx, b.namespace, b.modelUUID)
	return pinned, errors.Trace(err)
}

// tombContextWithCancel is a work-around for the bound manager that exposes
// calls to lease store methods outside the worker loop.
// Here, we create a new cancelable context and use that as a parent for
// tomb.Context. This means that killing the tomb will cancel the returned
// context, ensuring that these calls cannot block worker shutdown.
// Every cancel func returned from this method must be called when the
// function its context was passed to returns. Contexts are stored in the tomb,
// which deletes those that are `done` whenever a new context is added.
func (b *boundManager) tombContextWithCancel() (context.Context, func()) {
	parent, cancel := context.WithCancel(context.Background())
	return b.manager.tomb.Context(parent), cancel
}

// Pin (lease.Pinner) sends a pin message to the worker loop.
func (b *boundManager) Pin(leaseName string, entity string) error {
	return errors.Trace(b.pinOp(leaseName, entity, b.manager.pins))
}

// Unpin (lease.Pinner) sends an unpin message to the worker loop.
func (b *boundManager) Unpin(leaseName string, entity string) error {
	return errors.Trace(b.pinOp(leaseName, entity, b.manager.unpins))
}

// pinOp creates a pin instance from the input lease name,
// then sends it on the input channel.
func (b *boundManager) pinOp(leaseName string, entity string, ch chan pin) error {
	return errors.Trace(pin{
		leaseKey: b.leaseKey(leaseName),
		entity:   entity,
		response: make(chan error),
		stop:     b.manager.tomb.Dying(),
	}.invoke(ch))
}

// leaseKey returns a key for the manager's binding and the input lease name.
func (b *boundManager) leaseKey(leaseName string) lease.Key {
	return lease.Key{
		Namespace: b.namespace,
		ModelUUID: b.modelUUID,
		Lease:     leaseName,
	}
}
