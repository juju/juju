// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

// block is used to deliver lease-expiry-notification requests to a manager's
// loop goroutine on behalf of BlockUntilLeadershipReleased.
type block struct {
	leaseName string
	unblock   chan struct{}
	abort     <-chan struct{}
}

// invoke sends the block request on the supplied channel, and waits for the
// unblock channel to be closed.
func (b block) invoke(ch chan<- block) error {
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

// blocks is used to keep track of expiry-notification channels for
// each lease name.
type blocks map[string][]chan struct{}

// add records the block's unblock channel under the block's lease name.
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
