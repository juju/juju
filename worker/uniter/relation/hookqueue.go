// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"launchpad.net/tomb"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/worker/uniter/hook"
)

// HookQueue instances are expected to send hook.Info events onto a channel that
// consumes them for execution.
type HookQueue interface {
	Stop() error
}

// hookQueue implements the channel manipulation of the HookQueue, delegating
// queue implementation to its driver.
type hookQueue struct {
	tomb   tomb.Tomb
	out    chan<- hook.Info
	driver queueDriver
}

// queueDriver exposes pure queue-management operations intended for use with
// hookQueue.
type queueDriver interface {

	// empty returns whether the queue is empty.
	empty() bool

	// update modifies the state of the queue to reflect the supplied state
	// change. Any call to update invalidates previously-obtained results from
	// next. It will only be called if the hookQueue is run with a non-nil
	// updates channel.
	update(params.RelationUnitsChange)

	// next returns information about the hook at the head of the queue. It
	// will panic if the queue is empty. Calls to pop or update invalidate
	// values previously obtained from next.
	next() hook.Info

	// pop removes the hook at the head of the queue. It will panic if the
	// queue is empty. Any call to pop invalidates previously-obtained results
	// from next.
	pop()
}

// loop reads updates from the supplied channel and passes them into the
// driver; and repeatedly reads from the head of the driver's queue, passing
// values thus obtained through the out chan and popping them from the driver
// when sent. The updates can be nil; if so, the loop will terminate without
// error when the driver queue empties out.
func (q *hookQueue) loop(updates <-chan params.RelationUnitsChange) error {
	var next hook.Info
	var out chan<- hook.Info
	for {
		if q.driver.empty() {
			if updates == nil {
				return nil
			}
			out = nil
		} else {
			out = q.out
			next = q.driver.next()
		}
		select {
		case <-q.tomb.Dying():
			return tomb.ErrDying
		case out <- next:
			q.driver.pop()
		case update, ok := <-updates:
			if !ok {
				return errWatcherStopped
			}
			q.driver.update(update)
		}
	}
}

// Stop stops the hookQueue and returns any errors encountered during
// operation or while shutting down.
func (q *hookQueue) Stop() error {
	q.tomb.Kill(nil)
	return q.tomb.Wait()
}
