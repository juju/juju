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
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	charmscommon "github.com/juju/juju/api/common/charms"
	api "github.com/juju/juju/api/controller/caasapplicationprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationservice "github.com/juju/juju/domain/application/service"
	internalworker "github.com/juju/juju/internal/worker"
)

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	ProvisioningInfo(context.Context, string) (api.ProvisioningInfo, error)
	CharmInfo(context.Context, string) (*charmscommon.CharmInfo, error)
	ApplicationCharmInfo(context.Context, string) (*charmscommon.CharmInfo, error)
	ApplicationOCIResources(ctx context.Context, appName string) (map[string]resource.DockerImageDetails, error)
	RemoveUnit(ctx context.Context, unitName string) error
	WatchProvisioningInfo(context.Context, string) (watcher.NotifyWatcher, error)
	DestroyUnits(ctx context.Context, unitNames []string) error
}

// ApplicationService is used to interact with the application service.
type ApplicationService interface {
	// GetApplicationTrustSetting returns the application trust setting.
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
	GetApplicationTrustSetting(ctx context.Context, appName string) (bool, error)

	// WatchApplicationSettings watches for changes to the specified application's
	// settings.
	// This functions returns the following errors:
	// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
	WatchApplicationSettings(ctx context.Context, name string) (watcher.NotifyWatcher, error)

	// WatchApplicationUnitLife returns a watcher that observes changes to the life of any units if an application.
	WatchApplicationUnitLife(ctx context.Context, appName string) (watcher.StringsWatcher, error)

	// WatchApplicationScale returns a watcher that observes changes to an application's scale.
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
	WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error)

	// GetApplicationScale returns the desired scale of an application,
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
	GetApplicationScale(ctx context.Context, appName string) (int, error)

	SetApplicationScalingState(ctx context.Context, name string, scaleTarget int, scaling bool) error
	GetApplicationScalingState(ctx context.Context, name string) (applicationservice.ScalingState, error)
	GetApplicationLife(ctx context.Context, id application.ID) (life.Value, error)
	GetUnitLife(context.Context, unit.Name) (life.Value, error)
	GetAllUnitLifeForApplication(context.Context, application.ID) (map[unit.Name]life.Value, error)

	// GetApplicationName returns the application name for the given application ID.
	GetApplicationName(ctx context.Context, id application.ID) (string, error)

	// WatchApplications returns a watcher that observes changes to applications.
	WatchApplications(ctx context.Context) (watcher.StringsWatcher, error)

	// UpsertCloudService updates the cloud service for the specified application.
	UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.ProviderAddresses) error

	// IsControllerApplication returns true when the application is the controller.
	IsControllerApplication(ctx context.Context, id application.ID) (bool, error)

	// UpdateCAASUnit updates the specified CAAS unit
	UpdateCAASUnit(context.Context, unit.Name, applicationservice.UpdateCAASUnitParams) error

	// GetAllUnitCloudContainerIDsForApplication returns a map of the unit names
	// and their cloud container provider IDs for the given application.
	GetAllUnitCloudContainerIDsForApplication(ctx context.Context, id application.ID) (map[unit.Name]string, error)
}

// CAASBroker exposes CAAS broker functionality to a worker.
type CAASBroker interface {
	Application(string, caas.DeploymentType) caas.Application
	AnnotateUnit(ctx context.Context, appName string, podName string, unit names.UnitTag) error
	Units(ctx context.Context, appName string) ([]caas.Unit, error)
}

// Runner exposes functionalities of a worker.Runner.
type Runner interface {
	Worker(id string, abort <-chan struct{}) (worker.Worker, error)
	StartWorker(ctx context.Context, id string, startFunc func(context.Context) (worker.Worker, error)) error
	StopAndRemoveWorker(id string, abort <-chan struct{}) error
	Report() map[string]any
	worker.Worker
}

type StatusService interface {
	// GetUnitAgentStatusesForApplication returns the agent statuses of all
	// units in the specified application, indexed by unit name, returning an error
	// satisfying [statuserrors.ApplicationNotFound] if the application doesn't
	// exist.
	GetUnitAgentStatusesForApplication(ctx context.Context, appID application.ID) (map[unit.Name]status.StatusInfo, error)

	// SetApplicationStatus saves the given application status, overwriting any
	// current status data. If returns an error satisfying
	// [statuserrors.ApplicationNotFound] if the application doesn't exist.
	SetApplicationStatus(ctx context.Context, name string, info status.StatusInfo) error
}

type AgentPasswordService interface {
	// SetApplicationPassword sets the password for the given application. If the
	// app does not exist, an error satisfying [applicationerrors.ApplicationNotFound]
	// is returned.
	SetApplicationPassword(ctx context.Context, appID application.ID, password string) error
}

// Config defines the operation of a Worker.
type Config struct {
	ApplicationService   ApplicationService
	StatusService        StatusService
	AgentPasswordService AgentPasswordService
	Facade               CAASProvisionerFacade
	Broker               CAASBroker
	ModelTag             names.ModelTag
	Clock                clock.Clock
	Logger               logger.Logger
	NewAppWorker         NewAppWorkerFunc
}

type provisioner struct {
	catacomb             catacomb.Catacomb
	runner               Runner
	applicationService   ApplicationService
	statusService        StatusService
	agentPasswordService AgentPasswordService
	facade               CAASProvisionerFacade
	broker               CAASBroker
	clock                clock.Clock
	logger               logger.Logger
	newAppWorker         NewAppWorkerFunc
	modelTag             names.ModelTag
}

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(config Config) (worker.Worker, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:         "provisioner",
		Clock:        config.Clock,
		IsFatal:      func(error) bool { return false },
		RestartDelay: 3 * time.Second,
		Logger:       internalworker.WrapLogger(config.Logger.Child("runner")),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newProvisionerWorker(config, runner)
}

func newProvisionerWorker(
	config Config, runner Runner,
) (worker.Worker, error) {
	p := &provisioner{
		applicationService:   config.ApplicationService,
		statusService:        config.StatusService,
		agentPasswordService: config.AgentPasswordService,
		facade:               config.Facade,
		broker:               config.Broker,
		modelTag:             config.ModelTag,
		clock:                config.Clock,
		logger:               config.Logger,
		newAppWorker:         config.NewAppWorker,
		runner:               runner,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-application-provisioner",
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

	appWatcher, err := p.applicationService.WatchApplications(ctx)
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
			for _, id := range apps {
				appID, err := application.ParseID(id)
				if err != nil {
					return errors.Trace(err)
				}

				existingWorker, err := p.runner.Worker(id, p.catacomb.Dying())
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
					AppID:                appID,
					ApplicationService:   p.applicationService,
					StatusService:        p.statusService,
					AgentPasswordService: p.agentPasswordService,
					Facade:               p.facade,
					Broker:               p.broker,
					ModelTag:             p.modelTag,
					Clock:                p.clock,
					Logger:               p.logger.Child(id),
				}
				startFunc := p.newAppWorker(config)
				p.logger.Debugf(ctx, "starting app worker %q", appID)
				err = p.runner.StartWorker(ctx, id, startFunc)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

// Report calls onto the runner give back information about each application
// worker for an engine report.
func (p *provisioner) Report() map[string]any {
	return p.runner.Report()
}

func (p *provisioner) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(p.catacomb.Context(context.Background()))
}
