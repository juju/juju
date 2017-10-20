// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"time"

	"github.com/juju/errors"
	mgo "gopkg.in/mgo.v2"
	tomb "gopkg.in/tomb.v1"
)

// Watcher provides a means of watching the global clock time.
//
// Watcher is goroutine-safe.
type Watcher struct {
	tomb   tomb.Tomb
	config WatcherConfig
	out    chan time.Time
}

// NewWatcher returns a new Watcher using the supplied config, or an error.
//
// Watchers must be stopped when they are no longer needed, and will not
// function past the lifetime of their configured *mgo.Session.
func NewWatcher(config WatcherConfig) (*Watcher, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	watcher := &Watcher{
		config: config,
		out:    make(chan time.Time),
	}
	go func() {
		defer watcher.tomb.Done()
		defer close(watcher.out)
		watcher.tomb.Kill(watcher.loop())
	}()
	return watcher, nil
}

// Changes returns the channel on which time.Time changes are sent.
//
// When the watcher starts, it will immediately query the current
// global time, and will make it available to send on the channel.
//
// The channel will be closed when the watcher stops.
func (w *Watcher) Changes() <-chan time.Time {
	return w.out
}

// Kill is part of the worker.Worker interface.
func (w *Watcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Watcher) Wait() error {
	return w.tomb.Wait()
}

func (w *Watcher) loop() error {
	timer := w.config.LocalClock.NewTimer(0) // query ~immediately
	defer timer.Stop()
	var lastTime time.Time
	var out chan<- time.Time
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-timer.Chan():
			timer.Reset(w.config.PollInterval)
			t, err := w.readClock()
			if err != nil {
				return errors.Trace(err)
			}
			if !t.After(lastTime) {
				continue
			}
			lastTime = t
			out = w.out
		case out <- lastTime:
			out = nil
		}
	}
}

func (w *Watcher) readClock() (time.Time, error) {
	t, err := readClock(w.config.Config)
	if errors.Cause(err) == mgo.ErrNotFound {
		// No time written yet. When it is written
		// for the first time, it'll be globalEpoch.
		t = globalEpoch
	} else if err != nil {
		return time.Time{}, errors.Trace(err)
	}
	return t, nil
}
