// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"github.com/juju/names"
)

// block is used to deliver leaderlessness-notification requests to a manager's
// loop goroutine on behalf of BlockUntilLeadershipReleased.
type block struct {
	serviceName string
	unblock     chan struct{}
	abort       <-chan struct{}
}

// validate returns an error if any fields are invalid or missing.
func (b block) validate() error {
	if !names.IsValidService(b.serviceName) {
		return errors.Errorf("invalid service name %q", b.serviceName)
	}
	if b.unblock == nil {
		return errors.New("missing unblock channel")
	}
	if b.abort == nil {
		return errors.New("missing abort channel")
	}
	return nil
}

// invoke sends the block request on the supplied channel, and waits for the
// unblock channel to be closed.
func (b block) invoke(ch chan<- block) error {
	if err := b.validate(); err != nil {
		return errors.Annotatef(err, "cannot wait for leaderlessness")
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
	b[block.serviceName] = append(b[block.serviceName], block.unblock)
}

// unblock closes all channels added under the supplied name and removes
// them from blocks.
func (b blocks) unblock(serviceName string) {
	unblocks := b[serviceName]
	delete(b, serviceName)
	for _, unblock := range unblocks {
		close(unblock)
	}
}
