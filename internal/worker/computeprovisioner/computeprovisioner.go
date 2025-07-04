// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provisionertask"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// Ensure our structs implement the required Provisioner interface.
var _ Provisioner = (*environProvisioner)(nil)

var (
	retryStrategyDelay = 10 * time.Second
	retryStrategyCount = 10
)

// Provisioner represents a running provisioner worker.
type Provisioner interface {
	worker.Worker
	getMachineWatcher(context.Context) (watcher.StringsWatcher, error)
	getRetryWatcher(context.Context) (watcher.NotifyWatcher, error)
}

// ControllerAPI describes API methods for querying a controller.
type ControllerAPI interface {
	ControllerConfig(context.Context) (controller.Config, error)
	CACert(context.Context) (string, error)
	ModelUUID(context.Context) (string, error)
	ModelConfig(context.Context) (*config.Config, error)
	WatchForModelConfigChanges(context.Context) (watcher.NotifyWatcher, error)
	APIAddresses(context.Context) ([]string, error)
}

// MachinesAPI describes API methods required to access machine provisioning info.
type MachinesAPI interface {
	Machines(context.Context, ...names.MachineTag) ([]apiprovisioner.MachineResult, error)
	MachinesWithTransientErrors(context.Context) ([]apiprovisioner.MachineStatusResult, error)
	WatchMachineErrorRetry(context.Context) (watcher.NotifyWatcher, error)
	WatchModelMachines(context.Context) (watcher.StringsWatcher, error)
	ProvisioningInfo(_ context.Context, machineTags []names.MachineTag) (params.ProvisioningInfoResults, error)
}

// Environ describes the methods for provisioning instances.
type Environ interface {
	environs.InstanceBroker
	environs.ConfigSetter
}

// ToolsFinder is an interface used for finding tools to run on
// provisioned instances.
type ToolsFinder interface {
	// FindTools returns a list of tools matching the specified
	// version, os, and architecture. If arch is empty, the
	// implementation is expected to use a well documented default.
	FindTools(ctx context.Context, version semversion.Number, os string, arch string) (coretools.List, error)
}

// DistributionGroupFinder provides access to machine distribution groups.
type DistributionGroupFinder interface {
	DistributionGroupByMachineId(context.Context, ...names.MachineTag) ([]apiprovisioner.DistributionGroupResult, error)
}

// environProvisioner represents a running provisioning worker for machine nodes
// belonging to an environment.
type environProvisioner struct {
	environ                 Environ
	configObserver          configObserver
	controllerAPI           ControllerAPI
	machineService          MachineService
	machinesAPI             MachinesAPI
	agentConfig             agent.Config
	logger                  logger.Logger
	broker                  environs.InstanceBroker
	distributionGroupFinder DistributionGroupFinder
	toolsFinder             ToolsFinder
	catacomb                catacomb.Catacomb
}

// configObserver is implemented so that tests can see when the environment
// configuration changes.
// The catacomb is set in export_test to the provider's member.
// This is used to prevent notify from blocking a provisioner that has had its
// Kill method invoked.
type configObserver struct {
	sync.Mutex
	observer chan<- *config.Config
	catacomb *catacomb.Catacomb
}

// notify notifies the observer of a configuration change.
func (o *configObserver) notify(cfg *config.Config) {
	o.Lock()
	if o.observer != nil {
		select {
		case o.observer <- cfg:
		case <-o.catacomb.Dying():
		}
	}
	o.Unlock()
}

// Kill implements worker.Worker.Kill.
func (p *environProvisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (p *environProvisioner) Wait() error {
	return p.catacomb.Wait()
}

// getStartTask creates a new worker for the provisioner,
func (p *environProvisioner) getStartTask(ctx context.Context, workerCount int) (provisionertask.ProvisionerTask, error) {
	// Start responding to changes in machines, and to any further updates
	// to the environment config.
	machineWatcher, err := p.getMachineWatcher(ctx)
	if err != nil {
		return nil, err
	}
	retryWatcher, err := p.getRetryWatcher(ctx)
	if err != nil && !errors.Is(err, errors.NotImplemented) {
		return nil, err
	}
	hostTag := p.agentConfig.Tag()
	if kind := hostTag.Kind(); kind != names.ControllerAgentTagKind && kind != names.MachineTagKind {
		return nil, errors.Errorf("agent's tag is not a machine or controller agent tag, got %T", hostTag)
	}

	modelCfg, err := p.controllerAPI.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve the model config.")
	}

	controllerCfg, err := p.controllerAPI.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve the controller config.")
	}

	task, err := provisionertask.NewProvisionerTask(provisionertask.TaskConfig{
		ControllerUUID:               controllerCfg.ControllerUUID(),
		HostTag:                      hostTag,
		Logger:                       p.logger,
		ControllerAPI:                p.controllerAPI,
		MachinesAPI:                  p.machinesAPI,
		GetMachineInstanceInfoSetter: p.machineInstanceInfoSetter,
		DistributionGroupFinder:      p.distributionGroupFinder,
		ToolsFinder:                  p.toolsFinder,
		MachineWatcher:               machineWatcher,
		RetryWatcher:                 retryWatcher,
		Broker:                       p.broker,
		ImageStream:                  modelCfg.ImageStream(),
		RetryStartInstanceStrategy: provisionertask.RetryStrategy{
			RetryDelay: retryStrategyDelay,
			RetryCount: retryStrategyCount,
		},
		NumProvisionWorkers: workerCount, // event callback is currently only being used by tests
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return task, nil
}

// machineInstanceInfoSetter provides the mechanism for setting instance info
// for compute (machine) resources.
// This implementation uses the machine service, because the compute
// provisioner is used on the controller where it's available.
func (p *environProvisioner) machineInstanceInfoSetter(machineProvisioner apiprovisioner.MachineProvisioner) func(
	ctx context.Context,
	id instance.Id, displayName string, nonce string, hc *instance.HardwareCharacteristics,
	networkConfig []params.NetworkConfig, volumes []params.Volume,
	volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
) error {
	return func(
		ctx context.Context,
		id instance.Id, displayName string, nonce string, hc *instance.HardwareCharacteristics,
		networkConfig []params.NetworkConfig, volumes []params.Volume,
		volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
	) error {
		machineName := coremachine.Name(machineProvisioner.Tag().Id())
		machineUUID, err := p.machineService.GetMachineUUID(ctx, machineName)
		if err != nil {
			return errors.Annotatef(err, "retrieving machineUUID for machine %q", machineName)
		}
		if err := p.machineService.SetMachineCloudInstance(
			ctx,
			machineUUID,
			id,
			displayName,
			nonce,
			hc,
		); err != nil {
			return errors.Annotatef(err, "setting machine cloud instance for machine uuid %q", machineUUID)
		}
		return nil
	}
}

// NewEnvironProvisioner returns a new Provisioner for an environment.
// When new machines are added to the state, it allocates instances
// from the environment and allocates them to the new machines.
func NewEnvironProvisioner(
	controllerAPI ControllerAPI,
	machineService MachineService,
	machinesAPI MachinesAPI,
	toolsFinder ToolsFinder,
	distributionGroupFinder DistributionGroupFinder,
	agentConfig agent.Config,
	logger logger.Logger,
	environ Environ,
) (Provisioner, error) {
	if logger == nil {
		return nil, errors.NotValidf("missing logger")
	}
	p := &environProvisioner{
		agentConfig:             agentConfig,
		logger:                  logger,
		controllerAPI:           controllerAPI,
		machineService:          machineService,
		machinesAPI:             machinesAPI,
		toolsFinder:             toolsFinder,
		distributionGroupFinder: distributionGroupFinder,
		environ:                 environ,
	}
	p.broker = environ
	logger.Tracef(context.Background(), "Starting environ provisioner for %q", p.agentConfig.Tag())

	err := catacomb.Invoke(catacomb.Plan{
		Name: "environ-provisioner",
		Site: &p.catacomb,
		Work: p.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p, nil
}

func (p *environProvisioner) loop() error {
	ctx, cancel := p.scopedContext()
	defer cancel()

	// TODO(mjs channeling axw) - It would be better if there were
	// APIs to watch and fetch provisioner specific config instead of
	// watcher for all changes to model config. This would avoid the
	// need for a full model config.
	var modelConfigChanges <-chan struct{}
	modelWatcher, err := p.controllerAPI.WatchForModelConfigChanges(ctx)
	if err != nil {
		return loggedErrorStack(p.logger, errors.Trace(err))
	}
	if err := p.catacomb.Add(modelWatcher); err != nil {
		return errors.Trace(err)
	}
	modelConfigChanges = modelWatcher.Changes()

	modelConfig, err := p.controllerAPI.ModelConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	p.configObserver.notify(modelConfig)
	workerCount := modelConfig.NumProvisionWorkers()
	task, err := p.getStartTask(ctx, workerCount)
	if err != nil {
		return loggedErrorStack(p.logger, errors.Trace(err))
	}
	if err := p.catacomb.Add(task); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case _, ok := <-modelConfigChanges:
			if !ok {
				return errors.New("model configuration watcher closed")
			}
			modelConfig, err := p.controllerAPI.ModelConfig(ctx)
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration")
			}

			if err := p.setConfig(ctx, modelConfig); err != nil {
				return errors.Annotate(err, "loaded invalid model configuration")
			}
			task.SetNumProvisionWorkers(modelConfig.NumProvisionWorkers())
		}
	}
}

func (p *environProvisioner) getMachineWatcher(ctx context.Context) (watcher.StringsWatcher, error) {
	return p.machinesAPI.WatchModelMachines(ctx)
}

func (p *environProvisioner) getRetryWatcher(ctx context.Context) (watcher.NotifyWatcher, error) {
	return p.machinesAPI.WatchMachineErrorRetry(ctx)
}

// setConfig updates the environment configuration and notifies
// the config observer.
func (p *environProvisioner) setConfig(ctx context.Context, modelConfig *config.Config) error {
	if err := p.environ.SetConfig(ctx, modelConfig); err != nil {
		return errors.Trace(err)
	}
	p.configObserver.notify(modelConfig)
	return nil
}

func (p *environProvisioner) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(p.catacomb.Context(context.Background()))
}
