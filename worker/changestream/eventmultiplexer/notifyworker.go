// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"sync/atomic"

	"github.com/juju/juju/core/changestream"
	"gopkg.in/tomb.v2"
)

type Signaller interface {
	Signal(changes []changestream.ChangeEvent)
}

type notifyWorker struct {
	tomb tomb.Tomb

	notary      chan struct{}
	numNotaries int64
}

func newNotifyWorker(size int64) *notifyWorker {
	w := &notifyWorker{
		numNotaries: size,
		notary:      make(chan struct{}),
	}
	w.tomb.Go(w.loop)
	return w
}

func (s *notifyWorker) Notify(sub *subscription, changes []changestream.ChangeEvent) {
	s.tomb.Go(func() error {
		sub.Signal(changes)
		atomic.AddInt64(&s.numNotaries, -1)

		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		case s.notary <- struct{}{}:
		}
		return nil
	})
}

func (s *notifyWorker) Kill() {
	s.tomb.Kill(nil)
}

func (s *notifyWorker) Wait() error {
	return s.tomb.Wait()
}

func (s *notifyWorker) loop() error {
	// We need at least one loop, otherwise the tomb will be considered dead
	// when adding itself to the catacomb.
	for {
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		case <-s.notary:
			if atomic.LoadInt64(&s.numNotaries) == 0 {
				return nil
			}
		}
	}
}
