// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
)

// token implements lease.Token.
type token struct {
	leaseKey   lease.Key
	holderName string
	secretary  Secretary
	checks     chan<- check
	stop       <-chan struct{}
}

// Check is part of the lease.Token interface.
func (t token) Check() error {
	// This validation, which could be done at Token creation time, is deferred
	// until this point for historical reasons. In particular, this code was
	// extracted from a *leadership* implementation which has a LeadershipCheck
	// method returning a token; if it returned an error as well it would seem
	// to imply that the method implemented a check itself, rather than a check
	// factory.
	//
	// Fixing that would be great but seems out of scope.
	if err := t.secretary.CheckLease(t.leaseKey); err != nil {
		return errors.Annotatef(err, "cannot check lease %q", t.leaseKey.Lease)
	}
	if err := t.secretary.CheckHolder(t.holderName); err != nil {
		return errors.Annotatef(err, "cannot check holder %q", t.holderName)
	}
	return check{
		leaseKey:   t.leaseKey,
		holderName: t.holderName,
		response:   make(chan error),
		stop:       t.stop,
	}.invoke(t.checks)
}

// check is used to deliver lease-check requests to a manager's loop
// goroutine on behalf of a token (as returned by LeadershipCheck).
type check struct {
	leaseKey   lease.Key
	holderName string
	response   chan error
	stop       <-chan struct{}
}

// invoke sends the check on the supplied channel and waits for an error
// response.
func (c check) invoke(ch chan<- check) error {
	for {
		select {
		case <-c.stop:
			return errStopped
		case ch <- c:
			ch = nil
		case err := <-c.response:
			return errors.Trace(err)
		}
	}
}

// respond notifies the originating invoke of completion status.
func (c check) respond(err error) {
	select {
	case <-c.stop:
	case c.response <- err:
	}
}
