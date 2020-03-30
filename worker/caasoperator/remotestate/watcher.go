// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1/catacomb"
)

var logger = loggo.GetLogger("juju.worker.caasoperator.remotestate")

// RemoteStateWatcher collects application information from separate state watchers,
// and updates a Snapshot which is sent on a channel upon change.
type RemoteStateWatcher struct {
	config      WatcherConfig
	application string

	catacomb catacomb.Catacomb

	out     chan struct{}
	mu      sync.Mutex
	current Snapshot
}

// WatcherConfig holds configuration parameters for the
// remote state watcher.
type WatcherConfig struct {
	Application        string
	CharmGetter        charmGetter
	ApplicationWatcher applicationWatcher
}

// NewWatcher returns a RemoteStateWatcher that handles state changes pertaining to the
// supplied application.
func NewWatcher(config WatcherConfig) (*RemoteStateWatcher, error) {
	w := &RemoteStateWatcher{
		config:      config,
		application: config.Application,
		// Note: it is important that the out channel be buffered!
		// The remote state watcher will perform a non-blocking send
		// on the channel to wake up the observer. It is non-blocking
		// so that we coalesce events while the observer is busy.
		out:     make(chan struct{}, 1),
		current: Snapshot{},
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			return w.loop()
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *RemoteStateWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *RemoteStateWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *RemoteStateWatcher) RemoteStateChanged() <-chan struct{} {
	return w.out
}

func (w *RemoteStateWatcher) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	snapshot := w.current
	return snapshot
}

func (w *RemoteStateWatcher) loop() (err error) {
	defer func() {
		if errors.IsNotFound(err) {
			logger.Debugf("ignoring error %v and exit", err)
			err = nil
		}
	}()

	var requiredEvents int

	var seenApplicationChange bool
	applicationw, err := w.config.ApplicationWatcher.Watch(w.config.Application)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(applicationw); err != nil {
		return errors.Trace(err)
	}
	applicationChanges := applicationw.Changes()
	requiredEvents++

	var eventsObserved int
	observedEvent := func(flag *bool) {
		if !*flag {
			*flag = true
			eventsObserved++
		}
	}

	// fire will, once the first event for each watcher has
	// been observed, send a signal on the out channel.
	fire := func() {
		if eventsObserved != requiredEvents {
			return
		}
		select {
		case w.out <- struct{}{}:
		default:
		}
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-applicationChanges:
			logger.Debugf("got application change")
			if !ok {
				return errors.New("application watcher closed")
			}
			if err := w.applicationChanged(); err != nil {
				return errors.Trace(err)
			}
			observedEvent(&seenApplicationChange)
		}

		// Something changed.
		fire()
	}
}

// applicationChanged responds to changes in the application.
func (w *RemoteStateWatcher) applicationChanged() error {
	info, err := w.config.CharmGetter.Charm(w.application)
	if err != nil {
		return errors.Trace(err)
	}
	w.mu.Lock()
	w.current.CharmURL = info.URL
	w.current.ForceCharmUpgrade = info.ForceUpgrade
	w.current.CharmModifiedVersion = info.CharmModifiedVersion
	w.mu.Unlock()
	return nil
}
