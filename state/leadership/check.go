// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"github.com/juju/names"
)

// check is used to deliver leadership-check requests to a manager's loop
// goroutine on behalf of CheckLeadership.
type check struct {
	serviceName string
	unitName    string
	response    chan Token
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
// returns either a Token that can be used to assert continued leadership in
// the future, or an error.
func (c check) invoke(ch chan<- check) (Token, error) {
	if err := c.validate(); err != nil {
		return nil, errors.Annotatef(err, "cannot check leadership")
	}
	for {
		select {
		case <-c.abort:
			return nil, errStopped
		case ch <- c:
			ch = nil
		case token := <-c.response:
			if token == nil {
				return nil, errors.Errorf("%q is not leader of %q", c.unitName, c.serviceName)
			}
			return token, nil
		}
	}
}

// respond causes the supplied token to be sent back to invoke.
func (c check) respond(token Token) {
	select {
	case <-c.abort:
	case c.response <- token:
	}
}
