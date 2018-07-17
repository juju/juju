// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseriesworker

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
	"gopkg.in/juju/worker.v1"
)

// Facade exposes functionality required by a Worker to handle parts of the
// upgrade series functionality.
type Facade interface {

	// WatchUpgradeSeriesNotifications
	WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error)
}

// Logger represents the logging methods called.
type Logger interface {
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
	Logf(level loggo.Level, message string, args ...interface{})
}

type Config struct {
	Facade Facade

	// Logger is the logger for this worker.
	Logger Logger

	// Tag is the current machine tag
	Tag names.Tag
}

// Validate validates the upgradeseries worker configuration.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Tag == nil {
		return errors.NotValidf("nil machine tag")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	return nil
}

//func NewWorker(st *upgradeseries.State, config Config) (worker.Worker, error) {
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	u := &upgradeSeriesWorker{
		upgradeSeriesFacade: config.Facade,
		config:              config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: u.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

// upgradeSeriesWorker is responsible for machine and unit agent requirements
// during upgrade-series:
// 		copying the agent binary directory and renaming;
// 		rewriting the machine and unit(s) systemd files if necessary;
// 		stopping the unit agents;
//		starting the unit agents;
//		moving the status of the upgrade-series steps along.
type upgradeSeriesWorker struct {
	catacomb catacomb.Catacomb
	config   Config

	upgradeSeriesFacade Facade
}

func (w *upgradeSeriesWorker) loop() error {
	uw, err := w.upgradeSeriesFacade.WatchUpgradeSeriesNotifications()
	if err != nil {
		return errors.Trace(err)
	}
	w.catacomb.Add(uw)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-uw.Changes():
			w.config.Logger.Errorf("machineUpgradeSeriesLocks changed")
		}
	}
	return nil
}

// Kill implements worker.Worker.Kill.
func (w *upgradeSeriesWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *upgradeSeriesWorker) Wait() error {
	return w.catacomb.Wait()
}

// Stop stops the upgradeseriesworker and returns any
// error it encountered when running.
func (w *upgradeSeriesWorker) Stop() error {
	w.Kill()
	return w.Wait()
}
