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
	leaseName  string
	holderName string
	duration   time.Duration
	response   chan bool
	abort      <-chan struct{}
}

// invoke sends the claim on the supplied channel and waits for a response.
func (c claim) invoke(ch chan<- claim) error {
	for {
		select {
		case <-c.abort:
			return errStopped
		case ch <- c:
			ch = nil
		case success := <-c.response:
			if !success {
				return lease.ErrClaimDenied
			}
			return nil
		}
	}
}

// respond causes the supplied success value to be sent back to invoke.
func (c claim) respond(success bool) {
	select {
	case <-c.abort:
	case c.response <- success:
	}
}
