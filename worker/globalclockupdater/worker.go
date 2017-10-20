// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v1"
)

var logger = loggo.GetLogger("juju.worker.globalclockupdater")

type Worker struct {
	config Config
}

type Config struct {
	Updater        ClockUpdater
	LocalClock     clock.Clock
	UpdateInterval time.Duration
}

func (config Config) Validate() error {
	if config.Updater == nil {
		return errors.NotValidf("nil Updater")
	}
	if config.LocalClock == nil {
		return errors.NotValidf("nil LocalClock")
	}
	if config.UpdateInterval <= 0 {
		return errors.NotValidf("zero or negative UpdateInterval")
	}
	return nil
}

type ClockUpdater interface {
	AddTime(time.Duration) error
}

func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating config")
	}
	w := &updaterWorker{config: config}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w, nil
}

type updaterWorker struct {
	tomb   tomb.Tomb
	config Config
}

func (w *updaterWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *updaterWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *updaterWorker) loop() error {
	interval := w.config.UpdateInterval
	timer := w.config.LocalClock.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-timer.Chan():
			timer.Reset(interval)
			if err := w.config.Updater.AddTime(interval); err != nil {
				return errors.Annotate(err, "updating global clock")
			}
			logger.Tracef("incremented global time by %s", interval)
		}
	}
}
