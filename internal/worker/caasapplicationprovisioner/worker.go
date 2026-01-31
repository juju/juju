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

	api "github.com/juju/juju/api/controller/caasapplicationprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/storageprovisioning"
	internalworker "github.com/juju/juju/internal/worker"
)

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	ProvisioningInfo(context.Context, string) (api.ProvisioningInfo, error)
	FilesystemProvisioningInfo(context.Context, string) (api.FilesystemProvisioningInfo, error)
	RemoveUnit(ctx context.Context, unitName string) error
	WatchProvisioningInfo(context.Context, string) (watcher.NotifyWatcher, error)
	DestroyUnits(ctx context.Context, unitNames []string) error
}

// ApplicationService is used to interact with the application service.
type ApplicationService interface {
	// GetApplicationTrustSetting returns the application trust setting.
	GetApplicationTrustSetting(ctx context.Context, appName string) (bool, error)

	// WatchApplicationSettings watches for changes to the specified
	// application's settings.
	WatchApplicationSettings(ctx context.Context, name string) (watcher.NotifyWatcher, error)

	// WatchApplicationUnitLife returns a watcher that observes changes to the
	// life of any units if an application.
	WatchApplicationUnitLife(ctx context.Context, appName string) (watcher.StringsWatcher, error)

	// WatchApplicationScale returns a watcher that observes changes to an
	// application's scale.
	WatchApplicationScale(ctx context.Context, appName string) (watcher.NotifyWatcher, error)

	// GetApplicationScale returns the desired scale of an application,
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
	GetApplicationScale(ctx context.Context, appName string) (int, error)

	// SetApplicationScalingState sets the scaling state for an application.
	SetApplicationScalingState(ctx context.Context, name string, scaleTarget int, scaling bool) error

	// GetApplicationScalingState returns the scaling state for an application.
	GetApplicationScalingState(ctx context.Context, name string) (applicationservice.ScalingState, error)

	// GetApplicationLife returns the life value for the given application UUID.
	GetApplicationLife(ctx context.Context, id application.UUID) (life.Value, error)

	// GetUnitLife returns the life value for the given unit name.
	GetUnitLife(context.Context, unit.Name) (life.Value, error)

	// GetAllUnitLifeForApplication returns a map of the unit names and their
	// life values for the given application.
	GetAllUnitLifeForApplication(context.Context, application.UUID) (map[unit.Name]life.Value, error)

	// GetApplicationName returns the application name for the given application
	// UUID.
	GetApplicationName(ctx context.Context, id application.UUID) (string, error)

	// WatchApplications returns a watcher that observes changes to applications.
	WatchApplications(ctx context.Context) (watcher.StringsWatcher, error)

	// UpsertCloudService updates the cloud service for the specified application.
	UpdateCloudService(ctx context.Context, appName, providerID string, sAddrs network.ProviderAddresses) error

	// IsControllerApplication returns true when the application is the controller.
	IsControllerApplication(ctx context.Context, id application.UUID) (bool, error)

	// UpdateCAASUnit updates the specified CAAS unit
	UpdateCAASUnit(context.Context, unit.Name, applicationservice.UpdateCAASUnitParams) error

	// GetAllUnitCloudContainerIDsForApplication returns a map of the unit names
	// and their cloud container provider IDs for the given application.
	GetAllUnitCloudContainerIDsForApplication(ctx context.Context, id application.UUID) (map[unit.Name]string, error)

	// GetCharmByApplicationUUID returns the charm for the specified application
	// UUID.
	GetCharmByApplicationUUID(context.Context, application.UUID) (internalcharm.Charm, applicationcharm.CharmLocator, error)

	GetApplicationStorageUniqueID(context.Context, application.UUID) (string, error)
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

// StatusService is used to get and set application status.
type StatusService interface {
	// GetUnitAgentStatusesForApplication returns the agent statuses of all
	// units in the specified application, indexed by unit name.
	GetUnitAgentStatusesForApplication(ctx context.Context, appID application.UUID) (map[unit.Name]status.StatusInfo, error)

	// SetOperatorStatus saves the given operator status, overwriting any
	// current status data.
	SetOperatorStatus(ctx context.Context, name string, info status.StatusInfo) error
}

// AgentPasswordService is used to set application agent passwords.
type AgentPasswordService interface {
	// SetApplicationPassword sets the password for the given application.
	SetApplicationPassword(ctx context.Context, appID application.UUID, password string) error
}

type StorageProvisioningService interface {
	// GetFilesystemTemplatesForApplication returns all the filesystem templates
	// for a given application.
	GetFilesystemTemplatesForApplication(ctx context.Context, appID application.UUID) ([]storageprovisioning.FilesystemTemplate, error)
	// GetStorageResourceTagsForApplication returns the storage resource tags for
	// the given application. These tags are used when creating a resource in an
	// environ.
	GetStorageResourceTagsForApplication(ctx context.Context, appID application.UUID) (map[string]string, error)
}

// ResourceOpenerGetter provides a way to get a resource opener for an
// application.
type ResourceOpenerGetter interface {
	ResourceOpenerForApplication(ctx context.Context, appID application.UUID, appName string) (coreresource.Opener, error)
}

// ResourceOpenerGetterFunc is a function that gets a resource opener for an
// application.
type ResourceOpenerGetterFunc func(context.Context, application.UUID, string) (coreresource.Opener, error)

// ResourceOpenerForApplication calls the function to get a resource opener
// for an application.
func (f ResourceOpenerGetterFunc) ResourceOpenerForApplication(ctx context.Context, appID application.UUID, appName string) (coreresource.Opener, error) {
	return f(ctx, appID, appName)
}

// Config defines the operation of a Worker.
type Config struct {
	ApplicationService         ApplicationService
	StatusService              StatusService
	AgentPasswordService       AgentPasswordService
	StorageProvisioningService StorageProvisioningService
	ResourceOpenerGetter       ResourceOpenerGetter
	Facade                     CAASProvisionerFacade
	Broker                     CAASBroker
	Clock                      clock.Clock
	Logger                     logger.Logger
	NewAppWorker               NewAppWorkerFunc
}

type provisioner struct {
	catacomb                   catacomb.Catacomb
	runner                     Runner
	applicationService         ApplicationService
	statusService              StatusService
	agentPasswordService       AgentPasswordService
	storageProvisioningService StorageProvisioningService
	resourceOpenerGetter       ResourceOpenerGetter
	Facade                     CAASProvisionerFacade
	facade                     CAASProvisionerFacade
	broker                     CAASBroker
	clock                      clock.Clock
	logger                     logger.Logger
	newAppWorker               NewAppWorkerFunc
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
		applicationService:         config.ApplicationService,
		statusService:              config.StatusService,
		agentPasswordService:       config.AgentPasswordService,
		storageProvisioningService: config.StorageProvisioningService,
		resourceOpenerGetter:       config.ResourceOpenerGetter,
		facade:                     config.Facade,
		broker:                     config.Broker,
		clock:                      config.Clock,
		logger:                     config.Logger,
		newAppWorker:               config.NewAppWorker,
		runner:                     runner,
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
				appID, err := application.ParseUUID(id)
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
					AppID:                      appID,
					ApplicationService:         p.applicationService,
					StatusService:              p.statusService,
					AgentPasswordService:       p.agentPasswordService,
					StorageProvisioningService: p.storageProvisioningService,
					ResourceOpenerGetter:       p.resourceOpenerGetter,
					Facade:                     p.facade,
					Broker:                     p.broker,
					Clock:                      p.clock,
					Logger:                     p.logger.Child(id),
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
