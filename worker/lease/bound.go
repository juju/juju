// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/lease"
)

// broker describes methods for manipulating and checking leases.
type broker interface {
	lease.Checker
	lease.Claimer
	lease.Pinner
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
		stop:       b.manager.catacomb.Dying(),
	}.invoke(b.manager.claims)
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
		stop:     b.manager.catacomb.Dying(),
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
		stop:       b.manager.catacomb.Dying(),
	}
}

// Pin (lease.Pinner) sends a pin message to the worker loop.
func (b *boundManager) Pin(leaseName string, entity names.Tag) error {
	return errors.Trace(b.pinOp(leaseName, entity, b.manager.pins))
}

// Unpin (lease.Pinner) sends an unpin message to the worker loop.
func (b *boundManager) Unpin(leaseName string, entity names.Tag) error {
	return errors.Trace(b.pinOp(leaseName, entity, b.manager.unpins))
}

// pinOp creates a pin instance from the input lease name,
// then sends it on the input channel.
func (b *boundManager) pinOp(leaseName string, entity names.Tag, ch chan pin) error {
	return errors.Trace(pin{
		leaseKey: b.leaseKey(leaseName),
		entity:   entity,
		response: make(chan error),
		stop:     b.manager.catacomb.Dying(),
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
