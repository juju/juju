// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/os/v2/series"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/internal/worker/upgradeseries Facade,UnitDiscovery,Upgrader

var hostSeries = series.HostSeries

// Logger represents the methods required to emit log messages.
type Logger interface {
	Debugf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
}

// UnitDiscovery represents how the worker determines which units need
// to check in.
type UnitDiscovery interface {
	Units() ([]names.UnitTag, error)
}

// Config is the configuration needed to construct an UpgradeSeries worker.
type Config struct {
	// Facade is used to access back-end state.
	Facade Facade

	// Logger is the logger for this worker.
	Logger Logger

	// UnitDiscovery determines how the worker knows which units should
	// be running on the machine.
	UnitDiscovery UnitDiscovery

	// UpgraderFactory is a factory method that will return an upgrader capable
	// of handling service and agent binary manipulation for a
	// runtime-determined current and target OS series.
	UpgraderFactory func(string, string) (Upgrader, error)
}

// Validate validates the upgrade-series worker configuration.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.UnitDiscovery == nil {
		return errors.NotValidf("nil UnitDiscovery")
	}
	if config.UpgraderFactory == nil {
		return errors.NotValidf("nil UpgraderFactory")
	}
	return nil
}

// upgradeSeriesWorker is responsible for machine and unit agent requirements
// during upgrade-series:
//
//	copying the agent binary directory and renaming;
//	rewriting the machine and unit(s) systemd files if necessary;
//	ensuring unit agents are started post-upgrade;
//	moving the status of the upgrade-series steps along.
type upgradeSeriesWorker struct {
	Facade

	catacomb        catacomb.Catacomb
	logger          Logger
	unitDiscovery   UnitDiscovery
	upgraderFactory func(string, string) (Upgrader, error)

	// Some local state retained for reporting purposes.
	mu             sync.Mutex
	machineStatus  model.UpgradeSeriesStatus
	units          names.Set
	preparedUnits  []names.UnitTag
	completedUnits []names.UnitTag

	// Ensure that leaders are pinned only once if possible,
	// on the first transition to UpgradeSeriesPrepareStarted.
	// However repeated pin calls are not of too much concern,
	// as the pin operations are idempotent.
	leadersPinned bool
}

// NewWorker creates, starts and returns a new upgrade-series worker based on
// the input configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeSeriesWorker{
		Facade:          config.Facade,
		logger:          config.Logger,
		unitDiscovery:   config.UnitDiscovery,
		upgraderFactory: config.UpgraderFactory,
		machineStatus:   model.UpgradeSeriesNotStarted,
		leadersPinned:   false,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *upgradeSeriesWorker) loop() error {
	uw, err := w.WatchUpgradeSeriesNotifications()
	if err != nil {
		return errors.Trace(err)
	}
	err = w.catacomb.Add(uw)
	if err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-uw.Changes():
			if err := w.handleUpgradeSeriesChange(); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// handleUpgradeSeriesChange retrieves the current upgrade-series status for
// this machine and based on the status, calls methods that will progress
// the workflow accordingly.
func (w *upgradeSeriesWorker) handleUpgradeSeriesChange() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var err error
	if w.machineStatus, err = w.MachineStatus(); err != nil {
		if errors.Is(err, errors.NotFound) {
			// No upgrade-series lock. This can happen when:
			// - The first watch call is made.
			// - The lock is removed after a completed upgrade.
			w.logger.Infof("no series upgrade lock present")
			w.machineStatus = model.UpgradeSeriesNotStarted
			w.preparedUnits = nil
			w.completedUnits = nil
			return nil
		}
		return errors.Trace(err)
	}
	w.logger.Infof("machine series upgrade status is %q", w.machineStatus)

	// Determine the set of units that are on the machine.
	if w.units == nil {
		units, err := w.unitDiscovery.Units()
		if err != nil {
			return errors.Annotate(err, "unit discovery")
		}
		w.units = names.NewSet(asGenericTags(units)...)
	}

	switch w.machineStatus {
	case model.UpgradeSeriesValidate:
		err = w.handleValidate()
	case model.UpgradeSeriesPrepareStarted:
		err = w.handlePrepareStarted()
	case model.UpgradeSeriesCompleteStarted:
		err = w.handleCompleteStarted()
	case model.UpgradeSeriesCompleted:
		err = w.handleCompleted()
	}

	if err != nil {
		if err := w.SetInstanceStatus(model.UpgradeSeriesError, err.Error()); err != nil {
			w.logger.Errorf("failed to set series upgrade error status: %s", err.Error())
		}
	}
	return errors.Trace(err)
}

// handleValidate handles the workflow for the machine with validating the
// given set of machine applications and charms.
func (w *upgradeSeriesWorker) handleValidate() error {
	if err := w.SetInstanceStatus(model.UpgradeSeriesValidate, "validating units"); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// handlePrepareStarted handles workflow for the machine with an upgrade-series
// lock status of "UpgradeSeriesPrepareStarted"
func (w *upgradeSeriesWorker) handlePrepareStarted() error {
	var err error
	if err = w.SetInstanceStatus(model.UpgradeSeriesPrepareStarted, "preparing units"); err != nil {
		return errors.Trace(err)
	}

	if !w.leadersPinned {
		if err = w.pinLeaders(); err != nil {
			return errors.Trace(err)
		}
	}

	if w.preparedUnits, err = w.UnitsPrepared(); err != nil {
		return errors.Trace(err)
	}

	// If not all the units have checked in, we are still preparing.
	prepared := names.NewSet(asGenericTags(w.preparedUnits)...)
	if remaining := w.units.Difference(prepared); remaining.Size() > 0 {
		// Not done yet.
		var names []string
		for _, tag := range remaining.SortedValues() {
			names = append(names, tag.Id())
		}
		w.logger.Debugf("waiting for units: %s", strings.Join(names, ","))
		return nil
	}

	return errors.Trace(w.transitionPrepareComplete())
}

// transitionPrepareComplete rewrites service unit files for unit agents running
// on this machine so that they are compatible with the init system of the
// series upgrade target.
func (w *upgradeSeriesWorker) transitionPrepareComplete() error {
	if err := w.SetInstanceStatus(model.UpgradeSeriesPrepareStarted, "completing preparation"); err != nil {
		return errors.Trace(err)
	}

	w.logger.Infof("preparing service units for series upgrade")
	currentSeries, err := w.CurrentSeries()
	if err != nil {
		return errors.Trace(err)
	}

	toSeries, err := w.TargetSeries()
	if err != nil {
		return errors.Trace(err)
	}

	upgrader, err := w.upgraderFactory(currentSeries, toSeries)
	if err != nil {
		return errors.Trace(err)
	}
	if err := upgrader.PerformUpgrade(); err != nil {
		return errors.Trace(err)
	}

	if err := w.SetMachineStatus(model.UpgradeSeriesPrepareCompleted, "binaries and service files written"); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(w.SetInstanceStatus(model.UpgradeSeriesPrepareCompleted, "waiting for completion command"))
}

func (w *upgradeSeriesWorker) handleCompleteStarted() error {
	if err := w.SetInstanceStatus(model.UpgradeSeriesCompleteStarted, "waiting for units"); err != nil {
		return errors.Trace(err)
	}

	var err error
	if w.preparedUnits, err = w.UnitsPrepared(); err != nil {
		return errors.Trace(err)
	}

	// If all the units are prepared, tell them to start.
	prepared := names.NewSet(asGenericTags(w.preparedUnits)...)
	if remaining := w.units.Difference(prepared); remaining.Size() == 0 && len(w.units) > 0 {
		return errors.Trace(w.StartUnitCompletion("start units after series upgrade"))
	}

	// If the units have all completed their workflow, then we are done.
	// Make the final update to the lock to say the machine is completed.
	if w.completedUnits, err = w.UnitsCompleted(); err != nil {
		return errors.Trace(err)
	}

	// If not all the units have checked in, we are still preparing.
	completed := names.NewSet(asGenericTags(w.completedUnits)...)
	if remaining := w.units.Difference(completed); remaining.Size() > 0 {
		// Not done yet.
		var names []string
		for _, tag := range remaining.SortedValues() {
			names = append(names, tag.Id())
		}
		w.logger.Debugf("waiting for units: %s", strings.Join(names, ","))
		return nil
	}

	w.logger.Infof("series upgrade complete")
	return errors.Trace(w.SetMachineStatus(model.UpgradeSeriesCompleted, "series upgrade complete"))
}

// handleCompleted notifies the server that it has completed the upgrade
// workflow, then unpins leadership for applications running on the machine.
func (w *upgradeSeriesWorker) handleCompleted() error {
	if err := w.SetInstanceStatus(model.UpgradeSeriesCompleted, "finalising upgrade"); err != nil {
		return errors.Trace(err)
	}

	s, err := hostSeries()
	if err != nil {
		return errors.Trace(err)
	}
	if err = w.FinishUpgradeSeries(s); err != nil {
		return errors.Trace(err)
	}
	if err = w.unpinLeaders(); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(w.SetInstanceStatus(model.UpgradeSeriesCompleted, "success"))
}

// pinLeaders pins leadership for applications
// represented by units running on this machine.
func (w *upgradeSeriesWorker) pinLeaders() (err error) {
	// if we encounter an error,
	// attempt to ensure that no application leaders remain pinned.
	defer func() {
		if err != nil {
			if unpinErr := w.unpinLeaders(); unpinErr != nil {
				err = errors.Wrap(err, unpinErr)
			}
		}
	}()

	results, err := w.PinMachineApplications()
	if err != nil {
		// If pin machine applications method return not implemented because it's
		// utilising the legacy leases store, then we should display the warning
		// in the log and return out. Unpinning leaders should be safe as that
		// should be considered a no-op
		if params.IsCodeNotImplemented(err) {
			w.logger.Infof("failed to pin machine applications, with legacy lease manager leadership pinning is not implemented")
			return nil
		}
		return errors.Trace(err)
	}

	var lastErr error
	for app, err := range results {
		if err == nil {
			w.logger.Infof("unpin leader for application %q", app)
			continue
		}
		w.logger.Errorf("failed to pin leader for application %q: %s", app, err.Error())
		lastErr = err
	}

	if lastErr == nil {
		w.leadersPinned = true
		return nil
	}
	return errors.Trace(lastErr)
}

// unpinLeaders unpins leadership for applications
// represented by units running on this machine.
func (w *upgradeSeriesWorker) unpinLeaders() error {
	results, err := w.UnpinMachineApplications()
	if err != nil {
		return errors.Trace(err)
	}

	var lastErr error
	for app, err := range results {
		if err == nil {
			w.logger.Infof("unpinned leader for application %q", app)
			continue
		}
		w.logger.Errorf("failed to unpin leader for application %q: %s", app, err.Error())
		lastErr = err
	}

	if lastErr == nil {
		w.leadersPinned = false
		return nil
	}
	return errors.Trace(lastErr)
}

// Report (worker.Reporter) generates a report for the Juju engine.
func (w *upgradeSeriesWorker) Report() map[string]interface{} {
	w.mu.Lock()
	defer w.mu.Unlock()

	report := map[string]interface{}{"machine status": w.machineStatus}

	if len(w.preparedUnits) > 0 {
		units := make([]string, len(w.preparedUnits))
		for i, u := range w.preparedUnits {
			units[i] = u.Id()
		}
		report["prepared units"] = units
	}

	if len(w.completedUnits) > 0 {
		units := make([]string, len(w.completedUnits))
		for i, u := range w.completedUnits {
			units[i] = u.Id()
		}
		report["completed units"] = units
	}

	return report
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

func asGenericTags(units []names.UnitTag) []names.Tag {
	result := make([]names.Tag, len(units))
	for i, tag := range units {
		result[i] = tag
	}
	return result
}
