// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
)

// check is used to deliver leadership-check requests to a manager's loop
// goroutine on behalf of LeadershipCheck.
type check struct {
	leaseName  string
	holderName string
	response   chan txn.Op
	abort      <-chan struct{}
}

// validate returns an error if any fields are invalid or missing.
func (c check) validate(secretary Secretary) error {
	if err := secretary.CheckLease(c.leaseName); err != nil {
		return errors.Annotate(err, "lease name")
	}
	if err := secretary.CheckHolder(c.holderName); err != nil {
		return errors.Annotate(err, "holder name")
	}
	if c.response == nil {
		return errors.NotValidf("nil response channel")
	}
	if c.abort == nil {
		return errors.NotValidf("nil abort channel")
	}
	return nil
}

// invoke sends the check on the supplied channel, waits for a response, and
// returns either a txn.Op that can be used to assert continued leadership in
// the future, or an error.
func (c check) invoke(secretary Secretary, ch chan<- check) (txn.Op, error) {
	if err := c.validate(secretary); err != nil {
		return txn.Op{}, errors.Annotatef(err, "cannot check lease")
	}
	for {
		select {
		case <-c.abort:
			return txn.Op{}, errStopped
		case ch <- c:
			ch = nil
		case op, ok := <-c.response:
			if !ok {
				return txn.Op{}, errors.Errorf("%q does not hold lease %q", c.holderName, c.leaseName)
			}
			return op, nil
		}
	}
}

// succeed sends the supplied operation back to the originating invoke.
func (c check) succeed(op txn.Op) {
	select {
	case <-c.abort:
	case c.response <- op:
	}
}

// fail causes the originating invoke to return an error indicating non-leadership.
func (c check) fail() {
	close(c.response)
}
