// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
)

// token implements leadership.Token.
type token struct {
	leaseName  string
	holderName string
	secretary  Secretary
	checks     chan<- check
	abort      <-chan struct{}
}

// Check is part of the leadership.Token interface.
func (t token) Check(trapdoorKey interface{}) error {

	// This validation, which could be done at Token creation time, is deferred
	// until this point so as not to pollute the LeadershipCheck signature with
	// an error that might cause clients to assume LeadershipCheck actually
	// checked leadership.
	//
	// Um, we should probably name it something different, shouldn't we?
	if err := t.secretary.CheckLease(t.leaseName); err != nil {
		return errors.Annotatef(err, "cannot check lease %q", t.leaseName)
	}
	if err := t.secretary.CheckHolder(t.holderName); err != nil {
		return errors.Annotatef(err, "cannot check holder %q", t.holderName)
	}
	return check{
		leaseName:   t.leaseName,
		holderName:  t.holderName,
		trapdoorKey: trapdoorKey,
		response:    make(chan error),
		abort:       t.abort,
	}.invoke(t.checks)
}

// check is used to deliver leadership-check requests to a manager's loop
// goroutine on behalf of a token (as returned by LeadershipCheck).
type check struct {
	leaseName   string
	holderName  string
	trapdoorKey interface{}
	response    chan error
	abort       <-chan struct{}
}

// invoke sends the check on the supplied channel, waits for a response, and
// returns either a txn.Op that can be used to assert continued leadership in
// the future, or an error.
func (c check) invoke(ch chan<- check) error {
	for {
		select {
		case <-c.abort:
			return errStopped
		case ch <- c:
			ch = nil
		case err := <-c.response:
			if err == ErrLeaseNotHeld {
				return errors.Errorf("%q is not leader of %q", c.holderName, c.leaseName)
			}
			return errors.Trace(err)
		}
	}
}

// respond notifies the originating invoke of completion status.
func (c check) respond(err error) {
	select {
	case <-c.abort:
	case c.response <- err:
	}
}
