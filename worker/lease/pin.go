// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/lease"
)

// pin is used to deliver lease pinning and unpinning requests to a manager's
// worker loop on behalf of PinLeadership and UnpinLeadership.
type pin struct {
	leaseKey lease.Key
	entity   names.Tag
	response chan error
	stop     <-chan struct{}
}

// invoke sends the claim on the supplied channel and waits for a response.
func (c pin) invoke(ch chan<- pin) error {
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
func (c pin) respond(err error) {
	select {
	case <-c.stop:
	case c.response <- err:
	}
}
