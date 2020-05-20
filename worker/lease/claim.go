// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/juju/core/lease"
)

// claim is used to deliver lease-claim requests to a manager's loop
// goroutine on behalf of ClaimLeadership.
type claim struct {
	leaseKey   lease.Key
	holderName string
	duration   time.Duration
	response   chan error
	stop       <-chan struct{}
}

// invoke sends the claim on the supplied channel and waits for a response.
func (c claim) invoke(ch chan<- claim) error {
	for {
		select {
		case <-c.stop:
			return errStopped
		case ch <- c:
			ch = nil
		case err := <-c.response:
			return err
		}
	}
}

// respond causes the supplied success value to be sent back to invoke.
func (c claim) respond(err error) {
	select {
	case <-c.stop:
	case c.response <- err:
	}
}

// revoke is used to deliver lease-revoke requests to a manager's loop
// goroutine on behalf of RevokeLeadership.
type revoke struct {
	leaseKey   lease.Key
	holderName string
	response   chan error
	stop       <-chan struct{}
}

// invoke sends the revocation on the supplied channel and waits for a response.
func (c revoke) invoke(ch chan<- revoke) error {
	for {
		select {
		case <-c.stop:
			return errStopped
		case ch <- c:
			ch = nil
		case err := <-c.response:
			return err
		}
	}
}

// respond causes the supplied success value to be sent back to invoke.
func (c revoke) respond(err error) {
	select {
	case <-c.stop:
	case c.response <- err:
	}
}
