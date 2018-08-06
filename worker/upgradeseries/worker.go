// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/worker/catacomb"
)

// TODO (manadart 2018-07-30) Relocate this somewhere more central?
//go:generate mockgen -package upgradeseries_test -destination worker_mock_test.go gopkg.in/juju/worker.v1 Worker

//go:generate mockgen -package upgradeseries_test -destination package_mock_test.go github.com/juju/juju/worker/upgradeseries Facade,Logger,AgentService

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
	return nil
}

// AgentService is a service managed by the local init system, for running a
// unit agent.
type AgentService interface {
	Start() error
	Stop() error
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

	// listServices returns a slice of service names known by the local init
	// system. It is a shim for service.ListServices.
	listServices func() ([]string, error)

	// discoverService returns a service implementation base on the input
	// service name and config. It is a shim for service.DiscoverService.
	discoverService func(string, common.Conf) (AgentService, error)
}

// NewWorker creates, starts and returns a new upgrade-series worker based on
// the input configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeSeriesWorker{
		Facade:       config.FacadeFactory(config.Tag),
		logger:       config.Logger,
		listServices: service.ListServices,
		discoverService: func(name string, cfg common.Conf) (AgentService, error) {
			return service.DiscoverService(name, cfg)
		},
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
func (w *upgradeSeriesWorker) handleUpgradeSeriesChange() error {
	statuses, err := w.UpgradeSeriesStatus(model.PrepareStatus)
	if err != nil {
		return errors.Trace(err)
	}

	if unitAgentsReadyToStop(statuses) {
		return errors.Trace(w.transitionPrepareComplete(len(statuses)))
	}

	if unitAgentsReadyToStart(statuses) {
		return errors.Trace(w.transitionUnitsStarted(len(statuses)))

	}

	return nil
}

// transitionPrepareComplete stops all unit agents on this machine and updates
// the upgrade-series status lock to indicate that upgrade work can proceed.
// The number of known upgrade-series unit statuses is passed in order to do a
// validation against the detected unit agents on the machine.
func (w *upgradeSeriesWorker) transitionPrepareComplete(statusCount int) error {
	unitServices, err := w.unitAgentServices(statusCount)
	if err != nil {
		return errors.Trace(err)
	}

	w.logger.Logf(loggo.INFO, "stopping units for series upgrade")
	for unit, serviceName := range unitServices {
		svc, err := w.discoverService(serviceName, common.Conf{})
		if err != nil {
			return errors.Trace(err)
		}

		if err := svc.Stop(); err != nil {
			return errors.Annotatef(err, "stopping %q unit agent for series upgrade", unit)
		}

		if err := w.setUnitStatus(unit, model.CompleteStatus, model.UnitNotStarted); err != nil {
			return errors.Trace(err)
		}
	}

	// TODO: Perhaps accrue errors above and stop the agents that we can,
	// then report failures here.
	return nil
}

// transitionUnitsStarted updates the upgrade-series status for this machine
// and its units to indicate readiness for the "complete" command, then starts
// all of the units.
func (w *upgradeSeriesWorker) transitionUnitsStarted(statusCount int) error {
	_, err := w.unitAgentServices(statusCount)
	if err != nil {
		return errors.Trace(err)
	}

	return errors.NotImplementedf("transitionUnitsStarted")
}

// unitAgentServices filters the services running on the local machine to those
// that are for unit agents. If the number of services returned differs from
// input count, then a error is returned.
func (w *upgradeSeriesWorker) unitAgentServices(statusCount int) (map[string]string, error) {
	services, err := w.listServices()
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitServices := service.FindUnitServiceNames(services)
	if len(unitServices) != statusCount {
		return nil, fmt.Errorf("missmatched counts; upgrade-series statuses: %d, detected services: %d",
			statusCount, len(unitServices))
	}
	return unitServices, nil
}

func (w *upgradeSeriesWorker) setUnitStatus(
	unitName string, statusType model.UpgradeSeriesStatusType, status model.UnitSeriesUpgradeStatus,
) error {
	return errors.Trace(w.facadeFactory(names.NewUnitTag(unitName)).SetUpgradeSeriesStatus(string(status), statusType))
}

func unitAgentsReadyToStop(statuses []string) bool {
	return unitsAllWithStatus(statuses, model.UnitCompleted)
}

func unitAgentsReadyToStart(statuses []string) bool {
	return unitsAllWithStatus(statuses, model.UnitNotStarted)
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
