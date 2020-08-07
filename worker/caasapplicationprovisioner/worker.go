// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/agent"
	api "github.com/juju/juju/api/caasapplicationprovisioner"
	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

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
}

// Config defines the operation of a Worker.
type Config struct {
	Facade      CAASProvisionerFacade
	Broker      caas.Broker
	ModelTag    names.ModelTag
	AgentConfig agent.Config
	Clock       clock.Clock
	Logger      Logger
}

type provisioner struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner
	facade   CAASProvisionerFacade
	broker   caas.Broker
	clock    clock.Clock
	logger   Logger

	modelTag    names.ModelTag
	agentConfig agent.Config
}

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(config Config) (worker.Worker, error) {
	p := &provisioner{
		facade:      config.Facade,
		broker:      config.Broker,
		modelTag:    config.ModelTag,
		agentConfig: config.AgentConfig,
		clock:       config.Clock,
		logger:      config.Logger,
		runner: worker.NewRunner(worker.RunnerParams{
			Clock:        config.Clock,
			IsFatal:      func(error) bool { return false },
			RestartDelay: 3 * time.Second,
			Logger:       config.Logger,
		}),
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
					worker := existingWorker.(*appWorker)
					worker.Notify()
					continue
				}

				config := appWorkerConfig{
					Name:     app,
					Facade:   p.facade,
					Broker:   p.broker,
					ModelTag: p.modelTag,
					Clock:    p.clock,
					Logger:   p.logger,
				}
				startFunc := newAppWorker(config)
				err = p.runner.StartWorker(app, startFunc)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}
