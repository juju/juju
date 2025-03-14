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
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	charmscommon "github.com/juju/juju/api/common/charms"
	api "github.com/juju/juju/api/controller/caasapplicationprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/k8s"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

type CAASUnitProvisionerFacade interface {
	ApplicationScale(context.Context, string) (int, error)
	WatchApplicationScale(context.Context, string) (watcher.NotifyWatcher, error)
	ApplicationTrust(context.Context, string) (bool, error)
	WatchApplicationTrustHash(context.Context, string) (watcher.StringsWatcher, error)
	UpdateApplicationService(ctx context.Context, arg params.UpdateApplicationServiceArg) error
}

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	ProvisioningInfo(context.Context, string) (api.ProvisioningInfo, error)
	WatchApplications(context.Context) (watcher.StringsWatcher, error)
	SetPassword(context.Context, string, string) error
	Life(context.Context, string) (life.Value, error)
	CharmInfo(context.Context, string) (*charmscommon.CharmInfo, error)
	ApplicationCharmInfo(context.Context, string) (*charmscommon.CharmInfo, error)
	SetOperatorStatus(ctx context.Context, appName string, status status.Status, message string, data map[string]interface{}) error
	Units(ctx context.Context, appName string) ([]params.CAASUnit, error)
	ApplicationOCIResources(ctx context.Context, appName string) (map[string]resource.DockerImageDetails, error)
	UpdateUnits(ctx context.Context, arg params.UpdateApplicationUnits) (*params.UpdateApplicationUnitsInfo, error)
	WatchApplication(ctx context.Context, appName string) (watcher.NotifyWatcher, error)
	ClearApplicationResources(ctx context.Context, appName string) error
	WatchUnits(ctx context.Context, application string) (watcher.StringsWatcher, error)
	RemoveUnit(ctx context.Context, unitName string) error
	WatchProvisioningInfo(context.Context, string) (watcher.NotifyWatcher, error)
	DestroyUnits(ctx context.Context, unitNames []string) error
	ProvisioningState(context.Context, string) (*params.CAASApplicationProvisioningState, error)
	SetProvisioningState(context.Context, string, params.CAASApplicationProvisioningState) error
	ProvisionerConfig(context.Context) (params.CAASApplicationProvisionerConfig, error)
}

// CAASBroker exposes CAAS broker functionality to a worker.
type CAASBroker interface {
	Application(string, k8s.WorkloadType) caas.Application
	AnnotateUnit(ctx context.Context, appName string, podName string, unit names.UnitTag) error
	Units(ctx context.Context, appName string) ([]caas.Unit, error)
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
	Logger       logger.Logger
	NewAppWorker NewAppWorkerFunc
	UnitFacade   CAASUnitProvisionerFacade
}

type provisioner struct {
	catacomb     catacomb.Catacomb
	runner       Runner
	facade       CAASProvisionerFacade
	broker       CAASBroker
	clock        clock.Clock
	logger       logger.Logger
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
			Logger:       internalworker.WrapLogger(config.Logger.Child("runner")),
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
	ctx, cancel := p.scopedContext()
	defer cancel()

	appWatcher, err := p.facade.WatchApplications(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	config, err := p.facade.ProvisionerConfig(ctx)
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
				_, err := p.facade.Life(ctx, appName)
				if err != nil && !errors.Is(err, errors.NotFound) {
					return errors.Trace(err)
				}
				if errors.Is(err, errors.NotFound) {
					p.logger.Debugf(ctx, "application %q not found, ignoring", appName)
					continue
				}

				existingWorker, err := p.runner.Worker(appName, p.catacomb.Dying())
				if errors.Is(err, errors.NotFound) {
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
				p.logger.Debugf(ctx, "starting app worker %q", appName)
				err = p.runner.StartWorker(appName, startFunc)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (p *provisioner) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(p.catacomb.Context(context.Background()))
}
