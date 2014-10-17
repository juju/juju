// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"launchpad.net/tomb"

	"github.com/juju/errors"

	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker/uniter/hook"
)

// HookSender maintains a HookSource and delivers its hooks via a channel.
type HookSender interface {
	Stop() error
}

// NewHookSender starts sending hooks from source onto the out channel, and will
// continue to do so until Stop()ped (or the source is exhausted). NewHookSender
// takes ownership of the supplied source, and responsibility for cleaning it up;
// but it will not close the out channel.
func NewHookSender(out chan<- hook.Info, source HookSource) HookSender {
	sender := &hookSender{
		out: out,
	}
	go func() {
		defer sender.tomb.Done()
		defer watcher.Stop(source, &sender.tomb)
		sender.tomb.Kill(sender.loop(source))
	}()
	return sender
}

// hookSender implements HookSender.
type hookSender struct {
	tomb tomb.Tomb
	out  chan<- hook.Info
}

// Stop stops the HookSender and returns any errors encountered during
// operation or while shutting down.
func (sender *hookSender) Stop() error {
	sender.tomb.Kill(nil)
	return sender.tomb.Wait()
}

// loop synchronously delivers the source's change events to its update method,
// and, whenever the source is nonempty, repeatedly sends its first scheduled
// event on the out chan (and pops it from the source).
func (sender *hookSender) loop(source HookSource) error {
	var next hook.Info
	var out chan<- hook.Info
	for {
		if source.Empty() {
			out = nil
		} else {
			out = sender.out
			next = source.Next()
		}
		select {
		case <-sender.tomb.Dying():
			return tomb.ErrDying
		case out <- next:
			source.Pop()
		case change, ok := <-source.Changes():
			if !ok {
				return errors.New("hook source stopped providing updates")
			}
			if err := source.Update(change); err != nil {
				return errors.Trace(err)
			}
		}
	}
}
