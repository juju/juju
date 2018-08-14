// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/service"
)

// TODO (manadart 2018-07-30) Relocate this somewhere more central?
//go:generate mockgen -package upgradeseries_test -destination worker_mock_test.go gopkg.in/juju/worker.v1 Worker

//go:generate mockgen -package upgradeseries_test -destination package_mock_test.go github.com/juju/juju/worker/upgradeseries Facade,Logger,AgentService,ServiceAccess

// Logger represents the methods required to emit log messages.
type Logger interface {
	Logf(level loggo.Level, message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
}

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

	facadeFactory func(names.Tag) Facade
	catacomb      catacomb.Catacomb
	logger        Logger
	service       ServiceAccess
}

// NewWorker creates, starts and returns a new upgrade-series worker based on
// the input configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeSeriesWorker{
		Facade:        config.FacadeFactory(config.Tag),
		facadeFactory: config.FacadeFactory,
		logger:        config.Logger,
		service:       config.Service,
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
	w.catacomb.Add(uw)

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

// handleUpgradeSeriesChange retrieves the upgrade-series status for this
// machine and all of its units.
// Based on the status, actions are taken.
// TODO (manadart 2018-08-06): This needs upstream work to streamline.
// We should effectively be getting the whole upgrade-series lock - machine
// and unit data, to properly assess the current state and to transition as
// required.
func (w *upgradeSeriesWorker) handleUpgradeSeriesChange() error {
	machineStatus, err := w.MachineStatus()
	if err != nil {
		if errors.IsNotFound(err) {
			// No upgrade-series lock.
			// This can only happen on the first watch call.
			return nil
		}
		return errors.Trace(err)
	}
	w.logger.Logf(loggo.DEBUG, "series upgrade lock changed")

	preparation, err := w.UpgradeSeriesStatus(model.PrepareStatus)
	if err != nil {
		return errors.Trace(err)
	}
	prepared := unitsCompleted(preparation)

	// Units are completed, but not yet stopped - shut them down.
	if machineStatus == model.UpgradeSeriesPrepareStarted && prepared {
		return errors.Trace(w.transitionPrepareMachine(len(preparation)))
	}

	// Units are stopped, but not updated for the new init system.
	// Perform the required unit file manipulation.
	if machineStatus == model.UpgradeSeriesPrepareMachine && prepared {
		return errors.Trace(w.transitionPrepareComplete(len(preparation)))
	}

	// TODO (manadart 2018-08-06): We don't need this once we have a single
	// list of statuses.
	completion, err := w.UpgradeSeriesStatus(model.CompleteStatus)
	if err != nil {
		return errors.Trace(err)
	}

	started := unitsStarted(completion)
	completed := unitsCompleted(completion)

	// User has done the required manual work and run upgrade-series complete.
	// Restart the unit agents.
	if machineStatus == model.UpgradeSeriesCompleteStarted && prepared && !started && !completed {
		return errors.Trace(w.transitionUnitsStarted(len(preparation)))
	}

	// All the units have run their series-upgrade complete hooks and indicated
	// that they are completed. Transition the machine to completed too.
	if machineStatus == model.UpgradeSeriesCompleteStarted && completed {
		// TODO (manadart 2018-08-09): Do we remove the lock at some point?
		w.logger.Logf(loggo.INFO, "series upgrade complete")
		return errors.Trace(w.SetMachineStatus(model.UpgradeSeriesComplete))
	}

	return nil
}

// transitionPrepareMachine stops all unit agents on this machine and updates
// the upgrade-series status lock to indicate that upgrade work can proceed.
// The number of known upgrade-series unit statuses is passed in order to do a
// validation against the detected unit agents on the machine.
// TODO (manadart 2018-08-09): Rename when a better name is contrived for
// UpgradeSeriesPrepareMachine
func (w *upgradeSeriesWorker) transitionPrepareMachine(statusCount int) error {
	w.logger.Logf(loggo.INFO, "stopping units for series upgrade")

	unitServices, err := w.unitAgentServices(statusCount)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO (manadart 2018-08-06) This needs to be reworked during allotted
	// refactor period. At present there is no way to determine the status of
	// a *specific* unit, so we need to ensure they are *all* stopped and only
	// perform the status updates afterwards. We can't afford to be in a
	// partial state.
	for unit, serviceName := range unitServices {
		svc, err := w.service.DiscoverService(serviceName)

		running, err := svc.Running()
		if err != nil {
			return errors.Trace(err)
		}
		if !running {
			continue
		}

		if err := svc.Stop(); err != nil {
			return errors.Annotatef(err, "stopping %q unit agent for series upgrade", unit)
		}
	}

	return errors.Trace(w.SetMachineStatus(model.UpgradeSeriesPrepareMachine))
}

// transitionPrepareMachine rewrites service unit files for unit agents running
// on this machine so that they are compatible with the init system of the
// series upgrade target
func (w *upgradeSeriesWorker) transitionPrepareComplete(statusCount int) error {
	w.logger.Logf(loggo.INFO, "preparing service units for series upgrade")

	_, err := w.unitAgentServices(statusCount)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO (manadart 2018-08-09): Unit file wrangling to come.
	// For now we just update the machine status to progress the workflow.

	return errors.Trace(w.SetMachineStatus(model.UpgradeSeriesPrepareComplete))
}

// transitionUnitsStarted iterates over units managed by this machine.
// starts the unit's agent service.
func (w *upgradeSeriesWorker) transitionUnitsStarted(statusCount int) error {
	w.logger.Logf(loggo.INFO, "starting units after series upgrade")

	unitServices, err := w.unitAgentServices(statusCount)
	if err != nil {
		return errors.Trace(err)
	}

	for unit, serviceName := range unitServices {
		svc, err := w.service.DiscoverService(serviceName)
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

	return nil
}

// unitAgentServices filters the services running on the local machine to those
// that are for unit agents. If the number of services returned differs from
// input count, then a error is returned.
func (w *upgradeSeriesWorker) unitAgentServices(statusCount int) (map[string]string, error) {
	services, err := w.service.ListServices()
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitServices := service.FindUnitServiceNames(services)
	if len(unitServices) != statusCount {
		return nil, errors.Errorf("mismatched counts; upgrade-series statuses: %d, detected services: %d",
			statusCount, len(unitServices))
	}
	return unitServices, nil
}

func (w *upgradeSeriesWorker) setUnitStatus(
	unitName string, statusType model.UpgradeSeriesStatusType, status model.UnitSeriesUpgradeStatus,
) error {
	return errors.Trace(w.facadeFactory(names.NewUnitTag(unitName)).SetUpgradeSeriesStatus(string(status), statusType))
}

func unitsCompleted(statuses []string) bool {
	return unitsAllWithStatus(statuses, model.UnitCompleted)
}

func unitsStarted(statuses []string) bool {
	return unitsAllWithStatus(statuses, model.UnitStarted)
}

func unitsAllWithStatus(statuses []string, target model.UnitSeriesUpgradeStatus) bool {
	t := string(target)
	for _, s := range statuses {
		if s != t {
			return false
		}
	}
	return true
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
