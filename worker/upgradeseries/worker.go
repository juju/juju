// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"strings"
	"sync"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/service"
	"github.com/juju/os/series"
)

//go:generate mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/worker/upgradeseries Facade,Logger,AgentService,ServiceAccess,Upgrader

var hostSeries = series.HostSeries

// Logger represents the methods required to emit log messages.
type Logger interface {
	Debugf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
}

// Config is the configuration needed to construct an UpgradeSeries worker.
type Config struct {
	// FacadeFactory is used to acquire back-end state with
	// the input tag context.
	FacadeFactory func(names.Tag) Facade

	// Logger is the logger for this worker.
	Logger Logger

	// Tag is the current machine tag.
	Tag names.Tag

	// ServiceAccess provides access to the local init system.
	Service ServiceAccess

	// UpgraderFactory is a factory method that will return an upgrader capable
	// of handling service and agent binary manipulation for a
	// runtime-determined target OS series.
	UpgraderFactory func(string) (Upgrader, error)
}

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
	if config.FacadeFactory == nil {
		return errors.NotValidf("nil FacadeFactory")
	}
	if config.Service == nil {
		return errors.NotValidf("nil Service")
	}
	return nil
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

	facadeFactory   func(names.Tag) Facade
	catacomb        catacomb.Catacomb
	logger          Logger
	service         ServiceAccess
	upgraderFactory func(string) (Upgrader, error)

	// Some local state retained for reporting purposes.
	mu             sync.Mutex
	machineStatus  model.UpgradeSeriesStatus
	preparedUnits  []names.UnitTag
	completedUnits []names.UnitTag
}

// NewWorker creates, starts and returns a new upgrade-series worker based on
// the input configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeSeriesWorker{
		Facade:          config.FacadeFactory(config.Tag),
		facadeFactory:   config.FacadeFactory,
		logger:          config.Logger,
		service:         config.Service,
		upgraderFactory: config.UpgraderFactory,
		machineStatus:   model.UpgradeSeriesNotStarted,
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
		if errors.IsNotFound(err) {
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
	w.logger.Infof("machine series upgrade status changed to %q", w.machineStatus)

	switch w.machineStatus {
	case model.UpgradeSeriesPrepareStarted:
		err = w.handlePrepareStarted()
	case model.UpgradeSeriesCompleteStarted:
		err = w.handleCompleteStarted()
	case model.UpgradeSeriesCompleted:
		err = w.handleCompleted()
	}
	return errors.Trace(err)
}

// handlePrepareStarted handles workflow for the machine with an upgrade-series
// lock status of "UpgradeSeriesPrepareStarted"
func (w *upgradeSeriesWorker) handlePrepareStarted() error {
	var err error
	if w.preparedUnits, err = w.UnitsPrepared(); err != nil {
		return errors.Trace(err)
	}

	unitServices, allConfirmed, err := w.compareUnitAgentServices(w.preparedUnits)
	if err != nil {
		return errors.Trace(err)
	}
	if !allConfirmed {
		w.logger.Debugf(
			"still waiting for units to complete series upgrade preparation; known unit agent services: %s",
			unitNames(unitServices),
		)
		return nil
	}

	return errors.Trace(w.transitionPrepareComplete(unitServices))
}

// transitionPrepareComplete rewrites service unit files for unit agents running
// on this machine so that they are compatible with the init system of the
// series upgrade target.
func (w *upgradeSeriesWorker) transitionPrepareComplete(unitServices map[string]string) error {
	w.logger.Infof("preparing service units for series upgrade")
	toSeries, err := w.TargetSeries()
	if err != nil {
		return errors.Trace(err)
	}
	upgrader, err := w.upgraderFactory(toSeries)
	if err != nil {
		return errors.Trace(err)
	}
	if err := upgrader.PerformUpgrade(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(w.SetMachineStatus(model.UpgradeSeriesPrepareCompleted, "all necessary binaries and service files written"))
}

func (w *upgradeSeriesWorker) handleCompleteStarted() error {
	var err error
	if w.preparedUnits, err = w.UnitsPrepared(); err != nil {
		return errors.Trace(err)
	}

	// If the units are still all in the "PrepareComplete" state, then the
	// manual tasks have been run and an operator has executed the
	// upgrade-series completion command; start all the unit agents,
	// and progress the workflow.
	unitServices, allConfirmed, err := w.compareUnitAgentServices(w.preparedUnits)
	if err != nil {
		return errors.Trace(err)
	}
	servicesPresent := len(unitServices) > 0

	// allConfirmed returns true when there are no units, so we only need this
	// transition when there are services to start.
	// If there are none, just proceed to the completed stage.
	if allConfirmed && servicesPresent {
		return errors.Trace(w.transitionUnitsStarted(unitServices))
	}

	// If the units have all completed their workflow, then we are done.
	// Make the final update to the lock to say the machine is completed.
	if w.completedUnits, err = w.UnitsCompleted(); err != nil {
		return errors.Trace(err)
	}

	unitServices, allConfirmed, err = w.compareUnitAgentServices(w.completedUnits)
	if err != nil {
		return errors.Trace(err)
	}

	if allConfirmed {
		w.logger.Infof("series upgrade complete")
		return errors.Trace(w.SetMachineStatus(model.UpgradeSeriesCompleted, "series upgrade complete"))
	}

	return nil
}

// transitionUnitsStarted iterates over units managed by this machine. Starts
// the unit's agent service, and transitions all unit subordinate statuses.
func (w *upgradeSeriesWorker) transitionUnitsStarted(unitServices map[string]string) error {
	w.logger.Infof("starting units after series upgrade")

	for unit, serviceName := range unitServices {
		svc, err := w.service.DiscoverService(serviceName)
		if err != nil {
			return errors.Trace(err)
		}
		running, err := svc.Running()
		if err != nil {
			return errors.Trace(err)
		}
		if running {
			continue
		}
		if err := svc.Start(); err != nil {
			return errors.Annotatef(err, "starting %q unit agent after series upgrade", unit)
		}
	}

	return errors.Trace(w.StartUnitCompletion("starting all unit agents after series upgrade"))
}

// handleCompleted notifies the server that it has completed the upgrade
// workflow, passing back the current host OS series.
func (w *upgradeSeriesWorker) handleCompleted() error {
	s, err := hostSeries()
	if err != nil {
		return errors.Trace(err)
	}
	err = w.FinishUpgradeSeries(s)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// compareUnitsAgentServices filters the services running on the local machine
// to those that are for unit agents.
// The service names keyed by unit names are returned, along with a boolean
// indicating whether all the input unit tags are represented in the
// service map.
// NOTE: No unit tags and no agent services returns true, meaning that the
// workflow can progress.
func (w *upgradeSeriesWorker) compareUnitAgentServices(units []names.UnitTag) (map[string]string, bool, error) {
	services, err := w.service.ListServices()
	if err != nil {
		return nil, false, errors.Trace(err)
	}

	unitServices := service.FindUnitServiceNames(services)
	if len(unitServices) == 0 {
		w.logger.Debugf("no unit agent services found")
	}
	if len(units) != len(unitServices) {
		return unitServices, false, nil
	}

	for _, u := range units {
		if _, ok := unitServices[u.Id()]; !ok {
			return unitServices, false, nil
		}
	}
	return unitServices, true, nil
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

// unitNames returns a comma-delimited string of unit names based on the input
// map of unit agent services.
func unitNames(units map[string]string) string {
	unitIds := make([]string, len(units))
	i := 0
	for u := range units {
		unitIds[i] = u
		i++
	}
	return strings.Join(unitIds, ", ")
}
