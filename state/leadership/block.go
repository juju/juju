// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
)

// block is used to deliver leaderlessness-notification requests to a manager's
// loop goroutine on behalf of BlockUntilLeadershipReleased.
type block struct {
	leaseName string
	unblock   chan struct{}
	abort     <-chan struct{}
}

// validate returns an error if any fields are invalid or missing.
func (b block) validate(secretary Secretary) error {
	if err := secretary.CheckLease(b.leaseName); err != nil {
		return errors.Annotate(err, "lease name")
	}
	if b.unblock == nil {
		return errors.NotValidf("nil unblock channel")
	}
	if b.abort == nil {
		return errors.NotValidf("nil abort channel")
	}
	return nil
}

// invoke sends the block request on the supplied channel, and waits for the
// unblock channel to be closed.
func (b block) invoke(secretary Secretary, ch chan<- block) error {
	if err := b.validate(secretary); err != nil {
		return errors.Annotatef(err, "cannot wait for lease expiry")
	}
	for {
		select {
		case <-b.abort:
			return errStopped
		case ch <- b:
			ch = nil
		case <-b.unblock:
			return nil
		}
	}
}

// blocks is used to keep track of leaderlessness-notification channels for
// each service name.
type blocks map[string][]chan struct{}

// add records the block's unblock channel under the block's service name.
func (b blocks) add(block block) {
	b[block.leaseName] = append(b[block.leaseName], block.unblock)
}

// unblock closes all channels added under the supplied name and removes
// them from blocks.
func (b blocks) unblock(leaseName string) {
	unblocks := b[leaseName]
	delete(b, leaseName)
	for _, unblock := range unblocks {
		close(unblock)
	}
}
