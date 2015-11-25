// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/leadership"
)

// claim is used to deliver leadership-claim requests to a manager's loop
// goroutine on behalf of ClaimLeadership.
type claim struct {
	leaseName  string
	holderName string
	duration   time.Duration
	response   chan bool
	abort      <-chan struct{}
}

// validate returns an error if any fields are invalid or missing.
func (c claim) validate(secretary Secretary) error {
	if err := secretary.CheckLease(c.leaseName); err != nil {
		return errors.Annotate(err, "lease name")
	}
	if err := secretary.CheckHolder(c.holderName); err != nil {
		return errors.Annotate(err, "holder name")
	}
	if c.duration <= 0 {
		return errors.NotValidf("duration %v", c.duration)
	}
	if c.response == nil {
		return errors.NotValidf("nil response channel")
	}
	if c.abort == nil {
		return errors.NotValidf("nil abort channel")
	}
	return nil
}

// invoke sends the claim on the supplied channel and waits for a response.
func (c claim) invoke(secretary Secretary, ch chan<- claim) error {
	if err := c.validate(secretary); err != nil {
		return errors.Annotatef(err, "cannot claim lease")
	}
	for {
		select {
		case <-c.abort:
			return errStopped
		case ch <- c:
			ch = nil
		case success := <-c.response:
			if !success {
				return leadership.ErrClaimDenied
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
