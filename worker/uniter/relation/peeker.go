// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker/uniter/hook"
)

// Peeker maintains a HookSource, and allows an external client to inspect
// and consume, or reject, the head of the queue.
type Peeker interface {
	// Peeks returns a channel on which Peeks are delivered. The receiver of a
	// Peek must Consume or Reject the Peek before further peeks will be sent.
	Peeks() <-chan Peek
	// Stop stops the Peeker's HookSource, and prevents any further Peeks from
	// being delivered.
	Stop() error
}

// Peek exists to support Peeker and has no independent meaning or existence.
// Receiving a Peek from a Peeker's Peek channel implies acceptance of the
// responsibility to either Consume or Reject the Peek.
type Peek interface {
	// HookInfo returns information about the hook at the head of the queue.
	HookInfo() hook.Info
	// Consume pops the hook from the head of the queue and makes new Peeks
	// available.
	Consume()
	// Reject makes new Peeks available.
	Reject()
}

// NewPeeker returns a new Peeker providing a view of the supplied source
// (of which it takes ownership).
func NewPeeker(source hook.Source) Peeker {
	p := &peeker{
		peeks: make(chan Peek),
	}
	go func() {
		defer p.tomb.Done()
		defer close(p.peeks)
		defer watcher.Stop(source, &p.tomb)
		p.tomb.Kill(p.loop(source))
	}()
	return p
}

// peeker implements Peeker.
type peeker struct {
	tomb  tomb.Tomb
	peeks chan Peek
}

// Peeks is part of the Peeker interface.
func (p *peeker) Peeks() <-chan Peek {
	return p.peeks
}

// Stop is part of the Peeker interface.
func (p *peeker) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}

// loop delivers events from the source's Changes channel to its Update method,
// continually, unless a Peek is active.
func (p *peeker) loop(source hook.Source) error {
	for {
		var next *peek
		var peeks chan Peek
		if !source.Empty() {
			peeks = p.peeks
			next = &peek{
				source: source,
				done:   make(chan struct{}),
			}
		}
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case peeks <- next:
			select {
			case <-p.tomb.Dying():
			case <-next.done:
			}
		case change, ok := <-source.Changes():
			if !ok {
				return errors.New("hook source stopped providing updates")
			}
			if err := change.Apply(); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// peek implements Peek.
type peek struct {
	source hook.Source
	done   chan struct{}
}

// HookInfo is part of the Peek interface.
func (p *peek) HookInfo() hook.Info {
	return p.source.Next()
}

// Consume is part of the Peek interface.
func (p *peek) Consume() {
	p.source.Pop()
	close(p.done)
}

// Reject is part of the Peek interface.
func (p *peek) Reject() {
	close(p.done)
}
