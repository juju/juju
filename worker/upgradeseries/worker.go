// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/worker/catacomb"
	"gopkg.in/juju/worker.v1"
)

// Logger represents the methods required to emit log messages.
//go:generate mockgen -package upgradeseries_test -destination logger_mock_test.go github.com/juju/juju/worker/upgradeseries Logger
type Logger interface {
	Logf(level loggo.Level, message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
}

type Config struct {
	Facade Facade

	// Logger is the logger for this worker.
	Logger Logger

	// Tag is the current machine tag.
	Tag names.Tag
}

// TODO (manadart 2018-07-30) Relocate this somewhere more central?
//go:generate mockgen -package upgradeseries_test -destination worker_mock_test.go gopkg.in/juju/worker.v1 Worker

// Validate validates the upgrade-series worker configuration.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Tag == nil {
		return errors.NotValidf("nil machine tag")
	}
	k := config.Tag.Kind()
	if k != names.MachineTagKind {
		return errors.NotValidf("%q tag kind", k)
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	return nil
}

// NewWorker creates, starts and returns a new upgrade-series worker based on
// the input configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &upgradeSeriesWorker{
		Facade: config.Facade,
		logger: config.Logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// upgradeSeriesWorker is responsible for machine and unit agent requirements
// during upgrade-series:
// 		copying the agent binary directory and renaming;
// 		rewriting the machine and unit(s) systemd files if necessary;
// 		stopping the unit agents;
//		starting the unit agents;
//		moving the status of the upgrade-series steps along.
type upgradeSeriesWorker struct {
	Facade

	catacomb catacomb.Catacomb
	logger   Logger
}

func (w *upgradeSeriesWorker) loop() error {
	uw, err := w.WatchUpgradeSeriesNotifications()
	if err != nil {
		return errors.Trace(err)
	}
	w.catacomb.Add(uw)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-uw.Changes():
			w.logger.Errorf("machineUpgradeSeriesLocks changed")
		}
	}
}

// Kill implements worker.Worker.Kill.
func (w *upgradeSeriesWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *upgradeSeriesWorker) Wait() error {
	return w.catacomb.Wait()
}

// Stop stops the upgrade-series worker and returns any
// error it encountered when running.
func (w *upgradeSeriesWorker) Stop() error {
	w.Kill()
	return w.Wait()
}
