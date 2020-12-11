// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/core/watcher"
)

// NotifyWatcherInterface defines the methods of notifyWatcher.
type NotifyWatcherInterface interface {
	watcher.CoreWatcher
	Changes() watcher.NotifyChannel
}

// notifyWatcher reports changes of ecs resources.
type notifyWatcher struct {
	clock    jujuclock.Clock
	catacomb catacomb.Catacomb

	name    string
	checker func() (bool, error)
	out     chan struct{}
}

func newNotifyWatcher(name string, clock jujuclock.Clock, checker func() (bool, error)) (NotifyWatcherInterface, error) {
	w := &notifyWatcher{
		clock:   clock,
		name:    name,
		checker: checker,
		out:     make(chan struct{}),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

const sendDelay = 1 * time.Second

func (w *notifyWatcher) loop() error {
	defer close(w.out)

	// Set out now so that initial event is sent.
	out := w.out

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-w.clock.After(sendDelay):
			ok, err := w.checker()
			if err != nil {
				logger.Errorf("checking failed: %v", err)
				continue
			}
			if ok {
				out = w.out
			}
		case out <- struct{}{}:
			logger.Debugf("fire notify watcher for %v", w.name)
			out = nil
		}
	}
}

// Changes returns the event channel for this watcher.
func (w *notifyWatcher) Changes() watcher.NotifyChannel {
	return w.out
}

// Kill asks the watcher to stop without waiting for it do so.
func (w *notifyWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *notifyWatcher) Wait() error {
	return w.catacomb.Wait()
}
