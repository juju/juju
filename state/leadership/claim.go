// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/leadership"
)

// claim is used to deliver leadership-claim requests to a manager's loop
// goroutine on behalf of ClaimLeadership.
type claim struct {
	serviceName string
	unitName    string
	duration    time.Duration
	response    chan bool
	abort       <-chan struct{}
}

// validate returns an error if any fields are invalid or missing.
func (c claim) validate() error {
	if !names.IsValidService(c.serviceName) {
		return errors.Errorf("invalid service name %q", c.serviceName)
	}
	if !names.IsValidUnit(c.unitName) {
		return errors.Errorf("invalid unit name %q", c.unitName)
	}
	if c.duration <= 0 {
		return errors.Errorf("invalid duration %v", c.duration)
	}
	if c.response == nil {
		return errors.New("missing response channel")
	}
	if c.abort == nil {
		return errors.New("missing abort channel")
	}
	return nil
}

// invoke sends the claim on the supplied channel and waits for a response.
func (c claim) invoke(ch chan<- claim) error {
	if err := c.validate(); err != nil {
		return errors.Annotatef(err, "cannot claim leadership")
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
