// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import "github.com/juju/juju/core/lease"

// block is used to deliver lease-expiry-notification requests to a manager's
// loop goroutine on behalf of BlockUntilLeadershipReleased.
type block struct {
	leaseKey lease.Key
	unblock  chan struct{}
	stop     <-chan struct{}
	cancel   <-chan struct{}
}

// invoke sends the block request on the supplied channel, and waits for the
// unblock channel to be closed.
func (b block) invoke(ch chan<- block) error {
	for {
		select {
		case <-b.stop:
			return errStopped
		case <-b.cancel:
			return lease.ErrWaitCancelled
		case ch <- b:
			ch = nil
		case <-b.unblock:
			return nil
		}
	}
}

// blocks is used to keep track of expiry-notification channels for
// each lease key.
type blocks map[lease.Key][]chan struct{}

// add records the block's unblock channel under the block's lease key.
func (b blocks) add(block block) {
	b[block.leaseKey] = append(b[block.leaseKey], block.unblock)
}

// unblock closes all channels added under the supplied key and removes
// them from blocks.
func (b blocks) unblock(lease lease.Key) {
	unblocks := b[lease]
	delete(b, lease)
	for _, unblock := range unblocks {
		close(unblock)
	}
}
