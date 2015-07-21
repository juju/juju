// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2/txn"
)

// check is used to deliver leadership-check requests to a manager's loop
// goroutine on behalf of LeadershipCheck.
type check struct {
	serviceName string
	unitName    string
	response    chan txn.Op
	abort       <-chan struct{}
}

// validate returns an error if any fields are invalid or missing.
func (c check) validate() error {
	if !names.IsValidService(c.serviceName) {
		return errors.Errorf("invalid service name %q", c.serviceName)
	}
	if !names.IsValidUnit(c.unitName) {
		return errors.Errorf("invalid unit name %q", c.unitName)
	}
	if c.response == nil {
		return errors.New("missing response channel")
	}
	if c.abort == nil {
		return errors.New("missing abort channel")
	}
	return nil
}

// invoke sends the check on the supplied channel, waits for a response, and
// returns either a txn.Op that can be used to assert continued leadership in
// the future, or an error.
func (c check) invoke(ch chan<- check) (txn.Op, error) {
	if err := c.validate(); err != nil {
		return txn.Op{}, errors.Annotatef(err, "cannot check leadership")
	}
	for {
		select {
		case <-c.abort:
			return txn.Op{}, errStopped
		case ch <- c:
			ch = nil
		case op, ok := <-c.response:
			if !ok {
				return txn.Op{}, errors.Errorf("%q is not leader of %q", c.unitName, c.serviceName)
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
