// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	api "github.com/juju/juju/api/caasapplicationprovisioner"
	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
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
}

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	ProvisioningInfo(string) (api.ProvisioningInfo, error)
	WatchApplications() (watcher.StringsWatcher, error)
	SetPassword(string, string) error
	Life(string) (life.Value, error)
	CharmInfo(string) (*charmscommon.CharmInfo, error)
	ApplicationCharmURL(string) (*charm.URL, error)
	SetOperatorStatus(appName string, status status.Status, message string, data map[string]interface{}) error
	Units(appName string) ([]names.Tag, error)
	GarbageCollect(appName string, observedUnits []names.Tag, desiredReplicas int, activePodNames []string, force bool) error
	ApplicationOCIResources(appName string) (map[string]resources.DockerImageDetails, error)
	UpdateUnits(arg params.UpdateApplicationUnits) (*params.UpdateApplicationUnitsInfo, error)
	WatchApplication(appName string) (watcher.NotifyWatcher, error)
}

// CAASBroker exposes CAAS broker functionality to a worker.
type CAASBroker interface {
	Application(string, caas.DeploymentType) caas.Application
	AnnotateUnit(appName string, mode caas.DeploymentMode, podName string, unit names.UnitTag) error
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
	runner       *worker.Runner
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
	p := &provisioner{
		facade:       config.Facade,
		broker:       config.Broker,
		modelTag:     config.ModelTag,
		clock:        config.Clock,
		logger:       config.Logger,
		newAppWorker: config.NewAppWorker,
		runner: worker.NewRunner(worker.RunnerParams{
			Clock:        config.Clock,
			IsFatal:      func(error) bool { return false },
			RestartDelay: 3 * time.Second,
			Logger:       config.Logger.Child("runner"),
		}),
		unitFacade: config.UnitFacade,
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

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case apps, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("app watcher closed channel")
			}
			for _, app := range apps {
				existingWorker, err := p.runner.Worker(app, nil)
				if err == worker.ErrNotFound {
					// Ignore.
				} else if err == worker.ErrDead {
					existingWorker = nil
				} else if err != nil {
					return errors.Trace(err)
				}

				if existingWorker != nil {
					worker := existingWorker.(appNotifyWorker)
					worker.Notify()
					continue
				}

				config := AppWorkerConfig{
					Name:       app,
					Facade:     p.facade,
					Broker:     p.broker,
					ModelTag:   p.modelTag,
					Clock:      p.clock,
					Logger:     p.logger.Child("applicationworker"),
					UnitFacade: p.unitFacade,
				}
				startFunc := p.newAppWorker(config)
				err = p.runner.StartWorker(app, startFunc)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}
