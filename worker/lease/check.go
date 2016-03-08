// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"github.com/juju/errors"
)

// token implements lease.Token.
type token struct {
	leaseName  string
	holderName string
	secretary  Secretary
	checks     chan<- check
	abort      <-chan struct{}
}

// Check is part of the lease.Token interface.
func (t token) Check(trapdoorKey interface{}) error {

	// This validation, which could be done at Token creation time, is deferred
	// until this point for historical reasons. In particular, this code was
	// extracted from a *leadership* implementation which has a LeadershipCheck
	// method returning a token; if it returned an error as well it would seem
	// to imply that the method implemented a check itself, rather than a check
	// factory.
	//
	// Fixing that would be great but seems out of scope.
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

// check is used to deliver lease-check requests to a manager's loop
// goroutine on behalf of a token (as returned by LeadershipCheck).
type check struct {
	leaseName   string
	holderName  string
	trapdoorKey interface{}
	response    chan error
	abort       <-chan struct{}
}

// invoke sends the check on the supplied channel and waits for an error
// response.
func (c check) invoke(ch chan<- check) error {
	for {
		select {
		case <-c.abort:
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
	case <-c.abort:
	case c.response <- err:
	}
}
