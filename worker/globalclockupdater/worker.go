// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/core/globalclock"
)

var logger = loggo.GetLogger("juju.worker.globalclockupdater")

// Config contains the configuration for the global clock updater worker.
type Config struct {
	// NewUpdater returns a new global clock updater.
	NewUpdater func() (globalclock.Updater, error)

	// LocalClock is the local wall clock. The times returned must
	// contain a monotonic component (Go 1.9+).
	LocalClock clock.Clock

	// UpdateInterval is the amount of time in between clock updates.
	UpdateInterval time.Duration

	// BackoffDelay is the amount of time to delay before attempting
	// another update when a concurrent write is detected.
	BackoffDelay time.Duration
}

// Validate validates the configuration.
func (config Config) Validate() error {
	if config.NewUpdater == nil {
		return errors.NotValidf("nil NewUpdater")
	}
	if config.LocalClock == nil {
		return errors.NotValidf("nil LocalClock")
	}
	if config.UpdateInterval <= 0 {
		return errors.NotValidf("non-positive UpdateInterval")
	}
	if config.BackoffDelay <= 0 {
		return errors.NotValidf("non-positive BackoffDelay")
	}
	return nil
}

// NewWorker returns a new global clock updater worker, using the given
// configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating config")
	}
	updater, err := config.NewUpdater()
	if err != nil {
		return nil, errors.Annotate(err, "getting new updater")
	}
	w := &updaterWorker{
		config:  config,
		updater: updater,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w, nil
}

type updaterWorker struct {
	tomb    tomb.Tomb
	config  Config
	updater globalclock.Updater
}

// Kill is part of the worker.Worker interface.
func (w *updaterWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *updaterWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *updaterWorker) loop() error {
	interval := w.config.UpdateInterval
	backoff := w.config.BackoffDelay

	last := w.config.LocalClock.Now()
	timer := w.config.LocalClock.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-timer.Chan():
			// Increment the global time by the amount of time
			// since the moment after we initially read or last
			// updated the clock.
			now := w.config.LocalClock.Now()
			amount := now.Sub(last)
			err := w.updater.Advance(amount)
			if err == globalclock.ErrConcurrentUpdate {
				logger.Tracef("concurrent update, backing off for %s", backoff)
				last = w.config.LocalClock.Now()
				timer.Reset(backoff)
				continue
			} else if err != nil {
				return errors.Annotate(err, "updating global clock")
			}
			logger.Tracef("incremented global time by %s", interval)
			last = w.config.LocalClock.Now()
			timer.Reset(interval)
		}
	}
}
