// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
)

// checkerClaimer combines the lease.Checker and lease.Claimer
// interfaces.
type checkerClaimer interface {
	lease.Checker
	lease.Claimer
}

// boundManager implements lease.Claimer and lease.Checker - it
// represents a lease manager for a specific namespace and model.
type boundManager struct {
	manager   *Manager
	secretary Secretary
	namespace string
	modelUUID string
}

// Claim is part of the lease.Claimer interface.
func (b *boundManager) Claim(leaseName, holderName string, duration time.Duration) error {
	key := lease.Key{
		Namespace: b.namespace,
		ModelUUID: b.modelUUID,
		Lease:     leaseName,
	}
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
	key := lease.Key{
		Namespace: b.namespace,
		ModelUUID: b.modelUUID,
		Lease:     leaseName,
	}
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
	key := lease.Key{
		Namespace: b.namespace,
		ModelUUID: b.modelUUID,
		Lease:     leaseName,
	}
	return token{
		leaseKey:   key,
		holderName: holderName,
		secretary:  b.secretary,
		checks:     b.manager.checks,
		stop:       b.manager.catacomb.Dying(),
	}
}
