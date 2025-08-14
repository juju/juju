// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This worker is responsible for watching the life cycle of CAAS sidecar
// applications and setting them up (or removing them). It creates a new
// worker goroutine for every application being monitored, so most of the
// actual operations happen in the child worker.
//
// Note that the separate caasoperatorprovisioner worker handles legacy CAAS
// pod-spec applications.

package caasapplicationprovisioner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	charmscommon "github.com/juju/juju/api/common/charms"
	api "github.com/juju/juju/api/controller/caasapplicationprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

type CAASUnitProvisionerFacade interface {
	ApplicationScale(string) (int, error)
	WatchApplicationScale(string) (watcher.NotifyWatcher, error)
	ApplicationTrust(string) (bool, error)
	WatchApplicationTrustHash(string) (watcher.StringsWatcher, error)
	UpdateApplicationService(arg params.UpdateApplicationServiceArg) error
}

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	ProvisioningInfo(string) (api.ProvisioningInfo, error)
	WatchApplications() (watcher.StringsWatcher, error)
	SetPassword(string, string) error
	Life(string) (life.Value, error)
	CharmInfo(string) (*charmscommon.CharmInfo, error)
	ApplicationCharmInfo(string) (*charmscommon.CharmInfo, error)
	SetOperatorStatus(appName string, status status.Status, message string, data map[string]interface{}) error
	Units(appName string) ([]params.CAASUnit, error)
	ApplicationOCIResources(appName string) (map[string]resources.DockerImageDetails, error)
	UpdateUnits(arg params.UpdateApplicationUnits) (*params.UpdateApplicationUnitsInfo, error)
	WatchApplication(appName string) (watcher.NotifyWatcher, error)
	ClearApplicationResources(appName string) error
	WatchUnits(application string) (watcher.StringsWatcher, error)
	RemoveUnit(unitName string) error
	WatchProvisioningInfo(string) (watcher.NotifyWatcher, error)
	DestroyUnits(unitNames []string) error
	ProvisioningState(string) (*params.CAASApplicationProvisioningState, error)
	SetProvisioningState(string, params.CAASApplicationProvisioningState) error
	ProvisionerConfig() (params.CAASApplicationProvisionerConfig, error)
}

// CAASBroker exposes CAAS broker functionality to a worker.
type CAASBroker interface {
	Application(string, caas.DeploymentType) caas.Application
	AnnotateUnit(appName string, mode caas.DeploymentMode, podName string, unit names.UnitTag) error
	DeleteOperator(appName string) error
	DeleteService(appName string) error
	OperatorExists(appName string) (caas.DeploymentState, error)
	Units(appName string, mode caas.DeploymentMode) ([]caas.Unit, error)
	DeleteCustomResourceDefinitionsForApps(appName string) error
}

// Runner exposes functionalities of a worker.Runner.
type Runner interface {
	Worker(id string, abort <-chan struct{}) (worker.Worker, error)
	StartWorker(id string, startFunc func() (worker.Worker, error)) error
	StopAndRemoveWorker(id string, abort <-chan struct{}) error
	worker.Worker
}

// Config defines the operation of a Worker.
type Config struct {
	Facade       CAASProvisionerFacade
	Broker       CAASBroker
	ModelTag     names.ModelTag
	Clock        clock.Clock
	Logger       Logger
	NewAppWorker NewAppWorkerFunc
	UnitFacade   CAASUnitProvisionerFacade
}

type provisioner struct {
	catacomb     catacomb.Catacomb
	runner       Runner
	facade       CAASProvisionerFacade
	broker       CAASBroker
	clock        clock.Clock
	logger       Logger
	newAppWorker NewAppWorkerFunc
	modelTag     names.ModelTag
	unitFacade   CAASUnitProvisionerFacade
}

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(config Config) (worker.Worker, error) {
	return newProvisionerWorker(config,
		worker.NewRunner(worker.RunnerParams{
			Clock:        config.Clock,
			IsFatal:      func(error) bool { return false },
			RestartDelay: 3 * time.Second,
			Logger:       config.Logger.Child("runner"),
		}),
	)
}

func newProvisionerWorker(
	config Config, runner Runner,
) (worker.Worker, error) {
	p := &provisioner{
		facade:       config.Facade,
		broker:       config.Broker,
		modelTag:     config.ModelTag,
		clock:        config.Clock,
		logger:       config.Logger,
		newAppWorker: config.NewAppWorker,
		runner:       runner,
		unitFacade:   config.UnitFacade,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
		Init: []worker.Worker{p.runner},
	})
	return p, err
}

// Kill is part of the worker.Worker interface.
func (p *provisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *provisioner) Wait() error {
	return p.catacomb.Wait()
}

func (p *provisioner) loop() error {
	appWatcher, err := p.facade.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	config, err := p.facade.ProvisionerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	unmanagedApps := set.NewStrings()
	for _, v := range config.UnmanagedApplications.Entities {
		app, err := names.ParseApplicationTag(v.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		unmanagedApps.Add(app.Name)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case apps, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("app watcher closed channel")
			}
			for _, appName := range apps {
				_, err := p.facade.Life(appName)
				if err != nil && !errors.IsNotFound(err) {
					return errors.Trace(err)
				}
				if errors.IsNotFound(err) {
					p.logger.Debugf("application %q not found, ignoring", appName)
					continue
				}

				existingWorker, err := p.runner.Worker(appName, p.catacomb.Dying())
				if errors.IsNotFound(err) {
					// Ignore.
				} else if err == worker.ErrDead {
					// Runner is dying so we need to stop processing.
					break
				} else if err != nil {
					return errors.Trace(err)
				}

				if existingWorker != nil {
					w := existingWorker.(appNotifyWorker)
					w.Notify()
					continue
				}

				config := AppWorkerConfig{
					Name:       appName,
					Facade:     p.facade,
					Broker:     p.broker,
					ModelTag:   p.modelTag,
					Clock:      p.clock,
					Logger:     p.logger.Child(appName),
					UnitFacade: p.unitFacade,
					StatusOnly: unmanagedApps.Contains(appName),
				}
				startFunc := p.newAppWorker(config)
				p.logger.Debugf("starting app worker %q", appName)
				err = p.runner.StartWorker(appName, startFunc)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}
